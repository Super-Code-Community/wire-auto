// Package exec запускает скрипт отдельным процессом и качает протокол моста
// до результата, применяя таймауты старта и выполнения. v2: обслуживает
// двусторонний канал request/response при protocol >= 2.
package exec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"wire-auto/cores/duplex/internal/capreg"
	"wire-auto/cores/duplex/internal/protocol"
)

const (
	StatusOK                = "OK"
	StatusScriptError       = "SCRIPT_ERROR"
	StatusProtocolViolation = "PROTOCOL_VIOLATION"
	StatusStartupTimeout    = "STARTUP_TIMEOUT"
	StatusRunTimeout        = "RUN_TIMEOUT"
	StatusCrashed           = "CRASHED"
	StatusCancelled         = "CANCELLED"
)

type LogLine struct {
	Level   string
	Message string
}

type Spec struct {
	Dir            string
	Command        string
	Args           []string
	Env            []string
	Protocol       int
	CoreAPI        int
	ScriptArgs     []string
	StartupTimeout time.Duration
	RunTimeout     time.Duration
	CancelGrace    time.Duration
	OnEvent        func(Event)
	// v2: авторизованные capability и реестр обработчиков.
	Provides []string
	Registry map[string]capreg.Handler
}

type Result struct {
	Status       string
	ExitCode     int
	Logs         []LogLine
	ErrorCode    string
	ErrorMessage string
	Result       json.RawMessage
}

type msgOrErr struct {
	msg protocol.Message
	err error
}

func containsStr(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// dispatchRequest применяет двойной гейт (provides → registry) и строит response.
// Ошибка capability уезжает в response.Code и прогон не роняет.
func dispatchRequest(spec Spec, req protocol.Message) protocol.Message {
	resp := protocol.Message{Type: protocol.TypeResponse, ID: req.ID}
	if !containsStr(spec.Provides, req.Capability) {
		resp.Code = "CAPABILITY_DENIED"
		resp.Message = "core does not provide capability " + req.Capability
		return resp
	}
	handler, ok := spec.Registry[req.Capability]
	if !ok {
		resp.Code = "CAPABILITY_UNIMPLEMENTED"
		resp.Message = "no handler for capability " + req.Capability
		return resp
	}
	result, code, err := handler(req.Params)
	if code != "" {
		resp.Code = code
		if err != nil {
			resp.Message = err.Error()
		}
		return resp
	}
	resp.Result = result
	return resp
}

func Run(ctx context.Context, spec Spec) Result {
	emit := func(e Event) {
		if spec.OnEvent != nil {
			spec.OnEvent(e)
		}
	}

	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{Status: StatusCrashed, ErrorCode: StatusCrashed, ErrorMessage: err.Error()}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{Status: StatusCrashed, ErrorCode: StatusCrashed, ErrorMessage: err.Error()}
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return Result{Status: StatusCrashed, ErrorCode: StatusCrashed, ErrorMessage: err.Error()}
	}

	_ = protocol.Encode(stdin, protocol.Message{
		Type:     protocol.TypeHello,
		Protocol: spec.Protocol,
		CoreAPI:  spec.CoreAPI,
		Args:     spec.ScriptArgs,
	})

	dec := protocol.NewDecoder(stdout)
	ch := make(chan msgOrErr)
	readerDone := make(chan struct{})
	defer close(readerDone)
	go func() {
		for {
			m, err := dec.Next()
			select {
			case ch <- msgOrErr{m, err}:
			case <-readerDone:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	res := Result{Logs: []LogLine{}}
	gotReady := false
	deadline := time.After(spec.StartupTimeout)

	kill := func(status, code, msg string) Result {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		res.Status, res.ErrorCode, res.ErrorMessage = status, code, msg
		return res
	}

loop:
	for {
		select {
		case <-deadline:
			if !gotReady {
				return kill(StatusStartupTimeout, StatusStartupTimeout, "script did not send ready in time")
			}
			_ = protocol.Encode(stdin, protocol.Message{Type: protocol.TypeCancel})
			if spec.CancelGrace > 0 {
				grace := time.After(spec.CancelGrace)
			graceLoop:
				for {
					select {
					case ev := <-ch:
						if ev.err != nil {
							break graceLoop
						}
					case <-grace:
						break graceLoop
					}
				}
			}
			return kill(StatusRunTimeout, StatusRunTimeout, "")
		case <-ctx.Done():
			_ = protocol.Encode(stdin, protocol.Message{Type: protocol.TypeCancel})
			if spec.CancelGrace > 0 {
				grace := time.After(spec.CancelGrace)
			graceCancel:
				for {
					select {
					case ev := <-ch:
						if ev.err != nil {
							break graceCancel
						}
					case <-grace:
						break graceCancel
					}
				}
			}
			return kill(StatusCancelled, StatusCancelled, "cancelled by client")
		case ev := <-ch:
			if ev.err != nil {
				if errors.Is(ev.err, io.EOF) {
					break loop
				}
				return kill(StatusProtocolViolation, StatusProtocolViolation, ev.err.Error())
			}
			switch ev.msg.Type {
			case protocol.TypeReady:
				gotReady = true
				deadline = time.After(spec.RunTimeout)
				emit(Event{Kind: "ready"})
			case protocol.TypeLog:
				res.Logs = append(res.Logs, LogLine{Level: ev.msg.Level, Message: ev.msg.Message})
				emit(Event{Kind: "log", Level: ev.msg.Level, Message: ev.msg.Message})
			case protocol.TypeRequest:
				if spec.Protocol < 2 {
					return kill(StatusProtocolViolation, StatusProtocolViolation, "request not allowed on protocol 1")
				}
				if !gotReady {
					return kill(StatusProtocolViolation, StatusProtocolViolation, "request before ready")
				}
				_ = protocol.Encode(stdin, dispatchRequest(spec, ev.msg))
			case protocol.TypeDone:
				code := 0
				if ev.msg.ExitCode != nil {
					code = *ev.msg.ExitCode
				}
				res.ExitCode = code
				res.Result = ev.msg.Result
				_ = cmd.Wait()
				if code == 0 {
					res.Status = StatusOK
				} else {
					res.Status = StatusScriptError
				}
				return res
			default:
				return kill(StatusProtocolViolation, StatusProtocolViolation, "unexpected message type: "+ev.msg.Type)
			}
		}
	}

	_ = cmd.Wait()
	res.Status = StatusCrashed
	res.ErrorCode = StatusCrashed
	res.ErrorMessage = "process exited without done"
	return res
}

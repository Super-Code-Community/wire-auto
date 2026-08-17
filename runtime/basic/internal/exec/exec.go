// Package exec запускает скрипт отдельным процессом и качает протокол моста
// до результата, применяя таймауты старта и выполнения.
package exec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"wire-auto/runtime/basic/internal/protocol"
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
	// CancelGrace is how long to wait after sending a cancel message before hard-killing the child process.
	CancelGrace time.Duration
	// OnEvent, если задан, получает ready/log по ходу прогона (для стриминга наружу).
	OnEvent func(Event)
}

type Result struct {
	Status       string
	ExitCode     int
	Logs         []LogLine
	ErrorCode    string
	ErrorMessage string
	Result       json.RawMessage
}

// msgOrErr — одно прочитанное сообщение либо ошибка чтения потока.
type msgOrErr struct {
	msg protocol.Message
	err error
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
	cmd.Stderr = os.Stderr // сырой лог самого шима — в диагностику

	if err := cmd.Start(); err != nil {
		return Result{Status: StatusCrashed, ErrorCode: StatusCrashed, ErrorMessage: err.Error()}
	}

	// Отправляем hello.
	// Child's stdin is intentionally left open for v1: the shim signals completion
	// by sending "done", not by reading stdin to EOF.
	_ = protocol.Encode(stdin, protocol.Message{
		Type:     protocol.TypeHello,
		Protocol: spec.Protocol,
		CoreAPI:  spec.CoreAPI,
		Args:     spec.ScriptArgs,
	})

	// Читаем сообщения в отдельной горутине, чтобы наложить таймауты.
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
			// cancel → grace → kill sequence; outcome is always RUN_TIMEOUT.
			// Send a best-effort cancel message; ignore write errors (pipe may already be closed).
			_ = protocol.Encode(stdin, protocol.Message{Type: protocol.TypeCancel})
			if spec.CancelGrace > 0 {
				// Wait up to CancelGrace for the process to wind down gracefully.
				grace := time.After(spec.CancelGrace)
			graceLoop:
				for {
					select {
					case ev := <-ch:
						// Drain messages; EOF or read error means the process is ending.
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
			// Внешняя отмена клиента: тот же путь cancel→grace→kill, статус CANCELLED.
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
					// Поток кончился без done → падение.
					break loop
				}
				return kill(StatusProtocolViolation, StatusProtocolViolation, ev.err.Error())
			}
			switch ev.msg.Type {
			case protocol.TypeReady:
				gotReady = true
				deadline = time.After(spec.RunTimeout) // переключаемся на таймаут выполнения
				emit(Event{Kind: "ready"})
			case protocol.TypeLog:
				res.Logs = append(res.Logs, LogLine{Level: ev.msg.Level, Message: ev.msg.Message})
				emit(Event{Kind: "log", Level: ev.msg.Level, Message: ev.msg.Message})
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
				// Host→shim messages (hello, cancel) and reserved types (request) are
				// not valid shim→host messages; any such type is a protocol violation.
				return kill(StatusProtocolViolation, StatusProtocolViolation, "unexpected message type: "+ev.msg.Type)
			}
		}
	}

	// Вышли по EOF без done.
	_ = cmd.Wait()
	res.Status = StatusCrashed
	res.ErrorCode = StatusCrashed
	res.ErrorMessage = "process exited without done"
	return res
}

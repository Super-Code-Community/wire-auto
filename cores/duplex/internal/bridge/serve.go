package bridge

import (
	"context"
	"fmt"
	"io"
	"sync"

	"wire-auto/cores/duplex/internal/exec"
)

// Deps — инъекция зависимостей моста (для тестируемости).
type Deps struct {
	List func() ([]Script, error)
	Run  func(ctx context.Context, dir string, onEvent func(exec.Event), answers <-chan exec.PromptAnswer) (exec.Result, error)
}

// runDone — сигнал о завершении одного прогона.
type runDone struct{}

// Serve крутит вечный цикл моста, пока не придёт exit или EOF stdin.
// Прогоны последовательны (один за раз); во время прогона принимаются только
// cancel (отменяет бегущий скрипт через контекст). Все остальные команды,
// включая exit, откладываются до конца прогона. Паника внутри прогона
// изолируется: наружу уходит event error, мост продолжает жить.
func Serve(in io.Reader, out io.Writer, deps Deps) error {
	var mu sync.Mutex
	write := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		_ = EncodeEvent(out, e)
	}

	cmds := make(chan Command)
	go func() {
		dec := NewCommandDecoder(in)
		for {
			c, err := dec.Next()
			if err != nil {
				close(cmds)
				return
			}
			cmds <- c
		}
	}()

	done := make(chan runDone, 1)
	var cancelCur context.CancelFunc

	finish := func() {
		if cancelCur != nil {
			cancelCur()
			cancelCur = nil
		}
	}

	// pending holds commands received while a run was in progress.
	// Only cancel is acted on immediately; everything else (including exit)
	// is deferred until the run finishes so events are emitted in order.
	var pending []Command
	// stdinClosed is set when the cmds channel is closed during a run-wait.
	stdinClosed := false

	for {
		// Drain pending commands from a previous run before reading new ones.
		for len(pending) > 0 {
			c := pending[0]
			pending = pending[1:]
			switch c.Type {
			case "list":
				scripts, err := deps.List()
				if err != nil {
					write(Event{Type: "error", Message: err.Error()})
					continue
				}
				if scripts == nil {
					scripts = []Script{}
				}
				write(Event{Type: "catalog", Scripts: scripts})
			case "cancel":
				// No active run after done; ignore.
			case "exit":
				finish()
				return nil
			}
			// Note: "run" is never buffered into pending (it is rejected
			// immediately in the waitLoop), so it needs no case here.
		}

		if stdinClosed {
			finish()
			return nil
		}

		c, ok := <-cmds
		if !ok {
			finish()
			return nil
		}
		switch c.Type {
		case "list":
			scripts, err := deps.List()
			if err != nil {
				write(Event{Type: "error", Message: err.Error()})
				continue
			}
			if scripts == nil {
				scripts = []Script{}
			}
			write(Event{Type: "catalog", Scripts: scripts})

		case "run":
			ctx, cancel := context.WithCancel(context.Background())
			cancelCur = cancel
			answers := make(chan exec.PromptAnswer, 1)
			go func(dir string) {
				defer func() {
					if r := recover(); r != nil {
						write(Event{Type: "error", Message: fmt.Sprintf("run panicked: %v", r)})
					}
					done <- runDone{}
				}()
				res, err := deps.Run(ctx, dir, func(ev exec.Event) {
					switch ev.Kind {
					case "ready":
						write(Event{Type: "ready"})
					case "log":
						write(Event{Type: "log", Level: ev.Level, Message: ev.Message})
					case "prompt":
						write(Event{Type: "prompt", ID: ev.ID, Message: ev.Message})
					}
				}, answers)
				if err != nil {
					write(Event{Type: "error", Message: err.Error()})
					return
				}
				write(Event{
					Type:         "result",
					Status:       res.Status,
					ExitCode:     res.ExitCode,
					ErrorCode:    res.ErrorCode,
					ErrorMessage: res.ErrorMessage,
					Result:       res.Result,
				})
			}(c.Dir)

			// Wait for the run to complete. Only cancel is acted on immediately
			// (via context); all other commands are buffered for after done.
		waitLoop:
			for {
				select {
				case <-done:
					cancelCur = nil
					break waitLoop
				case nc, nok := <-cmds:
					if !nok {
						stdinClosed = true
						// If a graceful exit was already buffered, let the
						// current run finish naturally so its result is emitted
						// before the bridge shuts down. Only hard-cancel when
						// there is no pending exit (e.g. the client disconnected
						// abruptly without sending exit).
						exitPending := false
						for _, p := range pending {
							if p.Type == "exit" {
								exitPending = true
								break
							}
						}
						if !exitPending {
							finish()
						}
						<-done
						cancelCur = nil
						break waitLoop
					}
					switch nc.Type {
					case "cancel":
						if cancelCur != nil {
							cancelCur()
						}
					case "input":
						// Доставить ответ на prompt в exec. Буфер cap 1 не даёт
						// подвиснуть; <-done страхует, если прогон уже кончился.
						select {
						case answers <- exec.PromptAnswer{ID: nc.ID, Value: nc.Value}:
						case <-done:
						}
					case "run":
						write(Event{Type: "error", Message: "busy: a script is already running"})
					default:
						// Defer list/exit until after done so their output stays
						// ordered after the active run's terminal event.
						pending = append(pending, nc)
					}
				}
			}

		case "cancel":
			// No active run.

		case "exit":
			finish()
			return nil
		}
	}
}

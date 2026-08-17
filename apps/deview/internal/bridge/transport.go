package bridge

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// closeGrace — сколько ждём мирного завершения подпроцесса в Close до жёсткого kill.
const closeGrace = 5 * time.Second

// Transport — двусторонний канал к мосту. Абстракция ради тестируемости:
// прод — подпроцесс wire, тест — in-memory фейк.
type Transport interface {
	Send(Command) error
	Recv() (Event, error) // io.EOF по концу
	Close() error
}

// ProcessTransport поднимает `wire` подпроцессом и говорит по его stdin/stdout.
type ProcessTransport struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	in  io.WriteCloser
	dec *eventDecoder
}

// NewProcessTransport запускает name с args (напр. "wire" или "go run ...").
// stderr подпроцесса пробрасывается в os.Stderr для диагностики.
func NewProcessTransport(name string, args ...string) (*ProcessTransport, error) {
	c := exec.Command(name, args...)
	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	c.Stderr = os.Stderr
	if err := c.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, err
	}
	return &ProcessTransport{cmd: c, in: stdin, dec: newEventDecoder(stdout)}, nil
}

func (p *ProcessTransport) Send(c Command) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return encodeCommand(p.in, c)
}

func (p *ProcessTransport) Recv() (Event, error) { return p.dec.next() }

func (p *ProcessTransport) Close() error {
	p.mu.Lock()
	_ = encodeCommand(p.in, Command{Type: "exit"})
	_ = p.in.Close()
	p.mu.Unlock()

	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(closeGrace):
		_ = p.cmd.Process.Kill()
		return <-done
	}
}

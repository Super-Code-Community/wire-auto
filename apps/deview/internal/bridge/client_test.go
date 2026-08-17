package bridge

import (
	"io"
	"testing"
)

// fakeTransport — программируемый поток событий; фиксирует отправленные команды.
type fakeTransport struct {
	events []Event
	i      int
	sent   []Command
}

func (f *fakeTransport) Send(c Command) error { f.sent = append(f.sent, c); return nil }
func (f *fakeTransport) Recv() (Event, error) {
	if f.i >= len(f.events) {
		return Event{}, io.EOF
	}
	e := f.events[f.i]
	f.i++
	return e, nil
}
func (f *fakeTransport) Close() error { return nil }

func TestClientListReturnsCatalog(t *testing.T) {
	ft := &fakeTransport{events: []Event{
		{Type: "catalog", Scripts: []Script{{Name: "hello", Dir: "scripts/examples/hello"}}},
	}}
	c := NewClient(ft)
	scripts, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(scripts) != 1 || scripts[0].Name != "hello" {
		t.Fatalf("catalog неверен: %+v", scripts)
	}
	if len(ft.sent) != 1 || ft.sent[0].Type != "list" {
		t.Fatalf("ожидали отправку list, got %+v", ft.sent)
	}
}

func TestClientRunStreamsThenTerminal(t *testing.T) {
	ft := &fakeTransport{events: []Event{
		{Type: "ready"},
		{Type: "log", Level: "info", Message: "hi"},
		{Type: "result", Status: "OK", ExitCode: 0},
	}}
	c := NewClient(ft)
	var streamed []Event
	term, err := c.Run("scripts/x", func(e Event) { streamed = append(streamed, e) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if term.Type != "result" || term.Status != "OK" {
		t.Fatalf("терминал неверен: %+v", term)
	}
	if len(streamed) != 2 || streamed[0].Type != "ready" || streamed[1].Type != "log" {
		t.Fatalf("стрим неверен: %+v", streamed)
	}
	if ft.sent[0].Type != "run" || ft.sent[0].Dir != "scripts/x" {
		t.Fatalf("ожидали run{scripts/x}, got %+v", ft.sent[0])
	}
}

func TestClientRunTerminatesOnError(t *testing.T) {
	ft := &fakeTransport{events: []Event{{Type: "error", Message: "nope"}}}
	c := NewClient(ft)
	term, err := c.Run("bad", func(Event) {})
	if err != nil {
		t.Fatalf("Run вернул err вместо терминального события: %v", err)
	}
	if term.Type != "error" || term.Message != "nope" {
		t.Fatalf("ожидали error-терминал, got %+v", term)
	}
}

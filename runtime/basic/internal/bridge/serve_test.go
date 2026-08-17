package bridge

import (
	"context"
	"strings"
	"sync"
	"testing"

	"wire-auto/runtime/basic/internal/exec"
)

// collectEvents декодирует NDJSON-вывод моста обратно в события.
func collectEvents(t *testing.T, out string) []Event {
	t.Helper()
	var evs []Event
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var e Event
		if err := jsonUnmarshalLine(line, &e); err != nil {
			t.Fatalf("bad event line %q: %v", line, err)
		}
		evs = append(evs, e)
	}
	return evs
}

func TestServeListThenRunThenExit(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"type":"list"}`,
		`{"type":"run","dir":"scripts/x"}`,
		`{"type":"exit"}`,
	}, "\n") + "\n")
	var out syncBuffer

	deps := Deps{
		List: func() ([]Script, error) {
			return []Script{{Name: "x", Dir: "scripts/x", Capabilities: []string{}}}, nil
		},
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) {
			onEvent(exec.Event{Kind: "ready"})
			onEvent(exec.Event{Kind: "log", Level: "info", Message: "hi"})
			return exec.Result{Status: exec.StatusOK, ExitCode: 0}, nil
		},
	}

	if err := Serve(in, &out, deps); err != nil {
		t.Fatalf("Serve error: %v", err)
	}

	evs := collectEvents(t, out.String())
	types := make([]string, len(evs))
	for i, e := range evs {
		types[i] = e.Type
	}
	want := []string{"catalog", "ready", "log", "result"}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Fatalf("порядок событий = %v, ожидали %v", types, want)
	}
	if evs[0].Scripts[0].Name != "x" {
		t.Fatalf("каталог без скрипта x: %+v", evs[0])
	}
	if evs[3].Status != exec.StatusOK {
		t.Fatalf("result status = %s", evs[3].Status)
	}
}

func TestServeSurvivesFailedRun(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"type":"run","dir":"bad"}`,
		`{"type":"list"}`,
		`{"type":"exit"}`,
	}, "\n") + "\n")
	var out syncBuffer

	deps := Deps{
		List: func() ([]Script, error) { return []Script{}, nil },
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) {
			panic("boom")
		},
	}

	if err := Serve(in, &out, deps); err != nil {
		t.Fatalf("Serve error: %v", err)
	}
	evs := collectEvents(t, out.String())
	// Первое событие — error от упавшего прогона; затем мост жив и отдаёт catalog.
	if len(evs) < 2 || evs[0].Type != "error" {
		t.Fatalf("ожидали error затем catalog, got %+v", evs)
	}
	var sawCatalog bool
	for _, e := range evs {
		if e.Type == "catalog" {
			sawCatalog = true
		}
	}
	if !sawCatalog {
		t.Fatal("мост не пережил упавший прогон (нет catalog после)")
	}
}

// --- маленькие тест-хелперы ---

type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

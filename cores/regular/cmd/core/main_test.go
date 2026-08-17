package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"wire-auto/cores/regular/internal/bridge"
	"wire-auto/cores/regular/internal/exec"
)

// repoRoot: .../cores/regular/cmd/core → поднимаемся на четыре уровня.
func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func coreManifest(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "cores", "regular", "core.manifest")
}

func TestEndToEndHello(t *testing.T) {
	root := repoRoot(t)
	res, err := run(coreManifest(t), filepath.Join(root, "scripts", "examples", "hello"))
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Status != "OK" {
		t.Fatalf("status=%s err=%s", res.Status, res.ErrorMessage)
	}
	if len(res.Logs) != 1 || res.Logs[0].Message != "hello from python" {
		t.Fatalf("logs=%+v", res.Logs)
	}
}

func TestEndToEndCapabilityDenied(t *testing.T) {
	res, err := run(coreManifest(t), filepath.Join("testdata", "needs-serial"))
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Status != "HANDSHAKE_FAILED" || res.ErrorCode != "CAPABILITY_DENIED" {
		t.Fatalf("status=%s code=%s", res.Status, res.ErrorCode)
	}
}

func TestEndToEndUnknownCore(t *testing.T) {
	res, err := run(coreManifest(t), filepath.Join("testdata", "needs-ghost"))
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Status != "HANDSHAKE_FAILED" || res.ErrorCode != "UNKNOWN_CORE" {
		t.Fatalf("status=%s code=%s", res.Status, res.ErrorCode)
	}
}

func TestBridgeRunsHelloEndToEnd(t *testing.T) {
	root := repoRoot(t)
	cm := coreManifest(t)
	deps := bridge.Deps{
		List: func() ([]bridge.Script, error) { return nil, nil },
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) {
			return runStreaming(ctx, cm, dir, onEvent)
		},
	}
	in := strings.NewReader(
		`{"type":"run","dir":"` + filepath.ToSlash(filepath.Join(root, "scripts", "examples", "hello")) + `"}` + "\n" +
			`{"type":"exit"}` + "\n")
	var out strings.Builder
	if err := bridge.Serve(in, &out, deps); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !strings.Contains(out.String(), `"type":"ready"`) ||
		!strings.Contains(out.String(), "hello from python") ||
		!strings.Contains(out.String(), `"status":"OK"`) {
		t.Fatalf("мост не отдал ожидаемые события:\n%s", out.String())
	}
}

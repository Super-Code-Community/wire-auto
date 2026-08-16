package main

import (
	"path/filepath"
	"testing"
)

// repoRoot: .../runtime/basic/cmd/wire → поднимаемся на четыре уровня.
func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestEndToEndHello(t *testing.T) {
	root := repoRoot(t)
	res, err := run(
		filepath.Join(root, "runtime", "basic", "runtime.manifest"),
		filepath.Join(root, "cores"),
		filepath.Join(root, "scripts", "examples", "hello"),
	)
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
	root := repoRoot(t)
	res, err := run(
		filepath.Join(root, "runtime", "basic", "runtime.manifest"),
		filepath.Join(root, "cores"),
		filepath.Join(root, "runtime", "basic", "cmd", "wire", "testdata", "needs-serial"),
	)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Status != "HANDSHAKE_FAILED" || res.ErrorCode != "CAPABILITY_DENIED" {
		t.Fatalf("status=%s code=%s", res.Status, res.ErrorCode)
	}
}

func TestEndToEndUnknownCore(t *testing.T) {
	root := repoRoot(t)
	res, err := run(
		filepath.Join(root, "runtime", "basic", "runtime.manifest"),
		filepath.Join(root, "cores"),
		filepath.Join(root, "runtime", "basic", "cmd", "wire", "testdata", "needs-ghost"),
	)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Status != "HANDSHAKE_FAILED" || res.ErrorCode != "UNKNOWN_CORE" {
		t.Fatalf("status=%s code=%s", res.Status, res.ErrorCode)
	}
}

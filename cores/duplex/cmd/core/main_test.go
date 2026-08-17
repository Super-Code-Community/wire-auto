package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"wire-auto/cores/duplex/internal/bridge"
	"wire-auto/cores/duplex/internal/exec"
)

func coreManifest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "cores", "duplex", "core.manifest")
}

func TestE2EEnvReport(t *testing.T) {
	t.Setenv("USER", "cyrille")
	root, _ := filepath.Abs("../../../..")
	res, err := run(coreManifest(t), filepath.Join(root, "scripts", "examples", "env-report"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "OK" {
		t.Fatalf("status=%s err=%s %s", res.Status, res.ErrorCode, res.ErrorMessage)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "USER=cyrille") {
		t.Fatalf("logs=%+v", res.Logs)
	}
}

func TestE2EEnvReportNode(t *testing.T) {
	t.Setenv("USER", "cyrille")
	root, _ := filepath.Abs("../../../..")
	res, err := run(coreManifest(t), filepath.Join(root, "scripts", "examples", "env-report-node"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "OK" {
		t.Fatalf("status=%s err=%s %s", res.Status, res.ErrorCode, res.ErrorMessage)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "USER=cyrille") {
		t.Fatalf("logs=%+v", res.Logs)
	}
}

func TestE2EGoHello(t *testing.T) {
	dir, err := filepath.Abs("testdata/gohello")
	if err != nil {
		t.Fatal(err)
	}
	res, err := run(coreManifest(t), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "OK" {
		t.Fatalf("status=%s err=%s %s", res.Status, res.ErrorCode, res.ErrorMessage)
	}
}

func TestE2EPromptViaBridge(t *testing.T) {
	askDir, err := filepath.Abs("testdata/ask")
	if err != nil {
		t.Fatal(err)
	}
	cm := coreManifest(t)
	deps := bridge.Deps{
		List: func() ([]bridge.Script, error) { return []bridge.Script{}, nil },
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event), answers <-chan exec.PromptAnswer) (exec.Result, error) {
			return runStreaming(ctx, cm, dir, onEvent, answers)
		},
	}
	in := bytes.NewBufferString(strings.Join([]string{
		`{"type":"run","dir":"` + strings.ReplaceAll(askDir, `\`, `\\`) + `"}`,
		`{"type":"input","id":"1","value":"Bob"}`,
		`{"type":"exit"}`,
	}, "\n") + "\n")
	var out bytes.Buffer
	if err := bridge.Serve(in, &out, deps); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, `"type":"prompt"`) {
		t.Fatalf("нет события prompt в выводе:\n%s", s)
	}
	if !strings.Contains(s, `"status":"OK"`) || !strings.Contains(s, "Bob") {
		t.Fatalf("нет успешного result со значением Bob:\n%s", s)
	}
}

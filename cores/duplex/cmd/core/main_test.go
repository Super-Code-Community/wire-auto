package main

import (
	"path/filepath"
	"strings"
	"testing"
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

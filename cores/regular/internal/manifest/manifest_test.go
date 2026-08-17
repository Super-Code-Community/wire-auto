package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadScriptValid(t *testing.T) {
	p := write(t, "script.manifest", `
name = "hello"
version = "0.1.0"
core = "regular"
coreApi = 1
link = "stdio"
cmd = ["python", "main.py"]
language = "python"
capabilities = []
`)
	s, err := LoadScript(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "hello" || s.Core != "regular" || s.CoreAPI != 1 || s.Link != "stdio" || len(s.Cmd) != 2 || s.Cmd[0] != "python" {
		t.Fatalf("bad parse: %+v", s)
	}
}

func TestLoadScriptMissingCmd(t *testing.T) {
	p := write(t, "script.manifest", `
name = "hello"
version = "0.1.0"
core = "regular"
coreApi = 1
link = "stdio"
`)
	if _, err := LoadScript(p); err == nil {
		t.Fatal("expected error for missing cmd")
	}
}

func TestLoadCoreValid(t *testing.T) {
	p := write(t, "core.manifest", `
name = "regular"
version = "0.1.0"
coreApi = 1
protocol = 1
links = ["stdio"]
`)
	c, err := LoadCore(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Protocol != 1 || len(c.Links) != 1 || c.Links[0] != "stdio" {
		t.Fatalf("bad parse: %+v", c)
	}
}

func TestLoadCoreMissingName(t *testing.T) {
	p := write(t, "core.manifest", `
version = "0.1.0"
coreApi = 1
protocol = 1
links = ["stdio"]
`)
	if _, err := LoadCore(p); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoadCoreMissingProtocol(t *testing.T) {
	p := write(t, "core.manifest", `
name = "regular"
version = "0.1.0"
coreApi = 1
links = ["stdio"]
`)
	if _, err := LoadCore(p); err == nil {
		t.Fatal("expected error for missing protocol")
	}
}

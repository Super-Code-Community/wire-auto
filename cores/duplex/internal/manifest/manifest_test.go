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

func TestLoadCoreDuplex(t *testing.T) {
	p := write(t, "core.manifest", `
name = "duplex"
version = "0.1.0"
coreApi = 1
protocol = 2
links = ["stdio"]
`)
	c, err := LoadCore(p)
	if err != nil {
		t.Fatalf("load core: %v", err)
	}
	if c.Protocol != 2 || c.Name != "duplex" || c.CoreAPI != 1 {
		t.Fatalf("bad parse: %+v", c)
	}
}

func TestLoadScriptDuplex(t *testing.T) {
	p := write(t, "script.manifest", `
name = "env-report"
version = "0.1.0"
core = "duplex"
coreApi = 1
capabilities = ["env.get"]
link = "stdio"
language = "python"
cmd = ["python", "main.py"]
`)
	s, err := LoadScript(p)
	if err != nil {
		t.Fatalf("load script: %v", err)
	}
	if s.Core != "duplex" || len(s.Capabilities) != 1 || s.Capabilities[0] != "env.get" {
		t.Fatalf("bad parse: %+v", s)
	}
}

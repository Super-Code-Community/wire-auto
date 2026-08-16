package registry

import (
	"os"
	"path/filepath"
	"testing"

	"wire-auto/runtime/basic/internal/manifest"
)

func rt() manifest.Runtime {
	return manifest.Runtime{Name: "rt", Version: "0", Protocols: []int{1}, Transports: []string{"stdio"}, Cores: []string{}}
}

func writeCore(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "core.manifest"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCompatibleOK(t *testing.T) {
	c := manifest.Core{Name: "regular", CoreAPI: 1, Protocol: 1, Links: []string{"stdio"}}
	if ok, why := Compatible(rt(), c); !ok {
		t.Fatalf("want compatible, got: %s", why)
	}
}

func TestIncompatibleProtocol(t *testing.T) {
	c := manifest.Core{Name: "x", CoreAPI: 1, Protocol: 2, Links: []string{"stdio"}}
	if ok, _ := Compatible(rt(), c); ok {
		t.Fatal("want reject on protocol mismatch")
	}
}

func TestIncompatibleTransport(t *testing.T) {
	c := manifest.Core{Name: "x", CoreAPI: 1, Protocol: 1, Links: []string{"socket"}}
	if ok, _ := Compatible(rt(), c); ok {
		t.Fatal("want reject on transport mismatch")
	}
}

func TestDiscoverPartitions(t *testing.T) {
	root := t.TempDir()
	writeCore(t, filepath.Join(root, "regular"), `name = "regular"
version = "0"
coreApi = 1
protocol = 1
provides = []
links = ["stdio"]
`)
	writeCore(t, filepath.Join(root, "future"), `name = "future"
version = "0"
coreApi = 1
protocol = 2
provides = []
links = ["stdio"]
`)
	admitted, rejected, err := Discover(root, rt())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := admitted["regular"]; !ok {
		t.Fatal("regular should be admitted")
	}
	if len(rejected) != 1 || rejected[0].Name != "future" {
		t.Fatalf("future should be rejected, got: %+v", rejected)
	}
}

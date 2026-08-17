package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script.manifest"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanFindsAndSortsValidScripts(t *testing.T) {
	root := t.TempDir()
	// Скрипты живут под категориями (examples/, tools/), т.е. на глубине >1
	// от корня. Scan обязан находить их рекурсивно.
	writeManifest(t, filepath.Join(root, "examples", "beta"), `name = "beta"
version = "0.2.0"
core = "regular"
coreApi = 1
link = "stdio"
cmd = ["python", "main.py"]
language = "python"
capabilities = []
`)
	writeManifest(t, filepath.Join(root, "tools", "flash"), `name = "alpha"
core = "regular"
coreApi = 1
link = "stdio"
cmd = ["bash", "run.sh"]
language = "bash"
capabilities = ["serial"]
`)
	// Категория без единого манифеста — должна тихо игнорироваться.
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ожидали 2 скрипта, got %d: %+v", len(got), got)
	}
	if got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Fatalf("ожидали сортировку alpha,beta, got %s,%s", got[0].Name, got[1].Name)
	}
	if got[0].Language != "bash" || len(got[0].Capabilities) != 1 || got[0].Capabilities[0] != "serial" {
		t.Fatalf("alpha метаданные неверны: %+v", got[0])
	}
	if !filepath.IsAbs(got[0].Dir) {
		t.Fatalf("Dir должен быть абсолютным, got %s", got[0].Dir)
	}
}

func TestScanMissingDirIsError(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("ожидали ошибку на отсутствующей директории")
	}
}

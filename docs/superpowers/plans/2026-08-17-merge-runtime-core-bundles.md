# Merge runtime + core into `cores/<name>` bundles — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the runtime↔core split into two self-contained `core` bundles (`cores/regular`, `cores/duplex`), each a standalone Go module that vendors its own runtime-lib copy, deleting `runtime/` and the admission layer.

**Architecture:** Each bundle is one Go module with `cmd/core` as its binary and a vendored `internal/` copy of the runtime code (WET, deliberate). `regular` (protocol 1) keeps the streaming app↔core bridge that deview drives; `duplex` (protocol 2) is a single-shot runner with the `env.get` capability. The runtime↔core admission handshake and `runtime.manifest` are removed; only two handshakes remain (app↔core, core↔script). Capability authorization derives from the `capreg` registry keys, not a manifest field.

**Tech Stack:** Go 1.26.4, `github.com/BurntSushi/toml v1.6.0`, stdio JSON Lines protocol, python3 + node on PATH for e2e tests.

## Global Constraints

- **NO git operations by any agent — ever.** No `commit`, `branch`, `checkout -b`, `switch -c`, `add`, `push`, `stash`. The user commits manually (CLAUDE.md hard rule). Each task's deliverable is "its tests pass"; stop there.
- **No root `go.work` / `go.work.sum`.** Every module builds independently: `cd cores/<name> && go build ./... && go vet ./... && go test ./...` must pass on its own.
- **All file operations use absolute Windows paths** (e.g. `D:\Projects\wire-auto\cores\regular\go.mod`).
- **WET is intentional.** Each bundle vendors its own `internal/` copy. Do NOT extract shared code into `packages/`.
- **Import-path rewrite rule (used by every copy step):** after copying a package tree, rewrite every occurrence of the old module prefix to the new one and change nothing else:
  - regular: `wire-auto/runtime/basic` → `wire-auto/cores/regular`
  - duplex: `wire-auto/runtime/duplex` → `wire-auto/cores/duplex`
  Command form (run from repo root, git-bash): `find <dest> -name '*.go' -exec sed -i 's|OLD|NEW|g' {} +`.
- Go module version line: `go 1.26.4`. Dependency: `github.com/BurntSushi/toml v1.6.0`.
- **Do not delete `runtime/` until Task 7.** deview keeps working against it until repointed in Task 4.

---

## PART A — `cores/regular` bundle (protocol 1, deview backend)

### Task 1: Scaffold module; vendor protocol/exec/discovery/bridge; slim manifest

**Files:**
- Create: `cores/regular/go.mod`
- Copy (verbatim + import rewrite): `runtime/basic/internal/{protocol,exec,discovery,bridge}` → `cores/regular/internal/{protocol,exec,discovery,bridge}`
- Copy + modify: `runtime/basic/internal/manifest/manifest.go` → `cores/regular/internal/manifest/manifest.go`
- Copy + modify: `runtime/basic/internal/manifest/manifest_test.go` → `cores/regular/internal/manifest/manifest_test.go`

**Interfaces:**
- Produces: `manifest.Core{Name string; Version string; CoreAPI int; Protocol int; Links []string}` (no `Provides`), `manifest.Script{...}` (unchanged), `manifest.LoadCore(path) (Core,error)`, `manifest.LoadScript(path) (Script,error)`. No `Runtime`/`LoadRuntime`.
- Produces (unchanged, vendored): `protocol.*`, `exec.Run`/`exec.Spec`/`exec.Result`/`exec.Event`/`exec.LogLine`, `discovery.Scan`/`discovery.Script`, `bridge.Serve`/`bridge.Deps`/`bridge.Script`/`bridge.Command`/`bridge.Event`.

- [ ] **Step 1: Create `cores/regular/go.mod`**

```
module wire-auto/cores/regular

go 1.26.4

require github.com/BurntSushi/toml v1.6.0
```

- [ ] **Step 2: Copy the four verbatim packages and rewrite imports**

Run from repo root (git-bash):
```bash
mkdir -p cores/regular/internal
cp -r runtime/basic/internal/protocol  cores/regular/internal/protocol
cp -r runtime/basic/internal/exec      cores/regular/internal/exec
cp -r runtime/basic/internal/discovery cores/regular/internal/discovery
cp -r runtime/basic/internal/bridge    cores/regular/internal/bridge
find cores/regular/internal -name '*.go' -exec sed -i 's|wire-auto/runtime/basic|wire-auto/cores/regular|g' {} +
```
Do NOT copy `registry` — it is removed in this architecture.

- [ ] **Step 3: Write the slimmed `cores/regular/internal/manifest/manifest.go`**

`Runtime`/`LoadRuntime` deleted; `Provides` removed from `Core`.
```go
// Package manifest читает и валидирует два паспорта платформы: core и script.
// Формат — TOML. runtime.manifest упразднён: рантайм — встроенная библиотека
// бандла, а не отдельный паспорт.
package manifest

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// Core — контракт самого бандла. Обязательны: Name, CoreAPI, Protocol, Links.
// Optional: Version (информационно).
type Core struct {
	Name     string   `toml:"name"`
	Version  string   `toml:"version"`
	CoreAPI  int      `toml:"coreApi"`
	Protocol int      `toml:"protocol"`
	Links    []string `toml:"links"`
}

// Script — пользовательский скрипт. Обязательны: Name, Core, CoreAPI, Link, Cmd.
// Optional: Version, Language (метаданные для бейджа клиента).
type Script struct {
	Name         string   `toml:"name"`
	Version      string   `toml:"version"`
	Core         string   `toml:"core"`
	CoreAPI      int      `toml:"coreApi"`
	Link         string   `toml:"link"`
	Cmd          []string `toml:"cmd"`
	Language     string   `toml:"language"`
	Capabilities []string `toml:"capabilities"`
}

func LoadCore(path string) (Core, error) {
	var c Core
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return Core{}, fmt.Errorf("read core manifest %s: %w", path, err)
	}
	if c.Name == "" || c.CoreAPI == 0 || c.Protocol == 0 || len(c.Links) == 0 {
		return Core{}, fmt.Errorf("core manifest %s: name, coreApi, protocol, links are required", path)
	}
	return c, nil
}

func LoadScript(path string) (Script, error) {
	var s Script
	if _, err := toml.DecodeFile(path, &s); err != nil {
		return Script{}, fmt.Errorf("read script manifest %s: %w", path, err)
	}
	if s.Name == "" || s.Core == "" || s.CoreAPI == 0 || s.Link == "" || len(s.Cmd) == 0 {
		return Script{}, fmt.Errorf("script manifest %s: name, core, coreApi, link, cmd are required", path)
	}
	return s, nil
}
```

- [ ] **Step 4: Write the slimmed `cores/regular/internal/manifest/manifest_test.go`**

Runtime tests removed; `provides` lines dropped from core TOML.
```go
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
```

- [ ] **Step 5: Tidy and build the copied packages**

Run: `cd cores/regular && go mod tidy && go build ./internal/... && go vet ./internal/...`
Expected: `go mod tidy` fetches `github.com/BurntSushi/toml v1.6.0` and writes `go.sum`; build and vet clean, exit 0.

- [ ] **Step 6: Run the package tests**

Run: `cd cores/regular && go test ./internal/...`
Expected: PASS across `protocol`, `exec`, `discovery`, `bridge`, `manifest` (exec/bridge e2e-ish tests need `python` on PATH). (STOP — no git.)

---

### Task 2: Handshake (two-passport signature)

**Files:**
- Create + modify: `cores/regular/internal/handshake/handshake.go`
- Create + modify: `cores/regular/internal/handshake/handshake_test.go`

**Interfaces:**
- Consumes: `manifest.Core`, `manifest.Script` (Task 1).
- Produces: `handshake.Reconciled{Protocol int; CoreAPI int}`, `handshake.Error{Code, Message string}` (implements `error`), `handshake.Reconcile(core manifest.Core, scr manifest.Script, provides []string) (Reconciled, error)`.

- [ ] **Step 1: Write `cores/regular/internal/handshake/handshake.go`**

Fork of basic's handshake: drop the `rt manifest.Runtime` parameter and the `PROTOCOL_UNSUPPORTED` check; capability check now reads the passed `provides` slice.
```go
// Package handshake сводит два паспорта (core и script) перед запуском.
// Первое несведение — стоп с машиночитаемым кодом. runtime.manifest упразднён;
// протокол ядра — свойство самого бандла.
package handshake

import "wire-auto/cores/regular/internal/manifest"

type Reconciled struct {
	Protocol int
	CoreAPI  int
}

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func contains[T comparable](xs []T, v T) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// Reconcile сводит контракт бандла (core) со скриптом. provides — авторизованные
// capability бандла (для regular пусто); capability скрипта обязаны в нём лежать.
func Reconcile(core manifest.Core, scr manifest.Script, provides []string) (Reconciled, error) {
	if scr.Core != core.Name {
		return Reconciled{}, &Error{"UNKNOWN_CORE", "script targets core " + scr.Core + " which is not this core"}
	}
	if scr.CoreAPI != core.CoreAPI {
		return Reconciled{}, &Error{"CORE_API_MISMATCH", "script coreApi does not match core"}
	}
	for _, cap := range scr.Capabilities {
		if !contains(provides, cap) {
			return Reconciled{}, &Error{"CAPABILITY_DENIED", "core does not provide capability " + cap}
		}
	}
	if !contains(core.Links, scr.Link) {
		return Reconciled{}, &Error{"LINK_UNSUPPORTED", "core does not support link " + scr.Link}
	}
	return Reconciled{Protocol: core.Protocol, CoreAPI: core.CoreAPI}, nil
}
```

- [ ] **Step 2: Write `cores/regular/internal/handshake/handshake_test.go`**

`base()` drops the runtime; the runtime-only tests (`ProtocolUnsupported`, `UnknownCoreWrongObject`) are gone.
```go
package handshake

import (
	"errors"
	"testing"

	"wire-auto/cores/regular/internal/manifest"
)

func base() (manifest.Core, manifest.Script) {
	core := manifest.Core{Name: "regular", CoreAPI: 1, Protocol: 1, Links: []string{"stdio"}}
	scr := manifest.Script{Name: "hello", Core: "regular", CoreAPI: 1, Link: "stdio", Cmd: []string{"python", "main.py"}, Language: "python", Capabilities: []string{}}
	return core, scr
}

func TestReconcileOK(t *testing.T) {
	core, scr := base()
	got, err := Reconcile(core, scr, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Protocol != 1 || got.CoreAPI != 1 {
		t.Fatalf("bad reconciled: %+v", got)
	}
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("expected *Error, got %v", err)
	}
	return e.Code
}

func TestReconcileUnknownCore(t *testing.T) {
	core, scr := base()
	scr.Core = "weird"
	_, err := Reconcile(core, scr, nil)
	if got := codeOf(t, err); got != "UNKNOWN_CORE" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileCoreAPIMismatch(t *testing.T) {
	core, scr := base()
	scr.CoreAPI = 2
	_, err := Reconcile(core, scr, nil)
	if got := codeOf(t, err); got != "CORE_API_MISMATCH" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileCapabilityDenied(t *testing.T) {
	core, scr := base()
	scr.Capabilities = []string{"serial"}
	_, err := Reconcile(core, scr, nil)
	if got := codeOf(t, err); got != "CAPABILITY_DENIED" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileLinkUnsupported(t *testing.T) {
	core, scr := base()
	scr.Link = "weird"
	_, err := Reconcile(core, scr, nil)
	if got := codeOf(t, err); got != "LINK_UNSUPPORTED" {
		t.Fatalf("got %s", got)
	}
}
```

- [ ] **Step 3: Run the handshake tests**

Run: `cd cores/regular && go test ./internal/handshake/ -v`
Expected: all five tests PASS. (STOP — no git.)

---

### Task 3: `cmd/core` streaming bridge, slim core.manifest, e2e

**Files:**
- Modify: `cores/regular/internal/discovery/discovery.go` (add `Core` field)
- Create: `cores/regular/core.manifest`
- Create: `cores/regular/cmd/core/main.go`
- Copy: `runtime/basic/cmd/wire/testdata` → `cores/regular/cmd/core/testdata`
- Create: `cores/regular/cmd/core/main_test.go`

**Interfaces:**
- Consumes: `manifest`, `handshake`, `discovery`, `bridge`, `exec` (Tasks 1–2).
- Produces: `run(coreManifestPath, scriptDir string) (exec.Result, error)`, `runStreaming(ctx, coreManifestPath, scriptDir string, onEvent func(exec.Event)) (exec.Result, error)`.

- [ ] **Step 1: Add a `Core` field to `discovery.Script` so the bridge can filter by core**

In `cores/regular/internal/discovery/discovery.go`, add `Core string` to the `Script` struct and populate it. The struct becomes:
```go
// Script — сводка одного скрипта для каталога клиента.
type Script struct {
	Name         string
	Dir          string
	Core         string
	Language     string
	Version      string
	Capabilities []string
}
```
And in the `out = append(out, Script{...})` literal inside `Scan`, add the `Core` field:
```go
		out = append(out, Script{
			Name:         s.Name,
			Dir:          abs,
			Core:         s.Core,
			Language:     s.Language,
			Version:      s.Version,
			Capabilities: caps,
		})
```

- [ ] **Step 2: Write `cores/regular/core.manifest`**

```toml
name = "regular"
version = "0.1.0"
coreApi = 1
protocol = 1
links = ["stdio"]
```

- [ ] **Step 3: Write `cores/regular/cmd/core/main.go`**

Fork of `basic/cmd/wire/main.go`: no `LoadRuntime`, no `registry.Discover`. The bundle loads its own `core.manifest`; routing is a single `scr.Core == core.Name` check (via `handshake.Reconcile`); `list` is filtered to this core's scripts.
```go
// Command core — стриминговый мост бандла regular: приложение шлёт команды в
// stdin, бандл стримит события в stdout (JSON Lines). Живёт до exit/EOF,
// обслуживая запуск за запуском. Рантайм встроен; отдельного admission нет —
// бандл знает своё ядро из собственного core.manifest.
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"time"

	"wire-auto/cores/regular/internal/bridge"
	"wire-auto/cores/regular/internal/discovery"
	"wire-auto/cores/regular/internal/exec"
	"wire-auto/cores/regular/internal/handshake"
	"wire-auto/cores/regular/internal/manifest"
)

// runStreaming выполняет один прогон: load core.manifest → route (только своё
// ядро) → handshake → spawn → pump, прокидывая ctx и onEvent в exec.Run.
func runStreaming(ctx context.Context, coreManifestPath, scriptDir string, onEvent func(exec.Event)) (exec.Result, error) {
	core, err := manifest.LoadCore(coreManifestPath)
	if err != nil {
		return exec.Result{}, err
	}
	scr, err := manifest.LoadScript(filepath.Join(scriptDir, "script.manifest"))
	if err != nil {
		return exec.Result{}, err
	}

	// regular ничего не provides; авторизованное множество пусто.
	reconciled, err := handshake.Reconcile(core, scr, nil)
	if err != nil {
		var he *handshake.Error
		if errors.As(err, &he) {
			return exec.Result{Status: "HANDSHAKE_FAILED", ErrorCode: he.Code, ErrorMessage: he.Message, Logs: []exec.LogLine{}}, nil
		}
		return exec.Result{}, err
	}

	absCoreDir, err := filepath.Abs(filepath.Dir(coreManifestPath))
	if err != nil {
		return exec.Result{}, err
	}
	sdkDir := filepath.Join(absCoreDir, "sdk")

	spec := exec.Spec{
		Dir:            scriptDir,
		Command:        scr.Cmd[0],
		Args:           scr.Cmd[1:],
		Env:            []string{"WIRE_SDK_DIR=" + sdkDir},
		Protocol:       reconciled.Protocol,
		CoreAPI:        reconciled.CoreAPI,
		ScriptArgs:     []string{},
		StartupTimeout: 10 * time.Second,
		RunTimeout:     60 * time.Second,
		CancelGrace:    2 * time.Second,
		OnEvent:        onEvent,
	}
	return exec.Run(ctx, spec), nil
}

// run — тонкая обёртка без стриминга (используется e2e-тестами).
func run(coreManifestPath, scriptDir string) (exec.Result, error) {
	return runStreaming(context.Background(), coreManifestPath, scriptDir, nil)
}

func main() {
	coreManifest := flag.String("core", "cores/regular/core.manifest", "path to this core's manifest")
	scriptsDir := flag.String("scripts", "scripts", "path to the scripts directory")
	flag.Parse()

	core, err := manifest.LoadCore(*coreManifest)
	if err != nil {
		os.Exit(1)
	}

	deps := bridge.Deps{
		List: func() ([]bridge.Script, error) {
			found, err := discovery.Scan(*scriptsDir)
			if err != nil {
				return nil, err
			}
			out := make([]bridge.Script, 0, len(found))
			for _, s := range found {
				if s.Core != core.Name {
					continue // чужое ядро — этот бинарь его не запустит
				}
				out = append(out, bridge.Script{
					Name:         s.Name,
					Dir:          s.Dir,
					Language:     s.Language,
					Version:      s.Version,
					Capabilities: s.Capabilities,
				})
			}
			return out, nil
		},
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) {
			return runStreaming(ctx, *coreManifest, dir, onEvent)
		},
	}

	if err := bridge.Serve(os.Stdin, os.Stdout, deps); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Copy the e2e testdata fixtures**

Run from repo root:
```bash
mkdir -p cores/regular/cmd/core
cp -r runtime/basic/cmd/wire/testdata cores/regular/cmd/core/testdata
```
(`testdata/needs-serial` = `core="regular"`, `capabilities=["serial"]` → `CAPABILITY_DENIED`; `testdata/needs-ghost` = `core="ghost"` → `UNKNOWN_CORE`. No manifest changes needed.)

- [ ] **Step 5: Write `cores/regular/cmd/core/main_test.go`**

`run`/`runStreaming` now take the core manifest path (not a runtime manifest + cores dir). `repoRoot` climbs four levels from `cores/regular/cmd/core`.
```go
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
```

- [ ] **Step 6: Full module gate**

Run: `cd cores/regular && go build ./... && go vet ./... && go test ./...`
Expected: build + vet clean; all tests PASS (requires `python` on PATH). (STOP — no git.)

---

### Task 4: Repoint deview at `cores/regular/cmd/core`

**Files:**
- Modify: `apps/deview/cmd/deview/main.go` (two lines: default help text + dev spawn)

**Interfaces:**
- Consumes: the `cores/regular/cmd/core` binary over the unchanged bridge protocol. No Go import — deview spawns it as a subprocess.

- [ ] **Step 1: Update the `-wire` flag help text**

In `apps/deview/cmd/deview/main.go`, change the flag default help string:
```go
	wireBin := flag.String("wire", "", "path to the wire bridge binary (default: go run ./cores/regular/cmd/core)")
```

- [ ] **Step 2: Update the dev-default spawn in `startBridge`**

Change the final return of `startBridge`:
```go
	// dev-умолчание: запускать из корня репозитория.
	return bridge.NewProcessTransport("go", "run", "./cores/regular/cmd/core")
```

- [ ] **Step 3: Build deview**

Run: `cd apps/deview && go build ./... && go vet ./...`
Expected: clean, exit 0.

- [ ] **Step 4: Manual smoke test of deview against the new bundle**

Run from repo root: `go run ./apps/deview/cmd/deview`
Expected: the menu lists `hello` (regular script). Type `1`, press Enter → prints `⏳ выполняется…`, a `hello from python` log line, and an OK result. Type `q` to quit. (STOP — no git.)

---

## PART B — `cores/duplex` bundle (protocol 2, `env.get`)

### Task 5: Scaffold module; vendor protocol/exec/capreg; slim manifest + handshake

**Files:**
- Create: `cores/duplex/go.mod`
- Copy (verbatim + import rewrite): `runtime/duplex/internal/{protocol,exec,capreg}` → `cores/duplex/internal/{protocol,exec,capreg}`
- Create + modify: `cores/duplex/internal/manifest/manifest.go`
- Create + modify: `cores/duplex/internal/manifest/manifest_test.go`
- Create + modify: `cores/duplex/internal/handshake/handshake.go`
- Create + modify: `cores/duplex/internal/handshake/handshake_test.go`

**Interfaces:**
- Produces: `manifest.Core{Name;Version;CoreAPI;Protocol;Links}` (no `Provides`), `manifest.Script{...}`, `manifest.LoadCore`, `manifest.LoadScript`.
- Produces: `handshake.Reconciled{Protocol int; CoreAPI int; Provides []string}`, `handshake.Error`, `handshake.Reconcile(core manifest.Core, scr manifest.Script, provides []string) (Reconciled, error)`.
- Produces (unchanged, vendored): `protocol.*` (incl. `TypeRequest`/`TypeResponse` + `ID`/`Capability`/`Params`/`Code`), `exec.Run`/`exec.Spec` (with `Provides`/`Registry`), `capreg.Handler`/`capreg.Default` (`env.get`).

- [ ] **Step 1: Create `cores/duplex/go.mod`**

```
module wire-auto/cores/duplex

go 1.26.4

require github.com/BurntSushi/toml v1.6.0
```

- [ ] **Step 2: Copy the three verbatim packages and rewrite imports**

Run from repo root:
```bash
mkdir -p cores/duplex/internal
cp -r runtime/duplex/internal/protocol cores/duplex/internal/protocol
cp -r runtime/duplex/internal/exec     cores/duplex/internal/exec
cp -r runtime/duplex/internal/capreg   cores/duplex/internal/capreg
find cores/duplex/internal -name '*.go' -exec sed -i 's|wire-auto/runtime/duplex|wire-auto/cores/duplex|g' {} +
```
Do NOT copy `registry`. `exec/testdata/{env-echo,plain}/main.py` are copied along with the `exec` tree — keep them.

- [ ] **Step 3: Write `cores/duplex/internal/manifest/manifest.go`**

Identical slimmed manifest as the regular bundle (no internal imports, so byte-identical):
```go
// Package manifest читает и валидирует два паспорта платформы: core и script.
// Формат — TOML. runtime.manifest упразднён: рантайм — встроенная библиотека
// бандла, а не отдельный паспорт.
package manifest

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

// Core — контракт самого бандла. Обязательны: Name, CoreAPI, Protocol, Links.
type Core struct {
	Name     string   `toml:"name"`
	Version  string   `toml:"version"`
	CoreAPI  int      `toml:"coreApi"`
	Protocol int      `toml:"protocol"`
	Links    []string `toml:"links"`
}

// Script — пользовательский скрипт. Обязательны: Name, Core, CoreAPI, Link, Cmd.
type Script struct {
	Name         string   `toml:"name"`
	Version      string   `toml:"version"`
	Core         string   `toml:"core"`
	CoreAPI      int      `toml:"coreApi"`
	Link         string   `toml:"link"`
	Cmd          []string `toml:"cmd"`
	Language     string   `toml:"language"`
	Capabilities []string `toml:"capabilities"`
}

func LoadCore(path string) (Core, error) {
	var c Core
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return Core{}, fmt.Errorf("read core manifest %s: %w", path, err)
	}
	if c.Name == "" || c.CoreAPI == 0 || c.Protocol == 0 || len(c.Links) == 0 {
		return Core{}, fmt.Errorf("core manifest %s: name, coreApi, protocol, links are required", path)
	}
	return c, nil
}

func LoadScript(path string) (Script, error) {
	var s Script
	if _, err := toml.DecodeFile(path, &s); err != nil {
		return Script{}, fmt.Errorf("read script manifest %s: %w", path, err)
	}
	if s.Name == "" || s.Core == "" || s.CoreAPI == 0 || s.Link == "" || len(s.Cmd) == 0 {
		return Script{}, fmt.Errorf("script manifest %s: name, core, coreApi, link, cmd are required", path)
	}
	return s, nil
}
```

- [ ] **Step 4: Write `cores/duplex/internal/manifest/manifest_test.go`**

No `runtime.manifest`, no `provides`.
```go
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
```

- [ ] **Step 5: Write `cores/duplex/internal/handshake/handshake.go`**

Same two-passport signature as regular, but `Reconciled` carries `Provides`.
```go
// Package handshake сводит два паспорта (core и script) перед запуском.
// Первое несведение — стоп с машиночитаемым кодом. runtime.manifest упразднён.
package handshake

import "wire-auto/cores/duplex/internal/manifest"

type Reconciled struct {
	Protocol int
	CoreAPI  int
	Provides []string
}

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func contains[T comparable](xs []T, v T) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// Reconcile сводит контракт бандла (core) со скриптом. provides подаёт
// вызывающий (ключи capreg-реестра); capability скрипта обязаны в нём лежать.
func Reconcile(core manifest.Core, scr manifest.Script, provides []string) (Reconciled, error) {
	if scr.Core != core.Name {
		return Reconciled{}, &Error{"UNKNOWN_CORE", "script targets core " + scr.Core + " which is not this core"}
	}
	if scr.CoreAPI != core.CoreAPI {
		return Reconciled{}, &Error{"CORE_API_MISMATCH", "script coreApi does not match core"}
	}
	for _, cap := range scr.Capabilities {
		if !contains(provides, cap) {
			return Reconciled{}, &Error{"CAPABILITY_DENIED", "core does not provide capability " + cap}
		}
	}
	if !contains(core.Links, scr.Link) {
		return Reconciled{}, &Error{"LINK_UNSUPPORTED", "core does not support link " + scr.Link}
	}
	return Reconciled{Protocol: core.Protocol, CoreAPI: core.CoreAPI, Provides: provides}, nil
}
```

- [ ] **Step 6: Write `cores/duplex/internal/handshake/handshake_test.go`**

```go
package handshake

import (
	"errors"
	"testing"

	"wire-auto/cores/duplex/internal/manifest"
)

func duplexCore() manifest.Core {
	return manifest.Core{Name: "duplex", CoreAPI: 1, Protocol: 2, Links: []string{"stdio"}}
}
func duplexScript() manifest.Script {
	return manifest.Script{Name: "s", Core: "duplex", CoreAPI: 1, Link: "stdio", Cmd: []string{"python", "main.py"}, Capabilities: []string{"env.get"}}
}

func TestReconcileCarriesProvides(t *testing.T) {
	r, err := Reconcile(duplexCore(), duplexScript(), []string{"env.get"})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if r.Protocol != 2 {
		t.Fatalf("protocol = %d, want 2", r.Protocol)
	}
	if len(r.Provides) != 1 || r.Provides[0] != "env.get" {
		t.Fatalf("provides = %v, want [env.get]", r.Provides)
	}
}

func TestReconcileCapabilityDenied(t *testing.T) {
	_, err := Reconcile(duplexCore(), duplexScript(), []string{})
	var he *Error
	if !errors.As(err, &he) || he.Code != "CAPABILITY_DENIED" {
		t.Fatalf("err = %v, want CAPABILITY_DENIED", err)
	}
}
```

- [ ] **Step 7: Tidy, build, vet, test the module internals**

Run: `cd cores/duplex && go mod tidy && go build ./internal/... && go vet ./internal/... && go test ./internal/...`
Expected: `go.sum` written; build/vet clean; tests PASS across `protocol`, `exec` (needs `python`), `capreg`, `manifest`, `handshake`. The vendored `exec_test.go` (`TestRequestResponseHappyPath`, `TestRequestCapabilityDenied`, `TestRequestOnProtocol1IsViolation`, `TestPlainV1FlowStillWorks`) runs unchanged. (STOP — no git.)

---

### Task 6: `cmd/core` single-shot runner, slim core.manifest, e2e

**Files:**
- Create: `cores/duplex/core.manifest`
- Create: `cores/duplex/cmd/core/main.go`
- Create: `cores/duplex/cmd/core/main_test.go`

**Interfaces:**
- Consumes: `manifest`, `handshake`, `exec`, `capreg` (Task 5).
- Produces: `run(coreManifestPath, scriptDir string) (exec.Result, error)`, `providesFromRegistry(reg map[string]capreg.Handler) []string`.

- [ ] **Step 1: Write `cores/duplex/core.manifest`**

No `provides` (source of truth is the capreg registry).
```toml
name = "duplex"
version = "0.1.0"
coreApi = 1
protocol = 2
links = ["stdio"]
```

- [ ] **Step 2: Write `cores/duplex/cmd/core/main.go`**

Fork of `duplex/cmd/wire/main.go`: no `LoadRuntime`, no `registry.Discover`. Loads its own `core.manifest`; `provides` derives from `capreg.Default` keys and feeds both the handshake and the exec spec.
```go
// Command core — одноразовый прогонщик бандла duplex: встроенный рантайм
// запускает один скрипт по пути (load core.manifest→handshake→spawn→pump) и
// печатает исход. Двусторонний канал request/response обслуживает capreg-реестр;
// множество provides выводится из его ключей.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"wire-auto/cores/duplex/internal/capreg"
	"wire-auto/cores/duplex/internal/exec"
	"wire-auto/cores/duplex/internal/handshake"
	"wire-auto/cores/duplex/internal/manifest"
)

// providesFromRegistry — авторизованное множество capability бандла: ключи
// реестра хендлеров (код — источник правды, не манифест). Сортировка ради
// детерминизма.
func providesFromRegistry(reg map[string]capreg.Handler) []string {
	out := make([]string, 0, len(reg))
	for k := range reg {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func run(coreManifestPath, scriptDir string) (exec.Result, error) {
	core, err := manifest.LoadCore(coreManifestPath)
	if err != nil {
		return exec.Result{}, err
	}
	scr, err := manifest.LoadScript(filepath.Join(scriptDir, "script.manifest"))
	if err != nil {
		return exec.Result{}, err
	}

	provides := providesFromRegistry(capreg.Default)
	reconciled, err := handshake.Reconcile(core, scr, provides)
	if err != nil {
		var he *handshake.Error
		if errors.As(err, &he) {
			return exec.Result{Status: "HANDSHAKE_FAILED", ErrorCode: he.Code, ErrorMessage: he.Message, Logs: []exec.LogLine{}}, nil
		}
		return exec.Result{}, err
	}

	absCoreDir, err := filepath.Abs(filepath.Dir(coreManifestPath))
	if err != nil {
		return exec.Result{}, err
	}
	sdkDir := filepath.Join(absCoreDir, "sdk")

	spec := exec.Spec{
		Dir:            scriptDir,
		Command:        scr.Cmd[0],
		Args:           scr.Cmd[1:],
		Env:            []string{"WIRE_SDK_DIR=" + sdkDir},
		Protocol:       reconciled.Protocol,
		CoreAPI:        reconciled.CoreAPI,
		ScriptArgs:     []string{},
		StartupTimeout: 10 * time.Second,
		RunTimeout:     60 * time.Second,
		CancelGrace:    2 * time.Second,
		Provides:       reconciled.Provides,
		Registry:       capreg.Default,
	}
	return exec.Run(context.Background(), spec), nil
}

func main() {
	coreManifest := flag.String("core", "cores/duplex/core.manifest", "path to this core's manifest")
	scriptDir := flag.String("script", "", "path to the script directory to run")
	flag.Parse()

	if *scriptDir == "" {
		fmt.Fprintln(os.Stderr, "usage: core -script <dir> [-core manifest]")
		os.Exit(2)
	}

	res, err := run(*coreManifest, *scriptDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	fmt.Printf("Status: %s\n", res.Status)
	if res.ErrorCode != "" {
		fmt.Printf("Error:  %s %s\n", res.ErrorCode, res.ErrorMessage)
	}
	for _, l := range res.Logs {
		fmt.Printf("[%s] %s\n", l.Level, l.Message)
	}
	if len(res.Result) > 0 {
		fmt.Printf("Result: %s\n", res.Result)
	}
	if res.Status != exec.StatusOK {
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Write `cores/duplex/cmd/core/main_test.go`**

Only the `run(coreManifest, scriptDir)` signature; the cross-core `regular-on-duplex` e2e is removed (no admission — a `core="regular"` script would now be `UNKNOWN_CORE`; one-way flow is already covered by the vendored `exec.TestPlainV1FlowStillWorks`).
```go
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
```

- [ ] **Step 4: Full module gate**

Run: `cd cores/duplex && go build ./... && go vet ./... && go test ./...`
Expected: build + vet clean; all tests PASS (`TestE2EEnvReport` needs `python`, `TestE2EEnvReportNode` needs `node`). If `node` is missing, that is an environment gap — report it, do not delete the test.

- [ ] **Step 5: Manual smoke run from repo root**

Run: `cd D:/Projects/wire-auto && go run ./cores/duplex/cmd/core -script scripts/examples/env-report`
Expected: prints `Status: OK`, a `[info] USER=...` line, and `Result: {"user":"..."}`. (STOP — no git.)

---

## PART C — cleanup

### Task 7: Delete `runtime/`; verify all modules independently

**Files:**
- Delete: `runtime/basic/`, `runtime/duplex/` (the whole `runtime/` tree)

**Interfaces:** none.

- [ ] **Step 1: Confirm nothing in code still imports the old modules**

Run from repo root: `grep -rn "wire-auto/runtime/" --include=*.go cores apps`
Expected: no matches. (If any appear, fix the import before deleting.)

- [ ] **Step 2: Delete the runtime tree**

Run from repo root: `rm -rf runtime`
Expected: `runtime/` gone. No `runtime.manifest` remains anywhere: `find . -name runtime.manifest` prints nothing.

- [ ] **Step 3: Verify all three modules build, vet, and test independently**

Run:
```bash
cd D:/Projects/wire-auto/cores/regular && go build ./... && go vet ./... && go test ./...
cd D:/Projects/wire-auto/cores/duplex  && go build ./... && go vet ./... && go test ./...
cd D:/Projects/wire-auto/apps/deview   && go build ./... && go vet ./... && go test ./...
```
Expected: each module green on its own. No root `go.work` exists: `ls D:/Projects/wire-auto/go.work` → not found. (STOP — no git.)

---

### Task 8: Update `guide/` docs to the merged architecture

**Files:**
- Modify (prose): the `guide/demo/` topic files that describe the old runtime↔core split.

**Interfaces:** none (documentation only). Per CLAUDE.md, keep `guide/` as many small topic files; `guide/README.md` stays an index only.

Apply these conceptual changes consistently wherever they appear:
- There is **no separate runtime**. A `core` is a self-contained bundle (`cores/<name>`) that embeds the runtime as a library. "Run the runtime" language is wrong.
- There are **two handshakes**, not three: app↔core and core↔script. The runtime↔core admission handshake and `runtime.manifest` no longer exist.
- Manifests are now two: `core.manifest` (name/version/coreApi/protocol/links — no `provides`) and `script.manifest` (unchanged). Capability authorization derives from the core's capability registry, not a manifest field.
- The regular bundle's binary is `cores/regular/cmd/core` (streaming bridge; deview's backend). The duplex bundle's binary is `cores/duplex/cmd/core` (single-shot).
- deview spawns `go run ./cores/regular/cmd/core`.

- [ ] **Step 1: Enumerate stale references**

Run from repo root: `grep -rln "runtime/basic\|runtime/duplex\|runtime.manifest\|admission" guide`
Expected list includes at least: `guide/demo/01-overview.md`, `guide/demo/02-repo-and-modules.md`, `guide/demo/cores-and-runtimes.md`, `guide/demo/manifests.md`, `guide/demo/handshake.md`, `guide/demo/lifecycle.md`, `guide/demo/running.md`, `guide/demo/apps-deview.md`, `guide/demo/app-runtime-bridge.md`, `guide/demo/README.md`.

- [ ] **Step 2: Update each file**

Read each listed file and rewrite the affected passages per the conceptual changes above. Notable renames:
- `guide/demo/cores-and-runtimes.md` → its content should describe **bundles** (one file, one responsibility per CLAUDE.md). If the split framing is now wrong end-to-end, rename the file to `guide/demo/cores.md` and update links to it from `guide/demo/README.md` and any file that references it.
- `guide/demo/app-runtime-bridge.md` describes the app↔core bridge — keep it, but retitle "runtime" → "core/bundle".
- `guide/demo/manifests.md` — drop `runtime.manifest`; document the two remaining manifests and the removal of `provides`.
- `guide/demo/handshake.md` — two handshakes; admission removed.

- [ ] **Step 3: Verify no stale references remain**

Run: `grep -rn "runtime/basic\|runtime/duplex\|runtime.manifest" guide`
Expected: no matches. References to "runtime" as *the embedded library concept* are fine; references to it as a separate module/process/manifest are not. (STOP — no git.)

---

## Self-Review

**Spec coverage:**
- §3 layout → Tasks 1–3 (regular module), 5–6 (duplex module), 7 (delete runtime). ✓
- §4 regular bundle (streaming bridge, list filter, own-core routing) → Task 3. ✓
- §5 duplex bundle (single-shot, provides from capreg, no admission, plain-v1 via exec test) → Tasks 5–6. ✓
- §6 handshake new signature + manifest slimming → Tasks 1, 2, 5. ✓
- §7 final manifests → Task 3 Step 2, Task 6 Step 1. ✓
- §8 migration order (regular → deview → duplex → cleanup) → Task order. ✓
- §9 done criteria (each module independent, deview manual, no runtime/, no go.work) → Task 7 Step 3 + Task 4 Step 4. ✓
- A′ deview repoint (one line) → Task 4. ✓
- guide update → Task 8. ✓

**Placeholder scan:** No TBD/TODO; every code step shows full content or an exact copy+rewrite command. ✓

**Type consistency:** `Reconcile(core, scr, provides)` used identically in Tasks 2/3 (regular) and 5/6 (duplex). `manifest.Core` has no `Provides` in both bundles; duplex `Reconciled` carries `Provides`, regular's does not (regular never needs it). `run`/`runStreaming` signatures match between the `main.go` and `main_test.go` of each bundle. `discovery.Script.Core` added in Task 3 Step 1 before use in Step 3. ✓

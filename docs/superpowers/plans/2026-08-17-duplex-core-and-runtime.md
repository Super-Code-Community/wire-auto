# Duplex Core + Runtime (v2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new core (`cores/duplex`) and runtime (`runtime/duplex`) that introduce a bidirectional `request`/`response` channel over stdio JSON Lines, making the core's `provides` a real authorization gate — without touching the frozen v1 (`cores/regular`, `runtime/basic`).

**Architecture:** `runtime/duplex` is a Go module forked from `runtime/basic`, extended so its exec loop dispatches script `request` messages to an in-runtime capability registry (variant A), gated by the reconciled core's `provides`. It speaks `protocols = [1, 2]` — a strict superset of basic — admitting both `regular` (v1, one-way) and `duplex` (v2, two-way). Scripts speak the protocol raw (zero-shim) in python and node.

**Tech Stack:** Go 1.26.4, `github.com/BurntSushi/toml`, stdio JSON Lines protocol, python3 + node available on PATH for tests.

## Global Constraints

- **NO git operations by any agent — ever.** No `commit`, `branch`, `checkout -b`, `switch -c`, `add`, `push`, `stash`. The user commits manually. Each task's deliverable is "its tests pass"; stop there.
- **Do not modify `runtime/basic/**` or `cores/regular/**`.** v1 is frozen. All new code lives under `runtime/duplex/`, `cores/duplex/`, and `scripts/examples/`.
- **No root `go.work`.** Every module builds independently: `cd runtime/duplex && go build ./... && go vet ./... && go test ./...` must pass on its own.
- Go module path: `wire-auto/runtime/duplex`. Go version line: `go 1.26.4`. Dependency: `github.com/BurntSushi/toml v1.6.0`.
- Protocol wire format: one message = one line of compact JSON (`json.Marshal`, no extra spaces) + `\n`. New `Message` fields are `omitempty` so the v1 byte stream is unchanged.
- WET is acceptable at this stage: `runtime/duplex/internal/*` is a deliberate fork of `runtime/basic/internal/*`. Do not extract shared code into `packages/`.
- All Go file operations by agents must use absolute Windows paths (e.g. `D:\Projects\wire-auto\runtime\duplex\go.mod`).

---

### Task 1: Module scaffold, manifests, and manifest package

**Files:**
- Create: `runtime/duplex/go.mod`
- Create: `runtime/duplex/runtime.manifest`
- Create: `cores/duplex/core.manifest`
- Create: `runtime/duplex/internal/manifest/manifest.go`
- Test: `runtime/duplex/internal/manifest/manifest_test.go`

**Interfaces:**
- Produces: `manifest.Runtime{Name string; Version string; Protocols []int; Transports []string; Cores []string}`, `manifest.Core{Name string; Version string; CoreAPI int; Protocol int; Provides []string; Links []string}`, `manifest.Script{Name string; Version string; Core string; CoreAPI int; Link string; Cmd []string; Language string; Capabilities []string}`.
- Produces: `manifest.LoadRuntime(path string) (Runtime, error)`, `manifest.LoadCore(path string) (Core, error)`, `manifest.LoadScript(path string) (Script, error)`.

- [ ] **Step 1: Create the module and manifest files**

`runtime/duplex/go.mod`:
```
module wire-auto/runtime/duplex

go 1.26.4

require github.com/BurntSushi/toml v1.6.0
```

`runtime/duplex/runtime.manifest`:
```toml
name = "wire-auto-runtime-duplex"
version = "0.1.0"
protocols = [1, 2]
transports = ["stdio"]
cores = ["duplex", "regular"]
```

`cores/duplex/core.manifest`:
```toml
name = "duplex"
version = "0.1.0"
coreApi = 1
protocol = 2
provides = ["env.get"]
links = ["stdio"]
```

- [ ] **Step 2: Write `runtime/duplex/internal/manifest/manifest.go`**

Verbatim fork of `runtime/basic/internal/manifest/manifest.go` (package `manifest`, no internal imports, so it copies cleanly):
```go
// Package manifest читает и валидирует три вида паспортов платформы:
// runtime, core и script. Формат — TOML.
package manifest

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type Runtime struct {
	Name       string   `toml:"name"`
	Version    string   `toml:"version"`
	Protocols  []int    `toml:"protocols"`
	Transports []string `toml:"transports"`
	Cores      []string `toml:"cores"`
}

type Core struct {
	Name     string   `toml:"name"`
	Version  string   `toml:"version"`
	CoreAPI  int      `toml:"coreApi"`
	Protocol int      `toml:"protocol"`
	Provides []string `toml:"provides"`
	Links    []string `toml:"links"`
}

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

func LoadRuntime(path string) (Runtime, error) {
	var r Runtime
	if _, err := toml.DecodeFile(path, &r); err != nil {
		return Runtime{}, fmt.Errorf("read runtime manifest %s: %w", path, err)
	}
	if r.Name == "" || len(r.Protocols) == 0 || len(r.Transports) == 0 {
		return Runtime{}, fmt.Errorf("runtime manifest %s: name, protocols, and transports are required", path)
	}
	return r, nil
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

- [ ] **Step 3: Write the failing test `runtime/duplex/internal/manifest/manifest_test.go`**

```go
package manifest

import "testing"

func TestLoadRuntimeDuplex(t *testing.T) {
	r, err := LoadRuntime("../../runtime.manifest")
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}
	if len(r.Protocols) != 2 || r.Protocols[0] != 1 || r.Protocols[1] != 2 {
		t.Fatalf("protocols = %v, want [1 2]", r.Protocols)
	}
}

func TestLoadCoreDuplexProvides(t *testing.T) {
	c, err := LoadCore("../../../../cores/duplex/core.manifest")
	if err != nil {
		t.Fatalf("load core: %v", err)
	}
	if c.Protocol != 2 {
		t.Fatalf("protocol = %d, want 2", c.Protocol)
	}
	if len(c.Provides) != 1 || c.Provides[0] != "env.get" {
		t.Fatalf("provides = %v, want [env.get]", c.Provides)
	}
}
```

- [ ] **Step 4: Run `go mod tidy` then the test**

Run: `cd runtime/duplex && go mod tidy && go test ./internal/manifest/ -v`
Expected: `go mod tidy` fetches `github.com/BurntSushi/toml v1.6.0` and writes `go.sum`; both tests PASS.

- [ ] **Step 5: Verify the whole module builds and vets**

Run: `cd runtime/duplex && go build ./... && go vet ./...`
Expected: no output, exit 0. (STOP here — no git.)

---

### Task 2: Protocol package with `request`/`response`

**Files:**
- Create: `runtime/duplex/internal/protocol/protocol.go`
- Test: `runtime/duplex/internal/protocol/protocol_test.go`

**Interfaces:**
- Produces constants: `protocol.TypeHello="hello"`, `TypeReady="ready"`, `TypeLog="log"`, `TypeDone="done"`, `TypeCancel="cancel"`, `TypeRequest="request"`, `TypeResponse="response"`.
- Produces: `protocol.Message` struct (v1 fields plus `ID string`, `Capability string`, `Params json.RawMessage`, `Code string`).
- Produces: `protocol.Encode(w io.Writer, m Message) error`, `protocol.NewDecoder(r io.Reader) *Decoder`, `(*Decoder).Next() (Message, error)` returning `io.EOF` at end of stream.

- [ ] **Step 1: Write the failing test `runtime/duplex/internal/protocol/protocol_test.go`**

```go
package protocol

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestRequestResponseRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	req := Message{Type: TypeRequest, ID: "1", Capability: "env.get", Params: json.RawMessage(`{"name":"USER"}`)}
	if err := Encode(&buf, req); err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec := NewDecoder(&buf)
	got, err := dec.Next()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Type != TypeRequest || got.ID != "1" || got.Capability != "env.get" {
		t.Fatalf("got %+v", got)
	}
	if string(got.Params) != `{"name":"USER"}` {
		t.Fatalf("params = %s", got.Params)
	}
}

func TestResponseErrorEncoding(t *testing.T) {
	var buf bytes.Buffer
	_ = Encode(&buf, Message{Type: TypeResponse, ID: "1", Code: "ENV_NOT_FOUND", Message: "env var not set: USER"})
	// v1 fields must stay omitempty: no "protocol":0, no "params" key here.
	line := buf.String()
	if want := `{"type":"response","message":"env var not set: USER","id":"1","code":"ENV_NOT_FOUND"}` + "\n"; line != want {
		t.Fatalf("line = %q, want %q", line, want)
	}
}

func TestDecoderEOF(t *testing.T) {
	dec := NewDecoder(bytes.NewReader(nil))
	if _, err := dec.Next(); err != io.EOF {
		t.Fatalf("err = %v, want EOF", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd runtime/duplex && go test ./internal/protocol/ -v`
Expected: FAIL — package/symbols do not exist yet.

- [ ] **Step 3: Write `runtime/duplex/internal/protocol/protocol.go`**

Fork of basic's protocol with the four new fields appended (field order matters for `TestResponseErrorEncoding`: new fields come after `Result`):
```go
// Package protocol описывает сообщения моста и кодек JSON Lines:
// одно сообщение = одна строка компактного JSON + '\n'.
package protocol

import (
	"bufio"
	"encoding/json"
	"io"
)

const (
	TypeHello    = "hello"
	TypeReady    = "ready"
	TypeLog      = "log"
	TypeDone     = "done"
	TypeCancel   = "cancel"
	TypeRequest  = "request"
	TypeResponse = "response"
)

// MaxLineBytes bounds a single protocol line to guard against adversarial input.
const MaxLineBytes = 1 << 20 // 1 MiB

type Message struct {
	Type     string          `json:"type"`
	Protocol int             `json:"protocol,omitempty"`
	CoreAPI  int             `json:"coreApi,omitempty"`
	Args     []string        `json:"args,omitempty"`
	Level    string          `json:"level,omitempty"`
	Message  string          `json:"message,omitempty"`
	ExitCode *int            `json:"exitCode,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
	// v2 additions
	ID         string          `json:"id,omitempty"`
	Capability string          `json:"capability,omitempty"`
	Params     json.RawMessage `json:"params,omitempty"`
	Code       string          `json:"code,omitempty"`
}

func Encode(w io.Writer, m Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

type Decoder struct {
	sc *bufio.Scanner
}

func NewDecoder(r io.Reader) *Decoder {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineBytes)
	return &Decoder{sc: sc}
}

// Next читает следующее сообщение. Возвращает io.EOF, когда поток кончился.
func (d *Decoder) Next() (Message, error) {
	for d.sc.Scan() {
		line := append([]byte(nil), d.sc.Bytes()...)
		if len(line) == 0 {
			continue
		}
		var m Message
		if err := json.Unmarshal(line, &m); err != nil {
			return Message{}, err
		}
		return m, nil
	}
	if err := d.sc.Err(); err != nil {
		return Message{}, err
	}
	return Message{}, io.EOF
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd runtime/duplex && go test ./internal/protocol/ -v`
Expected: all three tests PASS. If `TestResponseErrorEncoding` fails on field order, the JSON key order in the assertion reflects struct field order — keep struct fields exactly as shown above. (STOP — no git.)

---

### Task 3: Registry (admission) package

**Files:**
- Create: `runtime/duplex/internal/registry/registry.go`
- Test: `runtime/duplex/internal/registry/registry_test.go`

**Interfaces:**
- Consumes: `manifest.Runtime`, `manifest.Core` from Task 1.
- Produces: `registry.Admitted{Manifest manifest.Core; Dir string}`, `registry.Rejection{Name, Dir, Reason string}`.
- Produces: `registry.Compatible(rt manifest.Runtime, c manifest.Core) (bool, string)`, `registry.Discover(coresDir string, rt manifest.Runtime) (map[string]Admitted, []Rejection, error)`.

- [ ] **Step 1: Write `runtime/duplex/internal/registry/registry.go`**

Fork of basic's registry with the import path changed to the duplex module:
```go
// Package registry discovers cores and admits those whose contract is
// compatible with the runtime — polymorphism by contract, not by name.
package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"wire-auto/runtime/duplex/internal/manifest"
)

type Admitted struct {
	Manifest manifest.Core
	Dir      string
}

type Rejection struct {
	Name   string
	Dir    string
	Reason string
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func overlap(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func Compatible(rt manifest.Runtime, c manifest.Core) (bool, string) {
	if !containsInt(rt.Protocols, c.Protocol) {
		return false, fmt.Sprintf("core protocol %d not spoken by runtime %v", c.Protocol, rt.Protocols)
	}
	if !overlap(rt.Transports, c.Links) {
		return false, fmt.Sprintf("core links %v share no transport with runtime %v", c.Links, rt.Transports)
	}
	return true, ""
}

func Discover(coresDir string, rt manifest.Runtime) (map[string]Admitted, []Rejection, error) {
	entries, err := os.ReadDir(coresDir)
	if err != nil {
		return nil, nil, err
	}
	admitted := map[string]Admitted{}
	var rejected []Rejection
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(coresDir, e.Name())
		mpath := filepath.Join(dir, "core.manifest")
		if _, err := os.Stat(mpath); err != nil {
			continue
		}
		c, err := manifest.LoadCore(mpath)
		if err != nil {
			rejected = append(rejected, Rejection{Name: e.Name(), Dir: dir, Reason: "invalid manifest: " + err.Error()})
			continue
		}
		if ok, why := Compatible(rt, c); !ok {
			rejected = append(rejected, Rejection{Name: c.Name, Dir: dir, Reason: why})
			continue
		}
		admitted[c.Name] = Admitted{Manifest: c, Dir: dir}
	}
	return admitted, rejected, nil
}
```

- [ ] **Step 2: Write the failing test `runtime/duplex/internal/registry/registry_test.go`**

Builds a temp cores dir with a v1-style core (protocol 1) and a v2 core (protocol 2); the duplex runtime (`protocols=[1,2]`) must admit both.
```go
package registry

import (
	"os"
	"path/filepath"
	"testing"

	"wire-auto/runtime/duplex/internal/manifest"
)

func writeCore(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "core.manifest"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverAdmitsBothProtocols(t *testing.T) {
	root := t.TempDir()
	writeCore(t, filepath.Join(root, "regular"),
		"name=\"regular\"\nversion=\"0.1.0\"\ncoreApi=1\nprotocol=1\nlinks=[\"stdio\"]\n")
	writeCore(t, filepath.Join(root, "duplex"),
		"name=\"duplex\"\nversion=\"0.1.0\"\ncoreApi=1\nprotocol=2\nprovides=[\"env.get\"]\nlinks=[\"stdio\"]\n")

	rt := manifest.Runtime{Name: "rt", Protocols: []int{1, 2}, Transports: []string{"stdio"}}
	admitted, rejected, err := Discover(root, rt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected) != 0 {
		t.Fatalf("rejected = %+v, want none", rejected)
	}
	if _, ok := admitted["regular"]; !ok {
		t.Fatal("regular not admitted")
	}
	d, ok := admitted["duplex"]
	if !ok {
		t.Fatal("duplex not admitted")
	}
	if len(d.Manifest.Provides) != 1 || d.Manifest.Provides[0] != "env.get" {
		t.Fatalf("duplex provides = %v", d.Manifest.Provides)
	}
}
```

- [ ] **Step 3: Run the test**

Run: `cd runtime/duplex && go test ./internal/registry/ -v`
Expected: PASS.

- [ ] **Step 4: Vet**

Run: `cd runtime/duplex && go vet ./internal/registry/`
Expected: no output. (STOP — no git.)

---

### Task 4: Handshake package with `Provides`

**Files:**
- Create: `runtime/duplex/internal/handshake/handshake.go`
- Test: `runtime/duplex/internal/handshake/handshake_test.go`

**Interfaces:**
- Consumes: `manifest.Runtime`, `manifest.Core`, `manifest.Script` from Task 1.
- Produces: `handshake.Reconciled{Protocol int; CoreAPI int; Provides []string}`, `handshake.Error{Code, Message string}` (implements `error`), `handshake.Reconcile(rt manifest.Runtime, core manifest.Core, scr manifest.Script) (Reconciled, error)`.

- [ ] **Step 1: Write the failing test `runtime/duplex/internal/handshake/handshake_test.go`**

```go
package handshake

import (
	"errors"
	"testing"

	"wire-auto/runtime/duplex/internal/manifest"
)

func duplexCore() manifest.Core {
	return manifest.Core{Name: "duplex", CoreAPI: 1, Protocol: 2, Provides: []string{"env.get"}, Links: []string{"stdio"}}
}
func duplexRuntime() manifest.Runtime {
	return manifest.Runtime{Name: "rt", Protocols: []int{1, 2}, Transports: []string{"stdio"}}
}
func duplexScript() manifest.Script {
	return manifest.Script{Name: "s", Core: "duplex", CoreAPI: 1, Link: "stdio", Cmd: []string{"python", "main.py"}, Capabilities: []string{"env.get"}}
}

func TestReconcileCarriesProvides(t *testing.T) {
	r, err := Reconcile(duplexRuntime(), duplexCore(), duplexScript())
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
	scr := duplexScript()
	scr.Capabilities = []string{"serial"}
	_, err := Reconcile(duplexRuntime(), duplexCore(), scr)
	var he *Error
	if !errors.As(err, &he) || he.Code != "CAPABILITY_DENIED" {
		t.Fatalf("err = %v, want CAPABILITY_DENIED", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd runtime/duplex && go test ./internal/handshake/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write `runtime/duplex/internal/handshake/handshake.go`**

Fork of basic's handshake, with `Provides` added to `Reconciled` and populated on success:
```go
// Package handshake сводит три паспорта (runtime, core, script) перед запуском.
// Первое несведение — стоп с машиночитаемым кодом.
package handshake

import "wire-auto/runtime/duplex/internal/manifest"

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

func Reconcile(rt manifest.Runtime, core manifest.Core, scr manifest.Script) (Reconciled, error) {
	if scr.Core != core.Name {
		return Reconciled{}, &Error{"UNKNOWN_CORE", "script targets core " + scr.Core + " which is not the reconciled core"}
	}
	if scr.CoreAPI != core.CoreAPI {
		return Reconciled{}, &Error{"CORE_API_MISMATCH", "script coreApi does not match core"}
	}
	for _, cap := range scr.Capabilities {
		if !contains(core.Provides, cap) {
			return Reconciled{}, &Error{"CAPABILITY_DENIED", "core does not provide capability " + cap}
		}
	}
	if !contains(rt.Protocols, core.Protocol) {
		return Reconciled{}, &Error{"PROTOCOL_UNSUPPORTED", "runtime does not speak core protocol"}
	}
	if !contains(core.Links, scr.Link) {
		return Reconciled{}, &Error{"LINK_UNSUPPORTED", "core does not support link " + scr.Link}
	}
	return Reconciled{Protocol: core.Protocol, CoreAPI: core.CoreAPI, Provides: core.Provides}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd runtime/duplex && go test ./internal/handshake/ -v`
Expected: both tests PASS. (STOP — no git.)

---

### Task 5: Capability registry (`env.get`)

**Files:**
- Create: `runtime/duplex/internal/capreg/capreg.go`
- Test: `runtime/duplex/internal/capreg/capreg_test.go`

**Interfaces:**
- Produces: `capreg.Handler = func(params json.RawMessage) (result json.RawMessage, code string, err error)`.
- Produces: `capreg.Default map[string]Handler` (contains key `"env.get"`).

- [ ] **Step 1: Write the failing test `runtime/duplex/internal/capreg/capreg_test.go`**

```go
package capreg

import (
	"encoding/json"
	"testing"
)

func TestEnvGetFound(t *testing.T) {
	t.Setenv("WIRE_TEST_VAR", "hello")
	result, code, err := Default["env.get"](json.RawMessage(`{"name":"WIRE_TEST_VAR"}`))
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct{ Value string `json:"value"` }
	if e := json.Unmarshal(result, &out); e != nil {
		t.Fatal(e)
	}
	if out.Value != "hello" {
		t.Fatalf("value = %q, want hello", out.Value)
	}
}

func TestEnvGetNotFound(t *testing.T) {
	_, code, _ := Default["env.get"](json.RawMessage(`{"name":"WIRE_DEFINITELY_MISSING_XYZ"}`))
	if code != "ENV_NOT_FOUND" {
		t.Fatalf("code = %q, want ENV_NOT_FOUND", code)
	}
}

func TestEnvGetBadParams(t *testing.T) {
	_, code, _ := Default["env.get"](json.RawMessage(`{}`))
	if code != "BAD_PARAMS" {
		t.Fatalf("code = %q, want BAD_PARAMS", code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd runtime/duplex && go test ./internal/capreg/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Write `runtime/duplex/internal/capreg/capreg.go`**

```go
// Package capreg — реестр обработчиков capability для рантайма duplex.
// Обработчик исполняет уже авторизованный запрос (гейт provides — в exec).
package capreg

import (
	"encoding/json"
	"fmt"
	"os"
)

// Handler исполняет capability. Возвращает либо result (code==""), либо
// машиночитаемый code ошибки capability (+пояснение в err). Ошибка capability
// НЕ роняет прогон — она уезжает в response.code.
type Handler func(params json.RawMessage) (result json.RawMessage, code string, err error)

// Default — реестр v2. Ключи должны совпадать с core.provides.
var Default = map[string]Handler{
	"env.get": envGet,
}

func envGet(params json.RawMessage) (json.RawMessage, string, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, "BAD_PARAMS", fmt.Errorf("env.get requires a non-empty string field \"name\"")
	}
	v, ok := os.LookupEnv(p.Name)
	if !ok {
		return nil, "ENV_NOT_FOUND", fmt.Errorf("env var not set: %s", p.Name)
	}
	out, err := json.Marshal(map[string]string{"value": v})
	if err != nil {
		return nil, "BAD_PARAMS", err
	}
	return out, "", nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd runtime/duplex && go test ./internal/capreg/ -v`
Expected: all three tests PASS. (STOP — no git.)

---

### Task 6: Exec loop with request/response dispatch

**Files:**
- Create: `runtime/duplex/internal/exec/event.go`
- Create: `runtime/duplex/internal/exec/exec.go`
- Create: `runtime/duplex/internal/exec/testdata/env-echo/main.py`
- Test: `runtime/duplex/internal/exec/exec_test.go`

**Interfaces:**
- Consumes: `protocol` (Task 2), `capreg.Handler` (Task 5).
- Produces: `exec.Event{Kind, Level, Message string}`.
- Produces: `exec.Spec` with all basic fields plus `Provides []string` and `Registry map[string]capreg.Handler`.
- Produces: `exec.Result{Status string; ExitCode int; Logs []LogLine; ErrorCode, ErrorMessage string; Result json.RawMessage}`, `exec.LogLine{Level, Message string}`.
- Produces: `exec.Run(ctx context.Context, spec Spec) Result` and status constants (`StatusOK`, `StatusScriptError`, `StatusProtocolViolation`, `StatusStartupTimeout`, `StatusRunTimeout`, `StatusCrashed`, `StatusCancelled`).

- [ ] **Step 1: Write `runtime/duplex/internal/exec/event.go`**

```go
package exec

// Event — единица живого потока прогона наружу (в мост/клиент).
// Kind = "ready" (скрипт поднялся) либо "log" (строка вывода).
type Event struct {
	Kind    string
	Level   string
	Message string
}
```

- [ ] **Step 2: Write `runtime/duplex/internal/exec/exec.go`**

Fork of basic's exec.go with three changes: (a) `Spec` gains `Provides` and `Registry`; (b) a `TypeRequest` case in the message switch with the double gate; (c) a `dispatchRequest` helper. Everything else (timeouts, cancel→grace→kill, EOF handling) is copied verbatim.
```go
// Package exec запускает скрипт отдельным процессом и качает протокол моста
// до результата, применяя таймауты старта и выполнения. v2: обслуживает
// двусторонний канал request/response при protocol >= 2.
package exec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"wire-auto/runtime/duplex/internal/capreg"
	"wire-auto/runtime/duplex/internal/protocol"
)

const (
	StatusOK                = "OK"
	StatusScriptError       = "SCRIPT_ERROR"
	StatusProtocolViolation = "PROTOCOL_VIOLATION"
	StatusStartupTimeout    = "STARTUP_TIMEOUT"
	StatusRunTimeout        = "RUN_TIMEOUT"
	StatusCrashed           = "CRASHED"
	StatusCancelled         = "CANCELLED"
)

type LogLine struct {
	Level   string
	Message string
}

type Spec struct {
	Dir            string
	Command        string
	Args           []string
	Env            []string
	Protocol       int
	CoreAPI        int
	ScriptArgs     []string
	StartupTimeout time.Duration
	RunTimeout     time.Duration
	CancelGrace    time.Duration
	OnEvent        func(Event)
	// v2: авторизованные capability и реестр обработчиков.
	Provides []string
	Registry map[string]capreg.Handler
}

type Result struct {
	Status       string
	ExitCode     int
	Logs         []LogLine
	ErrorCode    string
	ErrorMessage string
	Result       json.RawMessage
}

type msgOrErr struct {
	msg protocol.Message
	err error
}

func containsStr(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// dispatchRequest применяет двойной гейт (provides → registry) и строит response.
// Ошибка capability уезжает в response.Code и прогон не роняет.
func dispatchRequest(spec Spec, req protocol.Message) protocol.Message {
	resp := protocol.Message{Type: protocol.TypeResponse, ID: req.ID}
	if !containsStr(spec.Provides, req.Capability) {
		resp.Code = "CAPABILITY_DENIED"
		resp.Message = "core does not provide capability " + req.Capability
		return resp
	}
	handler, ok := spec.Registry[req.Capability]
	if !ok {
		resp.Code = "CAPABILITY_UNIMPLEMENTED"
		resp.Message = "no handler for capability " + req.Capability
		return resp
	}
	result, code, err := handler(req.Params)
	if code != "" {
		resp.Code = code
		if err != nil {
			resp.Message = err.Error()
		}
		return resp
	}
	resp.Result = result
	return resp
}

func Run(ctx context.Context, spec Spec) Result {
	emit := func(e Event) {
		if spec.OnEvent != nil {
			spec.OnEvent(e)
		}
	}

	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	if len(spec.Env) > 0 {
		cmd.Env = append(os.Environ(), spec.Env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Result{Status: StatusCrashed, ErrorCode: StatusCrashed, ErrorMessage: err.Error()}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{Status: StatusCrashed, ErrorCode: StatusCrashed, ErrorMessage: err.Error()}
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return Result{Status: StatusCrashed, ErrorCode: StatusCrashed, ErrorMessage: err.Error()}
	}

	_ = protocol.Encode(stdin, protocol.Message{
		Type:     protocol.TypeHello,
		Protocol: spec.Protocol,
		CoreAPI:  spec.CoreAPI,
		Args:     spec.ScriptArgs,
	})

	dec := protocol.NewDecoder(stdout)
	ch := make(chan msgOrErr)
	readerDone := make(chan struct{})
	defer close(readerDone)
	go func() {
		for {
			m, err := dec.Next()
			select {
			case ch <- msgOrErr{m, err}:
			case <-readerDone:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	res := Result{Logs: []LogLine{}}
	gotReady := false
	deadline := time.After(spec.StartupTimeout)

	kill := func(status, code, msg string) Result {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		res.Status, res.ErrorCode, res.ErrorMessage = status, code, msg
		return res
	}

loop:
	for {
		select {
		case <-deadline:
			if !gotReady {
				return kill(StatusStartupTimeout, StatusStartupTimeout, "script did not send ready in time")
			}
			_ = protocol.Encode(stdin, protocol.Message{Type: protocol.TypeCancel})
			if spec.CancelGrace > 0 {
				grace := time.After(spec.CancelGrace)
			graceLoop:
				for {
					select {
					case ev := <-ch:
						if ev.err != nil {
							break graceLoop
						}
					case <-grace:
						break graceLoop
					}
				}
			}
			return kill(StatusRunTimeout, StatusRunTimeout, "")
		case <-ctx.Done():
			_ = protocol.Encode(stdin, protocol.Message{Type: protocol.TypeCancel})
			if spec.CancelGrace > 0 {
				grace := time.After(spec.CancelGrace)
			graceCancel:
				for {
					select {
					case ev := <-ch:
						if ev.err != nil {
							break graceCancel
						}
					case <-grace:
						break graceCancel
					}
				}
			}
			return kill(StatusCancelled, StatusCancelled, "cancelled by client")
		case ev := <-ch:
			if ev.err != nil {
				if errors.Is(ev.err, io.EOF) {
					break loop
				}
				return kill(StatusProtocolViolation, StatusProtocolViolation, ev.err.Error())
			}
			switch ev.msg.Type {
			case protocol.TypeReady:
				gotReady = true
				deadline = time.After(spec.RunTimeout)
				emit(Event{Kind: "ready"})
			case protocol.TypeLog:
				res.Logs = append(res.Logs, LogLine{Level: ev.msg.Level, Message: ev.msg.Message})
				emit(Event{Kind: "log", Level: ev.msg.Level, Message: ev.msg.Message})
			case protocol.TypeRequest:
				if spec.Protocol < 2 {
					return kill(StatusProtocolViolation, StatusProtocolViolation, "request not allowed on protocol 1")
				}
				if !gotReady {
					return kill(StatusProtocolViolation, StatusProtocolViolation, "request before ready")
				}
				_ = protocol.Encode(stdin, dispatchRequest(spec, ev.msg))
			case protocol.TypeDone:
				code := 0
				if ev.msg.ExitCode != nil {
					code = *ev.msg.ExitCode
				}
				res.ExitCode = code
				res.Result = ev.msg.Result
				_ = cmd.Wait()
				if code == 0 {
					res.Status = StatusOK
				} else {
					res.Status = StatusScriptError
				}
				return res
			default:
				return kill(StatusProtocolViolation, StatusProtocolViolation, "unexpected message type: "+ev.msg.Type)
			}
		}
	}

	_ = cmd.Wait()
	res.Status = StatusCrashed
	res.ErrorCode = StatusCrashed
	res.ErrorMessage = "process exited without done"
	return res
}
```

Note: the `TypeRequest` case checks `!gotReady`, but a request can only be read after `ready` in practice since the script is synchronous; the guard is defensive and matches the spec's "request before ready → violation" rule.

- [ ] **Step 3: Write the zero-shim test fixture `runtime/duplex/internal/exec/testdata/env-echo/main.py`**

```python
import sys, json

def send(o): sys.stdout.write(json.dumps(o, separators=(",", ":")) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())

recv()  # hello
send({"type": "ready"})
send({"type": "request", "id": "1", "capability": "env.get", "params": {"name": "WIRE_ECHO_VAR"}})
resp = recv()
val = resp.get("result", {}).get("value", "<denied:%s>" % resp.get("code"))
send({"type": "log", "level": "info", "message": "WIRE_ECHO_VAR=" + val})
send({"type": "done", "exitCode": 0})
```

- [ ] **Step 4: Write the failing test `runtime/duplex/internal/exec/exec_test.go`**

```go
package exec

import (
	"context"
	"strings"
	"testing"
	"time"

	"wire-auto/runtime/duplex/internal/capreg"
)

func v2Spec(dir string) Spec {
	return Spec{
		Dir:            dir,
		Command:        "python",
		Args:           []string{"main.py"},
		Protocol:       2,
		CoreAPI:        1,
		StartupTimeout: 5 * time.Second,
		RunTimeout:     5 * time.Second,
		Provides:       []string{"env.get"},
		Registry:       capreg.Default,
	}
}

func TestRequestResponseHappyPath(t *testing.T) {
	t.Setenv("WIRE_ECHO_VAR", "duplex-works")
	res := Run(context.Background(), v2Spec("testdata/env-echo"))
	if res.Status != StatusOK {
		t.Fatalf("status=%s err=%s", res.Status, res.ErrorMessage)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "duplex-works") {
		t.Fatalf("logs=%+v", res.Logs)
	}
}

func TestRequestCapabilityDenied(t *testing.T) {
	t.Setenv("WIRE_ECHO_VAR", "should-not-matter")
	spec := v2Spec("testdata/env-echo")
	spec.Provides = []string{} // env.get no longer authorized
	res := Run(context.Background(), spec)
	if res.Status != StatusOK {
		t.Fatalf("status=%s (script handles denial gracefully)", res.Status)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "<denied:CAPABILITY_DENIED>") {
		t.Fatalf("logs=%+v, want denial marker", res.Logs)
	}
}

func TestRequestOnProtocol1IsViolation(t *testing.T) {
	spec := v2Spec("testdata/env-echo")
	spec.Protocol = 1 // v1 script has no right to send request
	res := Run(context.Background(), spec)
	if res.Status != StatusProtocolViolation {
		t.Fatalf("status=%s, want PROTOCOL_VIOLATION", res.Status)
	}
}

func TestPlainV1FlowStillWorks(t *testing.T) {
	// A protocol-2 spec whose script never sends request behaves exactly like v1.
	res := Run(context.Background(), v2Spec("testdata/plain"))
	if res.Status != StatusOK {
		t.Fatalf("status=%s err=%s", res.Status, res.ErrorMessage)
	}
	if len(res.Logs) != 1 || res.Logs[0].Message != "no request here" {
		t.Fatalf("logs=%+v", res.Logs)
	}
}
```

- [ ] **Step 5: Write the second fixture `runtime/duplex/internal/exec/testdata/plain/main.py`**

```python
import sys, json

def send(o): sys.stdout.write(json.dumps(o, separators=(",", ":")) + "\n"); sys.stdout.flush()

sys.stdin.readline()  # hello
send({"type": "ready"})
send({"type": "log", "level": "info", "message": "no request here"})
send({"type": "done", "exitCode": 0})
```

- [ ] **Step 6: Run the tests**

Run: `cd runtime/duplex && go test ./internal/exec/ -v`
Expected: all four tests PASS (requires `python` on PATH).

- [ ] **Step 7: Vet the module**

Run: `cd runtime/duplex && go vet ./...`
Expected: no output. (STOP — no git.)

---

### Task 7: CLI runner + python demo + end-to-end

**Files:**
- Create: `runtime/duplex/cmd/wire/main.go`
- Create: `scripts/examples/env-report/script.manifest`
- Create: `scripts/examples/env-report/main.py`
- Test: `runtime/duplex/cmd/wire/main_test.go`

**Interfaces:**
- Consumes: `manifest`, `registry`, `handshake`, `exec`, `capreg` from earlier tasks.
- Produces: `run(runtimePath, coresDir, scriptDir string) (exec.Result, error)` — single-shot discover→route→handshake→spawn; used by the e2e test.

- [ ] **Step 1: Write `runtime/duplex/cmd/wire/main.go`**

A single-shot CLI (simpler than basic's streaming bridge — deview integration is out of v2 scope). Wires `capreg.Default` and reconciled `Provides` into the exec spec.
```go
// Command wire — одноразовый прогонщик: duplex-рантайм запускает один скрипт по
// пути (discover→route→handshake→spawn→pump) и печатает исход. Двусторонний
// канал request/response обслуживается реестром capability.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"wire-auto/runtime/duplex/internal/capreg"
	"wire-auto/runtime/duplex/internal/exec"
	"wire-auto/runtime/duplex/internal/handshake"
	"wire-auto/runtime/duplex/internal/manifest"
	"wire-auto/runtime/duplex/internal/registry"
)

func run(runtimePath, coresDir, scriptDir string) (exec.Result, error) {
	rt, err := manifest.LoadRuntime(runtimePath)
	if err != nil {
		return exec.Result{}, err
	}
	admitted, rejected, err := registry.Discover(coresDir, rt)
	if err != nil {
		return exec.Result{}, err
	}
	scr, err := manifest.LoadScript(filepath.Join(scriptDir, "script.manifest"))
	if err != nil {
		return exec.Result{}, err
	}

	core, ok := admitted[scr.Core]
	if !ok {
		code, msg := "UNKNOWN_CORE", "no admitted core named "+scr.Core
		for _, r := range rejected {
			if r.Name == scr.Core {
				code, msg = "CORE_INCOMPATIBLE", r.Reason
				break
			}
		}
		return exec.Result{Status: "HANDSHAKE_FAILED", ErrorCode: code, ErrorMessage: msg, Logs: []exec.LogLine{}}, nil
	}

	reconciled, err := handshake.Reconcile(rt, core.Manifest, scr)
	if err != nil {
		var he *handshake.Error
		if errors.As(err, &he) {
			return exec.Result{Status: "HANDSHAKE_FAILED", ErrorCode: he.Code, ErrorMessage: he.Message, Logs: []exec.LogLine{}}, nil
		}
		return exec.Result{}, err
	}

	absCoreDir, err := filepath.Abs(core.Dir)
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
	runtimePath := flag.String("runtime", "runtime/duplex/runtime.manifest", "path to runtime manifest")
	coresDir := flag.String("cores", "cores", "path to the cores directory")
	scriptDir := flag.String("script", "", "path to the script directory to run")
	flag.Parse()

	if *scriptDir == "" {
		fmt.Fprintln(os.Stderr, "usage: wire -script <dir> [-runtime m] [-cores d]")
		os.Exit(2)
	}

	res, err := run(*runtimePath, *coresDir, *scriptDir)
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

- [ ] **Step 2: Write the python demo `scripts/examples/env-report/script.manifest`**

```toml
name = "env-report"
version = "0.1.0"
core = "duplex"
coreApi = 1
capabilities = ["env.get"]
link = "stdio"
language = "python"
cmd = ["python", "main.py"]
```

- [ ] **Step 3: Write the zero-shim `scripts/examples/env-report/main.py`**

```python
import sys, json

def send(o): sys.stdout.write(json.dumps(o, separators=(",", ":")) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())

recv()  # hello — never imports any SDK; speaks the protocol raw
send({"type": "ready"})
send({"type": "request", "id": "1", "capability": "env.get", "params": {"name": "USER"}})
resp = recv()
val = resp.get("result", {}).get("value", "<denied:%s>" % resp.get("code"))
send({"type": "log", "level": "info", "message": "USER=" + val})
send({"type": "done", "exitCode": 0, "result": {"user": val}})
```

- [ ] **Step 4: Write the failing e2e test `runtime/duplex/cmd/wire/main_test.go`**

Paths are relative to `runtime/duplex/cmd/wire`; repo root is four levels up.
```go
package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func repoPaths(t *testing.T) (runtimeManifest, coresDir string) {
	t.Helper()
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "runtime", "duplex", "runtime.manifest"),
		filepath.Join(root, "cores")
}

func TestE2EEnvReport(t *testing.T) {
	t.Setenv("USER", "cyrille")
	rtm, cores := repoPaths(t)
	root, _ := filepath.Abs("../../../..")
	res, err := run(rtm, cores, filepath.Join(root, "scripts", "examples", "env-report"))
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

- [ ] **Step 5: Run the e2e test**

Run: `cd runtime/duplex && go test ./cmd/wire/ -v -run TestE2EEnvReport`
Expected: PASS. On Windows where `USER` may be unset by the OS, `t.Setenv` guarantees it; the runtime forwards `os.Environ()` to the child, so the child's `env.get` sees it.

- [ ] **Step 6: Manual smoke run from repo root**

Run: `cd D:/Projects/wire-auto && go run ./runtime/duplex/cmd/wire -script scripts/examples/env-report`
Expected: prints `Status: OK`, a `[info] USER=...` line, and `Result: {"user":"..."}`. (STOP — no git.)

---

### Task 8: Node demo + backward-compatibility e2e

**Files:**
- Create: `scripts/examples/env-report-node/script.manifest`
- Create: `scripts/examples/env-report-node/main.js`
- Modify: `runtime/duplex/cmd/wire/main_test.go` (append two tests)

**Interfaces:**
- Consumes: `run(...)` from Task 7. No new production interfaces.

- [ ] **Step 1: Write `scripts/examples/env-report-node/script.manifest`**

```toml
name = "env-report-node"
version = "0.1.0"
core = "duplex"
coreApi = 1
capabilities = ["env.get"]
link = "stdio"
language = "node"
cmd = ["node", "main.js"]
```

- [ ] **Step 2: Write the zero-shim `scripts/examples/env-report-node/main.js`**

Pure Node, no external packages — proves the protocol is language-agnostic. Reads exactly one line per `recv()` from a buffered stdin reader.
```js
// zero-shim: speaks the wire protocol raw over stdio, no SDK import.
const chunks = [];
let buf = "";
const pending = [];

process.stdin.on("data", (d) => {
  buf += d;
  let i;
  while ((i = buf.indexOf("\n")) >= 0) {
    const line = buf.slice(0, i);
    buf = buf.slice(i + 1);
    if (line.length === 0) continue;
    const msg = JSON.parse(line);
    const waiter = pending.shift();
    if (waiter) waiter(msg);
    else chunks.push(msg);
  }
});

function recv() {
  if (chunks.length) return Promise.resolve(chunks.shift());
  return new Promise((res) => pending.push(res));
}
function send(o) {
  process.stdout.write(JSON.stringify(o) + "\n");
}

(async () => {
  await recv(); // hello
  send({ type: "ready" });
  send({ type: "request", id: "1", capability: "env.get", params: { name: "USER" } });
  const resp = await recv();
  const val = resp.result ? resp.result.value : "<denied:" + resp.code + ">";
  send({ type: "log", level: "info", message: "USER=" + val });
  send({ type: "done", exitCode: 0, result: { user: val } });
  process.exit(0);
})();
```

- [ ] **Step 3: Append two tests to `runtime/duplex/cmd/wire/main_test.go`**

```go
func TestE2EEnvReportNode(t *testing.T) {
	t.Setenv("USER", "cyrille")
	rtm, cores := repoPaths(t)
	root, _ := filepath.Abs("../../../..")
	res, err := run(rtm, cores, filepath.Join(root, "scripts", "examples", "env-report-node"))
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

// The duplex runtime admits the frozen v1 core (protocol 1). An old regular-core
// script runs unchanged as a one-way v1 dialog — backward compatibility live.
func TestE2ERegularOnDuplex(t *testing.T) {
	rtm, cores := repoPaths(t)
	root, _ := filepath.Abs("../../../..")
	res, err := run(rtm, cores, filepath.Join(root, "scripts", "examples", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "OK" {
		t.Fatalf("status=%s err=%s %s", res.Status, res.ErrorCode, res.ErrorMessage)
	}
}
```

- [ ] **Step 4: Run the new e2e tests**

Run: `cd runtime/duplex && go test ./cmd/wire/ -v`
Expected: `TestE2EEnvReport`, `TestE2EEnvReportNode`, `TestE2ERegularOnDuplex` all PASS (requires `python` and `node` on PATH). `TestE2ERegularOnDuplex` proves the frozen `cores/regular` + `scripts/examples/hello` run on the new runtime via the protocol-1 path.

- [ ] **Step 5: Full module gate**

Run: `cd runtime/duplex && go build ./... && go vet ./... && go test ./...`
Expected: all packages build, vet clean, all tests PASS. This is the v2 done-definition gate. (STOP — no git.)

---

## Notes for the implementer

- `scripts/examples/hello` already exists (v1 python shim, `core = "regular"`). Task 8 reuses it as-is for the backward-compat test — do not modify it.
- If `node` is unavailable in the environment, `TestE2EEnvReportNode` will fail with a spawn error; that is an environment gap, not a code defect — report it rather than deleting the test.
- The exec loop is copied from `runtime/basic` deliberately (WET). Keep the copied timeout/cancel logic byte-identical to basic so behavior stays proven; the only additions are the `Provides`/`Registry` spec fields and the `TypeRequest` case.
- The spec's §2 tree lists `internal/discovery/` and a deview-facing bridge; both are intentionally **omitted** from this plan. They power the app↔runtime script catalog, which §11 puts out of v2 scope. The single-shot `cmd/wire` runner (Task 7) replaces the streaming bridge for now. Add `discovery`/`bridge` only when deview integration is picked up.

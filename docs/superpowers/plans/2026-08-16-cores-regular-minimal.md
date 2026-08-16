# cores/regular минимальное ядро — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Собрать «позвоночник» ядра `regular`: рантайм читает три TOML-манифеста, делает рукопожатие, спавнит python-скрипт и общается с ним по протоколу JSON Lines до результата.

**Architecture:** Out-of-process. Рантайм на Go запускает скрипт отдельным процессом; общение — построчный JSON через stdin/stdout. Код разбит на изолированные пакеты: `manifest` (паспорта), `handshake` (сведение контрактов), `protocol` (сообщения+кодек), `exec` (спавн+прокачка+таймауты), `cmd` (CLI-склейка). Скрипт использует тонкий python-шим (~60 строк).

**Tech Stack:** Go 1.26 (stdlib + `github.com/BurntSushi/toml`), Python 3.14 (только stdlib). Тесты Go — колокейтед `_test.go`.

## Global Constraints

- Модуль Go: `wire-auto` (один go.mod в корне репозитория), `go 1.26`.
- Единственная внешняя зависимость: `github.com/BurntSushi/toml`. Больше ничего не тянем.
- Версия протокола моста: `1`. Версия контракта ядра (`coreApi`): `1`.
- Транспорт протокола: JSON Lines — одно сообщение = одна строка компактного JSON + `\n`.
- Набор сообщений v1 (ровно эти `type`, регистр важен): `hello`, `ready`, `log`, `done`, `cancel`. `request`/`response` — зарезервированы, в v1 любой `request` от скрипта = нарушение.
- Коды рукопожатия (машиночитаемые, верхний регистр): `UNKNOWN_CORE`, `CORE_API_MISMATCH`, `CAPABILITY_DENIED`, `PROTOCOL_UNSUPPORTED`, `LANGUAGE_UNSUPPORTED`.
- Статусы результата исполнения: `OK`, `SCRIPT_ERROR`, `HANDSHAKE_FAILED`, `PROTOCOL_VIOLATION`, `STARTUP_TIMEOUT`, `RUN_TIMEOUT`, `CRASHED`.
- Железа в v1 нет: у ядра `provides = []`, у скрипта `capabilities = []`.
- Команда запуска python — `python` (в этом окружении `python3` отсутствует).
- Все пути — абсолютные Windows-пути при операциях с файлами (правило проекта).

---

### Task 1: Go-модуль + пакет `manifest`

Читает и валидирует три вида TOML-паспортов. Чистые типы, без побочных эффектов кроме чтения файла.

**Files:**
- Create: `D:\Projects\wire-auto\go.mod`
- Create: `D:\Projects\wire-auto\runtime\internal\manifest\manifest.go`
- Test: `D:\Projects\wire-auto\runtime\internal\manifest\manifest_test.go`

**Interfaces:**
- Consumes: ничего (первый пакет).
- Produces:
  - `type Runtime struct { Name string; Version string; Protocols []int; Cores []string }`
  - `type Core struct { Name string; Version string; CoreAPI int; Protocol int; Provides []string; Languages []string }`
  - `type Script struct { Name string; Version string; Core string; CoreAPI int; Language string; Entry string; Capabilities []string }`
  - `func LoadRuntime(path string) (Runtime, error)`
  - `func LoadCore(path string) (Core, error)`
  - `func LoadScript(path string) (Script, error)`
  - Каждый Load возвращает ошибку, если обязательное поле пустое.

- [ ] **Step 1: Инициализировать модуль и зависимость**

Run:
```bash
cd /d/Projects/wire-auto && go mod init wire-auto && go get github.com/BurntSushi/toml@latest && go mod tidy
```
Expected: создан `go.mod` с `module wire-auto`, `go 1.26` и require `github.com/BurntSushi/toml`.

- [ ] **Step 2: Написать падающий тест**

Create `D:\Projects\wire-auto\runtime\internal\manifest\manifest_test.go`:
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
language = "python"
entry = "main.py"
capabilities = []
`)
	s, err := LoadScript(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "hello" || s.Core != "regular" || s.CoreAPI != 1 || s.Entry != "main.py" {
		t.Fatalf("bad parse: %+v", s)
	}
}

func TestLoadScriptMissingEntry(t *testing.T) {
	p := write(t, "script.manifest", `
name = "hello"
version = "0.1.0"
core = "regular"
coreApi = 1
language = "python"
`)
	if _, err := LoadScript(p); err == nil {
		t.Fatal("expected error for missing entry")
	}
}

func TestLoadCoreValid(t *testing.T) {
	p := write(t, "core.manifest", `
name = "regular"
version = "0.1.0"
coreApi = 1
protocol = 1
provides = []
languages = ["python"]
`)
	c, err := LoadCore(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Protocol != 1 || len(c.Languages) != 1 || c.Languages[0] != "python" {
		t.Fatalf("bad parse: %+v", c)
	}
}

func TestLoadRuntimeValid(t *testing.T) {
	p := write(t, "runtime.manifest", `
name = "wire-auto-runtime"
version = "0.1.0"
protocols = [1]
cores = ["regular"]
`)
	r, err := LoadRuntime(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Protocols) != 1 || r.Protocols[0] != 1 || r.Cores[0] != "regular" {
		t.Fatalf("bad parse: %+v", r)
	}
}
```

- [ ] **Step 3: Запустить тест — убедиться, что не компилируется/падает**

Run: `cd /d/Projects/wire-auto && go test ./runtime/internal/manifest/`
Expected: FAIL — `undefined: LoadScript` и т.п.

- [ ] **Step 4: Реализовать пакет**

Create `D:\Projects\wire-auto\runtime\internal\manifest\manifest.go`:
```go
// Package manifest читает и валидирует три вида паспортов платформы:
// runtime, core и script. Формат — TOML.
package manifest

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type Runtime struct {
	Name      string `toml:"name"`
	Version   string `toml:"version"`
	Protocols []int  `toml:"protocols"`
	Cores     []string `toml:"cores"`
}

type Core struct {
	Name      string   `toml:"name"`
	Version   string   `toml:"version"`
	CoreAPI   int      `toml:"coreApi"`
	Protocol  int      `toml:"protocol"`
	Provides  []string `toml:"provides"`
	Languages []string `toml:"languages"`
}

type Script struct {
	Name         string   `toml:"name"`
	Version      string   `toml:"version"`
	Core         string   `toml:"core"`
	CoreAPI      int      `toml:"coreApi"`
	Language     string   `toml:"language"`
	Entry        string   `toml:"entry"`
	Capabilities []string `toml:"capabilities"`
}

func LoadRuntime(path string) (Runtime, error) {
	var r Runtime
	if _, err := toml.DecodeFile(path, &r); err != nil {
		return Runtime{}, fmt.Errorf("read runtime manifest %s: %w", path, err)
	}
	if r.Name == "" || len(r.Protocols) == 0 {
		return Runtime{}, fmt.Errorf("runtime manifest %s: name and protocols are required", path)
	}
	return r, nil
}

func LoadCore(path string) (Core, error) {
	var c Core
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return Core{}, fmt.Errorf("read core manifest %s: %w", path, err)
	}
	if c.Name == "" || c.CoreAPI == 0 || c.Protocol == 0 || len(c.Languages) == 0 {
		return Core{}, fmt.Errorf("core manifest %s: name, coreApi, protocol, languages are required", path)
	}
	return c, nil
}

func LoadScript(path string) (Script, error) {
	var s Script
	if _, err := toml.DecodeFile(path, &s); err != nil {
		return Script{}, fmt.Errorf("read script manifest %s: %w", path, err)
	}
	if s.Name == "" || s.Core == "" || s.CoreAPI == 0 || s.Language == "" || s.Entry == "" {
		return Script{}, fmt.Errorf("script manifest %s: name, core, coreApi, language, entry are required", path)
	}
	return s, nil
}
```

- [ ] **Step 5: Запустить тесты — зелёные**

Run: `cd /d/Projects/wire-auto && go test ./runtime/internal/manifest/`
Expected: PASS (4 теста).

- [ ] **Step 6: Commit**

```bash
cd /d/Projects/wire-auto && git add go.mod go.sum runtime/internal/manifest/ && git commit -m "feat(manifest): load and validate runtime/core/script TOML manifests"
```

---

### Task 2: пакет `handshake`

Сводит три паспорта. Чистая функция, без I/O. Возвращает сведённые версии или типизированную ошибку с кодом.

**Files:**
- Create: `D:\Projects\wire-auto\runtime\internal\handshake\handshake.go`
- Test: `D:\Projects\wire-auto\runtime\internal\handshake\handshake_test.go`

**Interfaces:**
- Consumes: `manifest.Runtime`, `manifest.Core`, `manifest.Script`.
- Produces:
  - `type Reconciled struct { Protocol int; CoreAPI int }`
  - `type Error struct { Code string; Message string }` с методом `func (e *Error) Error() string`
  - `func Reconcile(rt manifest.Runtime, core manifest.Core, scr manifest.Script) (Reconciled, error)` — при провале возвращает `*Error` с одним из кодов из Global Constraints.

- [ ] **Step 1: Написать падающий тест**

Create `D:\Projects\wire-auto\runtime\internal\handshake\handshake_test.go`:
```go
package handshake

import (
	"errors"
	"testing"

	"wire-auto/runtime/internal/manifest"
)

func base() (manifest.Runtime, manifest.Core, manifest.Script) {
	rt := manifest.Runtime{Name: "rt", Protocols: []int{1}, Cores: []string{"regular"}}
	core := manifest.Core{Name: "regular", CoreAPI: 1, Protocol: 1, Provides: []string{}, Languages: []string{"python"}}
	scr := manifest.Script{Name: "hello", Core: "regular", CoreAPI: 1, Language: "python", Entry: "main.py", Capabilities: []string{}}
	return rt, core, scr
}

func TestReconcileOK(t *testing.T) {
	rt, core, scr := base()
	got, err := Reconcile(rt, core, scr)
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
	rt, core, scr := base()
	scr.Core = "weird"
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "UNKNOWN_CORE" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileCoreAPIMismatch(t *testing.T) {
	rt, core, scr := base()
	scr.CoreAPI = 2
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "CORE_API_MISMATCH" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileCapabilityDenied(t *testing.T) {
	rt, core, scr := base()
	scr.Capabilities = []string{"serial"}
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "CAPABILITY_DENIED" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileProtocolUnsupported(t *testing.T) {
	rt, core, scr := base()
	core.Protocol = 2
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "PROTOCOL_UNSUPPORTED" {
		t.Fatalf("got %s", got)
	}
}

func TestReconcileLanguageUnsupported(t *testing.T) {
	rt, core, scr := base()
	scr.Language = "ruby"
	_, err := Reconcile(rt, core, scr)
	if got := codeOf(t, err); got != "LANGUAGE_UNSUPPORTED" {
		t.Fatalf("got %s", got)
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd /d/Projects/wire-auto && go test ./runtime/internal/handshake/`
Expected: FAIL — `undefined: Reconcile`.

- [ ] **Step 3: Реализовать пакет**

Create `D:\Projects\wire-auto\runtime\internal\handshake\handshake.go`:
```go
// Package handshake сводит три паспорта (runtime, core, script) перед запуском.
// Первое несведение — стоп с машиночитаемым кодом.
package handshake

import "wire-auto/runtime/internal/manifest"

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

// Reconcile проверяет совместимость сверху вниз согласно разделу 3 спеки.
func Reconcile(rt manifest.Runtime, core manifest.Core, scr manifest.Script) (Reconciled, error) {
	if scr.Core != core.Name || !contains(rt.Cores, scr.Core) {
		return Reconciled{}, &Error{"UNKNOWN_CORE", "script targets core " + scr.Core + " which is not available"}
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
	if !contains(core.Languages, scr.Language) {
		return Reconciled{}, &Error{"LANGUAGE_UNSUPPORTED", "core cannot run language " + scr.Language}
	}
	return Reconciled{Protocol: core.Protocol, CoreAPI: core.CoreAPI}, nil
}
```

- [ ] **Step 4: Запустить тесты — зелёные**

Run: `cd /d/Projects/wire-auto && go test ./runtime/internal/handshake/`
Expected: PASS (6 тестов).

- [ ] **Step 5: Commit**

```bash
cd /d/Projects/wire-auto && git add runtime/internal/handshake/ && git commit -m "feat(handshake): reconcile three manifests with machine-readable codes"
```

---

### Task 3: пакет `protocol`

Типы сообщений и кодек JSON Lines. Чистый, работает поверх `io.Reader`/`io.Writer`.

**Files:**
- Create: `D:\Projects\wire-auto\runtime\internal\protocol\protocol.go`
- Test: `D:\Projects\wire-auto\runtime\internal\protocol\protocol_test.go`

**Interfaces:**
- Consumes: ничего.
- Produces:
  - `type Message struct { Type string; Protocol int; CoreAPI int; Args []string; Level string; Message string; ExitCode *int; Result json.RawMessage }` с корректными json-тегами и `omitempty`.
  - Константы типов: `TypeHello="hello"`, `TypeReady="ready"`, `TypeLog="log"`, `TypeDone="done"`, `TypeCancel="cancel"`.
  - `func Encode(w io.Writer, m Message) error` — пишет компактный JSON + `\n`.
  - `type Decoder struct{...}`; `func NewDecoder(r io.Reader) *Decoder`; `func (d *Decoder) Next() (Message, error)` — читает одну строку; `io.EOF` при конце.

- [ ] **Step 1: Написать падающий тест**

Create `D:\Projects\wire-auto\runtime\internal\protocol\protocol_test.go`:
```go
package protocol

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestEncodeCompactLine(t *testing.T) {
	var b bytes.Buffer
	if err := Encode(&b, Message{Type: TypeHello, Protocol: 1, CoreAPI: 1}); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if got != `{"type":"hello","protocol":1,"coreApi":1}`+"\n" {
		t.Fatalf("bad encoding: %q", got)
	}
}

func TestDecodeSequence(t *testing.T) {
	in := strings.NewReader(
		`{"type":"ready"}` + "\n" +
			`{"type":"log","level":"info","message":"hi"}` + "\n" +
			`{"type":"done","exitCode":0}` + "\n")
	d := NewDecoder(in)

	m1, err := d.Next()
	if err != nil || m1.Type != TypeReady {
		t.Fatalf("m1: %+v %v", m1, err)
	}
	m2, err := d.Next()
	if err != nil || m2.Type != TypeLog || m2.Message != "hi" {
		t.Fatalf("m2: %+v %v", m2, err)
	}
	m3, err := d.Next()
	if err != nil || m3.Type != TypeDone || m3.ExitCode == nil || *m3.ExitCode != 0 {
		t.Fatalf("m3: %+v %v", m3, err)
	}
	if _, err := d.Next(); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestDecodeBadJSON(t *testing.T) {
	d := NewDecoder(strings.NewReader("not json\n"))
	if _, err := d.Next(); err == nil {
		t.Fatal("expected error on bad json")
	}
}
```

- [ ] **Step 2: Запустить — падает**

Run: `cd /d/Projects/wire-auto && go test ./runtime/internal/protocol/`
Expected: FAIL — `undefined: Encode`.

- [ ] **Step 3: Реализовать пакет**

Create `D:\Projects\wire-auto\runtime\internal\protocol\protocol.go`:
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
	TypeHello  = "hello"
	TypeReady  = "ready"
	TypeLog    = "log"
	TypeDone   = "done"
	TypeCancel = "cancel"
)

type Message struct {
	Type     string          `json:"type"`
	Protocol int             `json:"protocol,omitempty"`
	CoreAPI  int             `json:"coreApi,omitempty"`
	Args     []string        `json:"args,omitempty"`
	Level    string          `json:"level,omitempty"`
	Message  string          `json:"message,omitempty"`
	ExitCode *int            `json:"exitCode,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
}

// Encode пишет сообщение как одну JSON-строку с переводом строки.
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
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return &Decoder{sc: sc}
}

// Next читает следующее сообщение. Возвращает io.EOF, когда поток кончился.
func (d *Decoder) Next() (Message, error) {
	for d.sc.Scan() {
		line := d.sc.Bytes()
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

- [ ] **Step 4: Запустить тесты — зелёные**

Run: `cd /d/Projects/wire-auto && go test ./runtime/internal/protocol/`
Expected: PASS (3 теста).

- [ ] **Step 5: Commit**

```bash
cd /d/Projects/wire-auto && git add runtime/internal/protocol/ && git commit -m "feat(protocol): JSON Lines message types and codec"
```

---

### Task 4: пакет `exec`

Спавнит процесс, шлёт `hello`, качает сообщения, применяет таймауты, формирует единый результат. Тестируется реальным `python -c`.

**Files:**
- Create: `D:\Projects\wire-auto\runtime\internal\exec\exec.go`
- Test: `D:\Projects\wire-auto\runtime\internal\exec\exec_test.go`

**Interfaces:**
- Consumes: `protocol` (Message, Encode, Decoder, константы типов).
- Produces:
  - `type LogLine struct { Level string; Message string }`
  - `type Spec struct { Dir string; Command string; Args []string; Env []string; Protocol int; CoreAPI int; ScriptArgs []string; StartupTimeout time.Duration; RunTimeout time.Duration }`
  - `type Result struct { Status string; ExitCode int; Logs []LogLine; ErrorCode string; ErrorMessage string }`
  - Константы статусов: `StatusOK="OK"`, `StatusScriptError="SCRIPT_ERROR"`, `StatusProtocolViolation="PROTOCOL_VIOLATION"`, `StatusStartupTimeout="STARTUP_TIMEOUT"`, `StatusRunTimeout="RUN_TIMEOUT"`, `StatusCrashed="CRASHED"`.
  - `func Run(spec Spec) Result`.

- [ ] **Step 1: Написать падающий тест**

Create `D:\Projects\wire-auto\runtime\internal\exec\exec_test.go`:
```go
package exec

import (
	"testing"
	"time"
)

// helloReadyLogDone: минимальный «скрипт» на python, эмулирующий шим.
const goodScript = `
import sys, json
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
send({"type":"log","level":"info","message":"hello from python"})
send({"type":"done","exitCode":0})
`

func baseSpec(code string) Spec {
	return Spec{
		Command:        "python",
		Args:           []string{"-c", code},
		Protocol:       1,
		CoreAPI:        1,
		StartupTimeout: 5 * time.Second,
		RunTimeout:     5 * time.Second,
	}
}

func TestRunOK(t *testing.T) {
	res := Run(baseSpec(goodScript))
	if res.Status != StatusOK {
		t.Fatalf("status=%s err=%s", res.Status, res.ErrorMessage)
	}
	if len(res.Logs) != 1 || res.Logs[0].Message != "hello from python" {
		t.Fatalf("logs=%+v", res.Logs)
	}
}

func TestRunScriptError(t *testing.T) {
	code := `
import sys, json
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
send({"type":"done","exitCode":3})
`
	res := Run(baseSpec(code))
	if res.Status != StatusScriptError || res.ExitCode != 3 {
		t.Fatalf("got status=%s exit=%d", res.Status, res.ExitCode)
	}
}

func TestRunProtocolViolationOnRequest(t *testing.T) {
	code := `
import sys, json
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
send({"type":"request","capability":"serial"})
`
	res := Run(baseSpec(code))
	if res.Status != StatusProtocolViolation {
		t.Fatalf("got status=%s", res.Status)
	}
}

func TestRunRunTimeout(t *testing.T) {
	code := `
import sys, json, time
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
time.sleep(30)
`
	spec := baseSpec(code)
	spec.RunTimeout = 500 * time.Millisecond
	res := Run(spec)
	if res.Status != StatusRunTimeout {
		t.Fatalf("got status=%s", res.Status)
	}
}

func TestRunCrashNoDone(t *testing.T) {
	code := `
import sys, json
sys.stdin.readline()
def send(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
send({"type":"ready"})
sys.exit(0)
`
	res := Run(baseSpec(code))
	if res.Status != StatusCrashed {
		t.Fatalf("got status=%s", res.Status)
	}
}
```

- [ ] **Step 2: Запустить — падает**

Run: `cd /d/Projects/wire-auto && go test ./runtime/internal/exec/`
Expected: FAIL — `undefined: Run`.

- [ ] **Step 3: Реализовать пакет**

Create `D:\Projects\wire-auto\runtime\internal\exec\exec.go`:
```go
// Package exec запускает скрипт отдельным процессом и качает протокол моста
// до результата, применяя таймауты старта и выполнения.
package exec

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"wire-auto/runtime/internal/protocol"
)

const (
	StatusOK                = "OK"
	StatusScriptError       = "SCRIPT_ERROR"
	StatusProtocolViolation = "PROTOCOL_VIOLATION"
	StatusStartupTimeout    = "STARTUP_TIMEOUT"
	StatusRunTimeout        = "RUN_TIMEOUT"
	StatusCrashed           = "CRASHED"
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
}

type Result struct {
	Status       string
	ExitCode     int
	Logs         []LogLine
	ErrorCode    string
	ErrorMessage string
}

// msgOrErr — одно прочитанное сообщение либо ошибка чтения потока.
type msgOrErr struct {
	msg protocol.Message
	err error
}

func Run(spec Spec) Result {
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
	cmd.Stderr = os.Stderr // сырой лог самого шима — в диагностику

	if err := cmd.Start(); err != nil {
		return Result{Status: StatusCrashed, ErrorCode: StatusCrashed, ErrorMessage: err.Error()}
	}

	// Отправляем hello.
	_ = protocol.Encode(stdin, protocol.Message{
		Type:     protocol.TypeHello,
		Protocol: spec.Protocol,
		CoreAPI:  spec.CoreAPI,
		Args:     spec.ScriptArgs,
	})

	// Читаем сообщения в отдельной горутине, чтобы наложить таймауты.
	dec := protocol.NewDecoder(stdout)
	ch := make(chan msgOrErr)
	go func() {
		for {
			m, err := dec.Next()
			ch <- msgOrErr{m, err}
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
			return kill(StatusRunTimeout, StatusRunTimeout, "script did not finish in time")
		case ev := <-ch:
			if ev.err != nil {
				if errors.Is(ev.err, io.EOF) {
					// Поток кончился без done → падение.
					break loop
				}
				return kill(StatusProtocolViolation, StatusProtocolViolation, ev.err.Error())
			}
			switch ev.msg.Type {
			case protocol.TypeReady:
				gotReady = true
				deadline = time.After(spec.RunTimeout) // переключаемся на таймаут выполнения
			case protocol.TypeLog:
				res.Logs = append(res.Logs, LogLine{Level: ev.msg.Level, Message: ev.msg.Message})
			case protocol.TypeDone:
				code := 0
				if ev.msg.ExitCode != nil {
					code = *ev.msg.ExitCode
				}
				res.ExitCode = code
				_ = cmd.Wait()
				if code == 0 {
					res.Status = StatusOK
				} else {
					res.Status = StatusScriptError
				}
				return res
			default:
				// Неизвестный тип, либо зарезервированный request/response в v1.
				return kill(StatusProtocolViolation, StatusProtocolViolation, "unexpected message type: "+ev.msg.Type)
			}
		}
	}

	// Вышли по EOF без done.
	_ = cmd.Wait()
	res.Status = StatusCrashed
	res.ErrorCode = StatusCrashed
	res.ErrorMessage = "process exited without done"
	return res
}
```

- [ ] **Step 4: Запустить тесты — зелёные**

Run: `cd /d/Projects/wire-auto && go test ./runtime/internal/exec/`
Expected: PASS (5 тестов). Примечание: тесты запускают реальный `python`.

- [ ] **Step 5: Commit**

```bash
cd /d/Projects/wire-auto && git add runtime/internal/exec/ && git commit -m "feat(exec): spawn script process, pump protocol, enforce timeouts"
```

---

### Task 5: python-шим + пример скрипта + три манифеста

Тонкий SDK на python и первый рабочий пример со своим паспортом. Плюс паспорта рантайма и ядра.

**Files:**
- Create: `D:\Projects\wire-auto\cores\regular\sdk\python\wire.py`
- Create: `D:\Projects\wire-auto\cores\regular\core.manifest`
- Create: `D:\Projects\wire-auto\runtime\runtime.manifest`
- Create: `D:\Projects\wire-auto\scripts\examples\hello\script.manifest`
- Create: `D:\Projects\wire-auto\scripts\examples\hello\main.py`
- Test: `D:\Projects\wire-auto\cores\regular\sdk\python\wire_test.py`

**Interfaces:**
- Consumes: протокол JSON Lines (Task 3), формат hello от `exec` (Task 4).
- Produces: python-класс `Script` с методами `start() -> dict`, `log(message, level="info")`, `done(exit_code=0, result=None)`; читает `hello` из stdin, шлёт `ready`/`log`/`done` в stdout. Манифесты — согласно Global Constraints.

- [ ] **Step 1: Написать падающий тест шима**

Create `D:\Projects\wire-auto\cores\regular\sdk\python\wire_test.py`:
```python
import io
import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(__file__))
import wire  # noqa: E402


class ShimTest(unittest.TestCase):
    def run_script(self, hello):
        stdin = io.StringIO(json.dumps(hello) + "\n")
        stdout = io.StringIO()
        s = wire.Script(stdin=stdin, stdout=stdout)
        got_hello = s.start()
        s.log("hello from python")
        s.done(0)
        lines = [json.loads(l) for l in stdout.getvalue().splitlines() if l]
        return got_hello, lines

    def test_handshake_and_messages(self):
        hello, lines = self.run_script({"type": "hello", "protocol": 1, "coreApi": 1})
        self.assertEqual(hello["coreApi"], 1)
        self.assertEqual(lines[0], {"type": "ready"})
        self.assertEqual(lines[1], {"type": "log", "level": "info", "message": "hello from python"})
        self.assertEqual(lines[2], {"type": "done", "exitCode": 0})


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Запустить — падает**

Run: `cd /d/Projects/wire-auto && python cores/regular/sdk/python/wire_test.py`
Expected: FAIL — `ModuleNotFoundError: No module named 'wire'` (файла ещё нет).

- [ ] **Step 3: Реализовать шим**

Create `D:\Projects\wire-auto\cores\regular\sdk\python\wire.py`:
```python
"""Тонкий шим wire-auto для python: говорит протокол моста (JSON Lines)."""
import json
import sys


class Script:
    def __init__(self, stdin=None, stdout=None):
        self._in = stdin if stdin is not None else sys.stdin
        self._out = stdout if stdout is not None else sys.stdout
        self.hello = None

    def start(self):
        """Читает hello от рантайма, отвечает ready, возвращает конфиг."""
        line = self._in.readline()
        self.hello = json.loads(line)
        self._send({"type": "ready"})
        return self.hello

    def log(self, message, level="info"):
        self._send({"type": "log", "level": level, "message": message})

    def done(self, exit_code=0, result=None):
        msg = {"type": "done", "exitCode": exit_code}
        if result is not None:
            msg["result"] = result
        self._send(msg)

    def _send(self, obj):
        self._out.write(json.dumps(obj) + "\n")
        self._out.flush()
```

- [ ] **Step 4: Запустить тест шима — зелёный**

Run: `cd /d/Projects/wire-auto && python cores/regular/sdk/python/wire_test.py`
Expected: PASS (`OK`).

- [ ] **Step 5: Создать три манифеста и пример**

Create `D:\Projects\wire-auto\runtime\runtime.manifest`:
```toml
name = "wire-auto-runtime"
version = "0.1.0"
protocols = [1]
cores = ["regular"]
```

Create `D:\Projects\wire-auto\cores\regular\core.manifest`:
```toml
name = "regular"
version = "0.1.0"
coreApi = 1
protocol = 1
provides = []
languages = ["python"]
```

Create `D:\Projects\wire-auto\scripts\examples\hello\script.manifest`:
```toml
name = "hello"
version = "0.1.0"
core = "regular"
coreApi = 1
language = "python"
entry = "main.py"
capabilities = []
```

Create `D:\Projects\wire-auto\scripts\examples\hello\main.py`:
```python
from wire import Script


def main():
    s = Script()
    s.start()
    s.log("hello from python")
    s.done(0)


if __name__ == "__main__":
    main()
```

- [ ] **Step 6: Commit**

```bash
cd /d/Projects/wire-auto && git add cores/ runtime/runtime.manifest scripts/ && git commit -m "feat(sdk): python shim, three manifests, hello example"
```

---

### Task 6: CLI `cmd` + сквозной интеграционный тест

Склейка: загрузка манифестов → рукопожатие → запуск → печать результата. Плюс end-to-end тест на реальном примере.

**Files:**
- Create: `D:\Projects\wire-auto\runtime\cmd\wire\main.go`
- Test: `D:\Projects\wire-auto\runtime\cmd\wire\main_test.go`

**Interfaces:**
- Consumes: `manifest.LoadRuntime/LoadCore/LoadScript`, `handshake.Reconcile`, `exec.Run`/`exec.Spec`/`exec.Result`.
- Produces: `func run(runtimePath, corePath, scriptDir string) (exec.Result, error)` — вся оркестрация; и `main()`, печатающий результат как JSON. При провале рукопожатия возвращает `Result{Status:"HANDSHAKE_FAILED", ErrorCode:<код>}`.

- [ ] **Step 1: Написать падающий сквозной тест**

Create `D:\Projects\wire-auto\runtime\cmd\wire\main_test.go`:
```go
package main

import (
	"path/filepath"
	"testing"
)

// repoRoot: .../runtime/cmd/wire → поднимаемся на три уровня.
func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestEndToEndHello(t *testing.T) {
	root := repoRoot(t)
	res, err := run(
		filepath.Join(root, "runtime", "runtime.manifest"),
		filepath.Join(root, "cores", "regular", "core.manifest"),
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
		filepath.Join(root, "runtime", "runtime.manifest"),
		filepath.Join(root, "cores", "regular", "core.manifest"),
		filepath.Join(root, "runtime", "cmd", "wire", "testdata", "needs-serial"),
	)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.Status != "HANDSHAKE_FAILED" || res.ErrorCode != "CAPABILITY_DENIED" {
		t.Fatalf("status=%s code=%s", res.Status, res.ErrorCode)
	}
}
```

Create `D:\Projects\wire-auto\runtime\cmd\wire\testdata\needs-serial\script.manifest`:
```toml
name = "needs-serial"
version = "0.1.0"
core = "regular"
coreApi = 1
language = "python"
entry = "main.py"
capabilities = ["serial"]
```

Create `D:\Projects\wire-auto\runtime\cmd\wire\testdata\needs-serial\main.py`:
```python
# Никогда не запустится в v1: рукопожатие отклонит из-за capability serial.
from wire import Script

Script().start()
```

- [ ] **Step 2: Запустить — падает**

Run: `cd /d/Projects/wire-auto && go test ./runtime/cmd/wire/`
Expected: FAIL — `undefined: run`.

- [ ] **Step 3: Реализовать CLI**

Create `D:\Projects\wire-auto\runtime\cmd\wire\main.go`:
```go
// Command wire — минимальный рантайм: сводит манифесты и запускает скрипт ядром regular.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"wire-auto/runtime/internal/exec"
	"wire-auto/runtime/internal/handshake"
	"wire-auto/runtime/internal/manifest"
)

func run(runtimePath, corePath, scriptDir string) (exec.Result, error) {
	rt, err := manifest.LoadRuntime(runtimePath)
	if err != nil {
		return exec.Result{}, err
	}
	core, err := manifest.LoadCore(corePath)
	if err != nil {
		return exec.Result{}, err
	}
	scr, err := manifest.LoadScript(filepath.Join(scriptDir, "script.manifest"))
	if err != nil {
		return exec.Result{}, err
	}

	reconciled, err := handshake.Reconcile(rt, core, scr)
	if err != nil {
		var he *handshake.Error
		if ok := asHandshake(err, &he); ok {
			return exec.Result{
				Status:       "HANDSHAKE_FAILED",
				ErrorCode:    he.Code,
				ErrorMessage: he.Message,
				Logs:         []exec.LogLine{},
			}, nil
		}
		return exec.Result{}, err
	}

	// Языковой адаптер: python + доставка шима через PYTHONPATH.
	shimDir := filepath.Join(filepath.Dir(corePath), "sdk", "python")
	spec := exec.Spec{
		Dir:            scriptDir,
		Command:        "python",
		Args:           []string{scr.Entry},
		Env:            []string{"PYTHONPATH=" + shimDir},
		Protocol:       reconciled.Protocol,
		CoreAPI:        reconciled.CoreAPI,
		ScriptArgs:     []string{},
		StartupTimeout: 10 * time.Second,
		RunTimeout:     60 * time.Second,
	}
	return exec.Run(spec), nil
}

func asHandshake(err error, target **handshake.Error) bool {
	he, ok := err.(*handshake.Error)
	if ok {
		*target = he
	}
	return ok
}

func main() {
	runtimePath := flag.String("runtime", "runtime/runtime.manifest", "path to runtime manifest")
	corePath := flag.String("core", "cores/regular/core.manifest", "path to core manifest")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wire [--runtime path] [--core path] <script-dir>")
		os.Exit(2)
	}
	res, err := run(*runtimePath, *corePath, flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
	if res.Status != exec.StatusOK {
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Запустить сквозные тесты — зелёные**

Run: `cd /d/Projects/wire-auto && go test ./runtime/cmd/wire/`
Expected: PASS (`TestEndToEndHello`, `TestEndToEndCapabilityDenied`).

- [ ] **Step 5: Прогнать всё и проверить сборку**

Run: `cd /d/Projects/wire-auto && go build ./... && go test ./... && go run ./runtime/cmd/wire ./scripts/examples/hello`
Expected: `go test ./...` — всё PASS; `go run` печатает JSON со `"Status": "OK"` и логом `hello from python`.

- [ ] **Step 6: Commit**

```bash
cd /d/Projects/wire-auto && git add runtime/cmd/ && git commit -m "feat(cmd): wire CLI orchestrating manifests, handshake and exec"
```

---

## Self-Review

**Spec coverage:**
- §2 три манифеста → Task 1 (типы+загрузка), Task 5 (реальные файлы). ✔
- §3 рукопожатие + все 5 кодов → Task 2. ✔
- §4 протокол JSON Lines + набор сообщений → Task 3; правило «request запрещён» → Task 4 (тест). ✔
- §5 жизненный цикл (spawn→hello→ready→run→finish) + таймауты → Task 4. ✔
- §6 статусы результата → Task 4 (константы+тесты), HANDSHAKE_FAILED → Task 6. ✔
- §7 границы (нет железа, один python-адаптер, локально) → соблюдено во всех задачах. ✔
- §8 раскладка кода → структура файлов задач совпадает. ✔
- §9 критерии готовности → Task 6 Step 5 (hello), Task 6 (capability denied), Task 4 (run timeout, request violation). Критерий §9 «CORE_API_MISMATCH до спавна» покрыт юнит-тестом Task 2 (`TestReconcileCoreAPIMismatch`). ✔

**Placeholder scan:** плейсхолдеров нет.

**Type consistency:** `exec.Result`/`exec.LogLine`/`exec.Spec`, `handshake.Reconciled`/`handshake.Error`, `protocol.Message` и константы типов согласованы между Task 4 и Task 6. `run()` возвращает `exec.Result` с `Logs: []exec.LogLine{}` во всех ветках. ✔

# Duplex net/sys capabilities — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Расширить реестр возможностей duplex-ядра стартовым паком из 6 stdlib-провайдеров (4 сетевых + 2 системных), чтобы скрипты могли прозванивать порты и снимать базовую инфу о системе.

**Architecture:** Реестр `capreg.Default` разбивается по категориям на отдельные файлы; каждая capability — маленький синхронный `Handler`. Ключи реестра автоматически становятся `provides` (`main.go: providesFromRegistry`), так что `core.manifest` и `deview` не трогаем. Демо-скрипт `port-scan` говорит на протоколе напрямую, как `env-report`.

**Tech Stack:** Go (только stdlib: `net`, `runtime`, `os`, `context`), Python (демо-скрипт, без SDK), JSON Lines протокол v2.

## Global Constraints

- **Только stdlib.** Никаких внешних Go-зависимостей, никакого per-OS кода/build-тегов. Собирается на всех платформах из коробки.
- **Независимая сборка модуля:** каждая проверка гоняется как `cd cores/duplex && GOWORK=off go build ./... && go vet ./... && go test ./...`.
- **Сигнатура Handler не меняется:** `func(params json.RawMessage) (result json.RawMessage, code string, err error)`. Ошибка capability уезжает в `response.code` и не роняет прогон.
- **Сетевые «отказ/таймаут» — это не ошибка capability**, а нормальный `result` (`status: closed|filtered`). `BAD_PARAMS` — только для битых/неполных входных params.
- **Таймауты:** `timeout_ms` дефолт 1000, потолок 10000; `read_bytes` дефолт 256, потолок 4096. Значения вне диапазона зажимаются (не ошибка).
- **Git-дисциплина (CLAUDE.md, жёсткое правило):** коммиты — только по явному указанию пользователя; git-запись настроена спрашивать. Шаги «Commit» выполняются с ведома пользователя, не автоматически.
- **Ограничение по дизайну:** `dispatchRequest` синхронный (`exec.go:225`) → прозвон последовательный. Конкурентный диспатч — вне этого плана.

Спек: `docs/superpowers/specs/2026-08-17-duplex-net-sys-capabilities-design.md`.

## File Structure

```
cores/duplex/internal/capreg/
├── capreg.go        // MODIFY: тип Handler + Default = merge(...) под-реестров
├── params.go        // CREATE: clampTimeout/clampReadBytes/badParams/okResult
├── params_test.go   // CREATE
├── env.go           // CREATE: перенос env.get + envCaps
├── net.go           // CREATE: net.resolve/tcp.connect/tcp.banner/interfaces + netCaps
├── net_test.go      // CREATE
├── sys.go           // CREATE: sys.info/sys.env.list + sysCaps
├── sys_test.go      // CREATE
└── capreg_test.go   // KEEP: существующие env-тесты остаются зелёными

scripts/examples/port-scan/
├── script.manifest  // CREATE
└── main.py          // CREATE

guide/demo/
├── capabilities.md      // CREATE
├── README.md            // MODIFY: строка индекса
└── writing-a-script.md  // MODIFY: пример вызова capability
```

---

### Task 1: params.go — общие хелперы разбора и таймаутов

**Files:**
- Create: `cores/duplex/internal/capreg/params.go`
- Test: `cores/duplex/internal/capreg/params_test.go`

**Interfaces:**
- Consumes: ничего.
- Produces:
  - `clampTimeout(ms int) int` — 0/отрицательное → `defaultTimeoutMS`; больше `maxTimeoutMS` → `maxTimeoutMS`.
  - `clampReadBytes(n int) int` — 0/отрицательное → `defaultReadBytes`; больше `maxReadBytes` → `maxReadBytes`.
  - `badParams(format string, args ...any) (json.RawMessage, string, error)` — возвращает `(nil, "BAD_PARAMS", fmt.Errorf(...))`.
  - `okResult(v any) (json.RawMessage, string, error)` — маршалит `v`; при ошибке — `(nil, "BAD_PARAMS", err)`.
  - Константы `defaultTimeoutMS=1000`, `maxTimeoutMS=10000`, `defaultReadBytes=256`, `maxReadBytes=4096`.

- [ ] **Step 1: Написать падающий тест**

`cores/duplex/internal/capreg/params_test.go`:
```go
package capreg

import "testing"

func TestClampTimeout(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, defaultTimeoutMS},
		{-5, defaultTimeoutMS},
		{500, 500},
		{999999, maxTimeoutMS},
	}
	for _, c := range cases {
		if got := clampTimeout(c.in); got != c.want {
			t.Errorf("clampTimeout(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestClampReadBytes(t *testing.T) {
	if clampReadBytes(0) != defaultReadBytes {
		t.Errorf("clampReadBytes(0) = %d, want %d", clampReadBytes(0), defaultReadBytes)
	}
	if clampReadBytes(1<<20) != maxReadBytes {
		t.Errorf("clampReadBytes(big) = %d, want %d", clampReadBytes(1<<20), maxReadBytes)
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что не компилируется/падает**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestClamp -v`
Expected: FAIL — `undefined: clampTimeout` (и др.).

- [ ] **Step 3: Реализовать params.go**

`cores/duplex/internal/capreg/params.go`:
```go
// Общие хелперы разбора params и политики таймаутов для хендлеров capreg.
package capreg

import (
	"encoding/json"
	"fmt"
)

const (
	defaultTimeoutMS = 1000
	maxTimeoutMS     = 10000
	defaultReadBytes = 256
	maxReadBytes     = 4096
)

// clampTimeout зажимает timeout_ms в (0, maxTimeoutMS]; 0/отрицательное → дефолт.
func clampTimeout(ms int) int {
	if ms <= 0 {
		return defaultTimeoutMS
	}
	if ms > maxTimeoutMS {
		return maxTimeoutMS
	}
	return ms
}

// clampReadBytes зажимает read_bytes в (0, maxReadBytes]; 0/отрицательное → дефолт.
func clampReadBytes(n int) int {
	if n <= 0 {
		return defaultReadBytes
	}
	if n > maxReadBytes {
		return maxReadBytes
	}
	return n
}

// badParams — единый способ вернуть код BAD_PARAMS с пояснением.
func badParams(format string, args ...any) (json.RawMessage, string, error) {
	return nil, "BAD_PARAMS", fmt.Errorf(format, args...)
}

// okResult маршалит успешный результат; при ошибке маршалинга → BAD_PARAMS.
func okResult(v any) (json.RawMessage, string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return nil, "BAD_PARAMS", err
	}
	return out, "", nil
}
```

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestClamp -v`
Expected: PASS (TestClampTimeout, TestClampReadBytes).

- [ ] **Step 5: Commit** (с ведома пользователя)

```bash
git add cores/duplex/internal/capreg/params.go cores/duplex/internal/capreg/params_test.go
git commit -m "feat(duplex/capreg): add params/timeout helpers"
```

---

### Task 2: Разбить реестр — capreg.go сборка + env.go

**Files:**
- Modify: `cores/duplex/internal/capreg/capreg.go`
- Create: `cores/duplex/internal/capreg/env.go`
- Test (существующий, менять не нужно): `cores/duplex/internal/capreg/capreg_test.go`

**Interfaces:**
- Consumes: ничего нового.
- Produces:
  - Тип `Handler` (без изменений).
  - `var Default map[string]Handler` — теперь собирается через `merge(envCaps)`.
  - `func merge(maps ...map[string]Handler) map[string]Handler`.
  - `var envCaps map[string]Handler` (в env.go) с ключом `env.get`.

- [ ] **Step 1: Переписать capreg.go на сборку из под-реестров**

`cores/duplex/internal/capreg/capreg.go` (полностью):
```go
// Package capreg — реестр обработчиков capability для рантайма duplex.
// Обработчик исполняет уже авторизованный запрос (гейт provides — в exec).
// Default собирается слиянием под-реестров по категориям (env/net/sys).
package capreg

import "encoding/json"

// Handler исполняет capability. Возвращает либо result (code==""), либо
// машиночитаемый code ошибки capability (+пояснение в err). Ошибка capability
// НЕ роняет прогон — она уезжает в response.code.
type Handler func(params json.RawMessage) (result json.RawMessage, code string, err error)

// Default — реестр v2. Ключи должны совпадать с core.provides (выводятся из них).
var Default = merge(envCaps)

// merge сливает под-реестры в один; при коллизии ключей побеждает последний.
func merge(maps ...map[string]Handler) map[string]Handler {
	out := make(map[string]Handler)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
```

- [ ] **Step 2: Создать env.go (перенос env.get дословно)**

`cores/duplex/internal/capreg/env.go`:
```go
package capreg

import (
	"encoding/json"
	"fmt"
	"os"
)

// envCaps — под-реестр возможностей работы с переменными окружения.
var envCaps = map[string]Handler{
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

- [ ] **Step 3: Запустить существующие тесты — должны остаться зелёными**

Run: `cd cores/duplex && GOWORK=off go build ./... && go test ./internal/capreg/ -v`
Expected: PASS — TestEnvGetFound, TestEnvGetNotFound, TestEnvGetBadParams, TestClamp*.

- [ ] **Step 4: Commit** (с ведома пользователя)

```bash
git add cores/duplex/internal/capreg/capreg.go cores/duplex/internal/capreg/env.go
git commit -m "refactor(duplex/capreg): split registry into per-category files"
```

---

### Task 3: net.resolve

**Files:**
- Create: `cores/duplex/internal/capreg/net.go`
- Create: `cores/duplex/internal/capreg/net_test.go`
- Modify: `cores/duplex/internal/capreg/capreg.go` (добавить `netCaps` в `merge`)

**Interfaces:**
- Consumes: `clampTimeout`, `badParams`, `okResult` (Task 1); `merge` (Task 2).
- Produces:
  - `var netCaps map[string]Handler` (ключ `net.resolve` пока).
  - `net.resolve`: params `{host string, timeout_ms int}` → result `{addrs []string}`; пустой host → `BAD_PARAMS`; ошибка резолва → код `RESOLVE_FAILED`.

- [ ] **Step 1: Написать падающие тесты**

`cores/duplex/internal/capreg/net_test.go`:
```go
package capreg

import (
	"encoding/json"
	"testing"
)

func TestNetResolveLocalhost(t *testing.T) {
	res, code, err := Default["net.resolve"](json.RawMessage(`{"host":"localhost"}`))
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		Addrs []string `json:"addrs"`
	}
	if e := json.Unmarshal(res, &out); e != nil {
		t.Fatal(e)
	}
	if len(out.Addrs) == 0 {
		t.Fatal("localhost resolved to no addresses")
	}
}

func TestNetResolveBadParams(t *testing.T) {
	_, code, _ := Default["net.resolve"](json.RawMessage(`{}`))
	if code != "BAD_PARAMS" {
		t.Fatalf("code=%q, want BAD_PARAMS", code)
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestNetResolve -v`
Expected: FAIL — `Default["net.resolve"]` = nil (паника) / undefined `netCaps`.

- [ ] **Step 3: Создать net.go с net.resolve**

`cores/duplex/internal/capreg/net.go`:
```go
// Под-реестр сетевых возможностей: резолв, TCP-проба, граб баннера, интерфейсы.
package capreg

import (
	"context"
	"encoding/json"
	"net"
	"time"
)

var netCaps = map[string]Handler{
	"net.resolve": netResolve,
}

func netResolve(params json.RawMessage) (json.RawMessage, string, error) {
	var p struct {
		Host      string `json:"host"`
		TimeoutMS int    `json:"timeout_ms"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Host == "" {
		return badParams("net.resolve requires a non-empty string field \"host\"")
	}
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(clampTimeout(p.TimeoutMS))*time.Millisecond)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, p.Host)
	if err != nil {
		return nil, "RESOLVE_FAILED", err
	}
	return okResult(map[string]any{"addrs": addrs})
}
```

- [ ] **Step 4: Подключить netCaps в Default**

В `cores/duplex/internal/capreg/capreg.go` заменить строку:
```go
var Default = merge(envCaps)
```
на:
```go
var Default = merge(envCaps, netCaps)
```

- [ ] **Step 5: Запустить — убедиться, что проходит**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestNetResolve -v`
Expected: PASS (TestNetResolveLocalhost, TestNetResolveBadParams).

- [ ] **Step 6: Commit** (с ведома пользователя)

```bash
git add cores/duplex/internal/capreg/net.go cores/duplex/internal/capreg/net_test.go cores/duplex/internal/capreg/capreg.go
git commit -m "feat(duplex/capreg): add net.resolve capability"
```

---

### Task 4: net.tcp.connect

**Files:**
- Modify: `cores/duplex/internal/capreg/net.go` (добавить хендлер + ключ в `netCaps`)
- Modify: `cores/duplex/internal/capreg/net_test.go`

**Interfaces:**
- Consumes: `clampTimeout`, `badParams`, `okResult`.
- Produces:
  - `net.tcp.connect`: params `{host string, port int, timeout_ms int}` → result `{status string, latency_ms int64}`; `status` ∈ `open|closed|filtered` (таймаут → `filtered`, прочий отказ → `closed`, успех → `open`). Некорректные host/port → `BAD_PARAMS`.

- [ ] **Step 1: Написать падающие тесты**

Добавить в `cores/duplex/internal/capreg/net_test.go` (импорты `net`, `strconv` понадобятся — обнови блок import):
```go
func TestNetTCPConnectOpen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	params := json.RawMessage(`{"host":"127.0.0.1","port":` + port + `}`)
	res, code, err := Default["net.tcp.connect"](params)
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		Status string `json:"status"`
	}
	json.Unmarshal(res, &out)
	if out.Status != "open" {
		t.Fatalf("status=%q, want open", out.Status)
	}
}

func TestNetTCPConnectClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close() // порт свободен → connection refused
	params := json.RawMessage(`{"host":"127.0.0.1","port":` + port + `,"timeout_ms":500}`)
	res, code, _ := Default["net.tcp.connect"](params)
	if code != "" {
		t.Fatalf("code=%q", code)
	}
	var out struct {
		Status string `json:"status"`
	}
	json.Unmarshal(res, &out)
	if out.Status != "closed" {
		t.Fatalf("status=%q, want closed", out.Status)
	}
}

func TestNetTCPConnectFiltered(t *testing.T) {
	// 192.0.2.0/24 — TEST-NET-1 (RFC 5737), не маршрутизируется → таймаут.
	// Может быть флейки в сетях, где такой трафик режется с ICMP-отказом.
	params := json.RawMessage(`{"host":"192.0.2.1","port":80,"timeout_ms":300}`)
	res, code, _ := Default["net.tcp.connect"](params)
	if code != "" {
		t.Fatalf("code=%q", code)
	}
	var out struct {
		Status string `json:"status"`
	}
	json.Unmarshal(res, &out)
	if out.Status != "filtered" {
		t.Fatalf("status=%q, want filtered", out.Status)
	}
}

func TestNetTCPConnectBadParams(t *testing.T) {
	_, code, _ := Default["net.tcp.connect"](json.RawMessage(`{"host":"x","port":0}`))
	if code != "BAD_PARAMS" {
		t.Fatalf("code=%q, want BAD_PARAMS", code)
	}
}
```

Обнови import-блок net_test.go на:
```go
import (
	"encoding/json"
	"net"
	"testing"
)
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestNetTCPConnect -v`
Expected: FAIL — `Default["net.tcp.connect"]` = nil (паника).

- [ ] **Step 3: Реализовать хендлер**

В `cores/duplex/internal/capreg/net.go` добавить `strconv` в import и `"net.tcp.connect": netTCPConnect` в `netCaps`, затем функцию:
```go
func netTCPConnect(params json.RawMessage) (json.RawMessage, string, error) {
	var p struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		TimeoutMS int    `json:"timeout_ms"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Host == "" || p.Port <= 0 || p.Port > 65535 {
		return badParams("net.tcp.connect requires \"host\" and \"port\" in 1..65535")
	}
	timeout := time.Duration(clampTimeout(p.TimeoutMS)) * time.Millisecond
	addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		status := "closed"
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			status = "filtered"
		}
		return okResult(map[string]any{"status": status, "latency_ms": latency})
	}
	_ = conn.Close()
	return okResult(map[string]any{"status": "open", "latency_ms": latency})
}
```

Обновлённый `netCaps`:
```go
var netCaps = map[string]Handler{
	"net.resolve":     netResolve,
	"net.tcp.connect": netTCPConnect,
}
```

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestNetTCPConnect -v`
Expected: PASS (Open, Closed, Filtered, BadParams). Если Filtered флейкнул из-за сети — перезапусти; поведение зависит от окружения (см. комментарий в тесте).

- [ ] **Step 5: Commit** (с ведома пользователя)

```bash
git add cores/duplex/internal/capreg/net.go cores/duplex/internal/capreg/net_test.go
git commit -m "feat(duplex/capreg): add net.tcp.connect port probe"
```

---

### Task 5: net.tcp.banner

**Files:**
- Modify: `cores/duplex/internal/capreg/net.go`
- Modify: `cores/duplex/internal/capreg/net_test.go`

**Interfaces:**
- Consumes: `clampTimeout`, `clampReadBytes`, `badParams`, `okResult`.
- Produces:
  - `net.tcp.banner`: params `{host string, port int, timeout_ms int, read_bytes int}` → result `{banner string, bytes int}`; неудача коннекта → код `CONNECT_FAILED`; частичное чтение/таймаут чтения — не ошибка (что успели прочитать, то и вернём).

- [ ] **Step 1: Написать падающий тест**

Добавить в `cores/duplex/internal/capreg/net_test.go`:
```go
func TestNetTCPBanner(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, e := ln.Accept()
		if e != nil {
			return
		}
		c.Write([]byte("SSH-2.0-test"))
		c.Close()
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	params := json.RawMessage(`{"host":"127.0.0.1","port":` + port + `}`)
	res, code, err := Default["net.tcp.banner"](params)
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		Banner string `json:"banner"`
		Bytes  int    `json:"bytes"`
	}
	json.Unmarshal(res, &out)
	if out.Banner != "SSH-2.0-test" || out.Bytes != len("SSH-2.0-test") {
		t.Fatalf("banner=%q bytes=%d", out.Banner, out.Bytes)
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestNetTCPBanner -v`
Expected: FAIL — `Default["net.tcp.banner"]` = nil (паника).

- [ ] **Step 3: Реализовать хендлер**

В `net.go` добавить `"net.tcp.banner": netTCPBanner` в `netCaps` и функцию:
```go
func netTCPBanner(params json.RawMessage) (json.RawMessage, string, error) {
	var p struct {
		Host      string `json:"host"`
		Port      int    `json:"port"`
		TimeoutMS int    `json:"timeout_ms"`
		ReadBytes int    `json:"read_bytes"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Host == "" || p.Port <= 0 || p.Port > 65535 {
		return badParams("net.tcp.banner requires \"host\" and \"port\" in 1..65535")
	}
	timeout := time.Duration(clampTimeout(p.TimeoutMS)) * time.Millisecond
	addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, "CONNECT_FAILED", err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, clampReadBytes(p.ReadBytes))
	n, _ := conn.Read(buf) // таймаут/EOF/частичное чтение — не ошибка capability
	return okResult(map[string]any{"banner": string(buf[:n]), "bytes": n})
}
```

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestNetTCPBanner -v`
Expected: PASS.

- [ ] **Step 5: Commit** (с ведома пользователя)

```bash
git add cores/duplex/internal/capreg/net.go cores/duplex/internal/capreg/net_test.go
git commit -m "feat(duplex/capreg): add net.tcp.banner grab"
```

---

### Task 6: net.interfaces

**Files:**
- Modify: `cores/duplex/internal/capreg/net.go`
- Modify: `cores/duplex/internal/capreg/net_test.go`

**Interfaces:**
- Consumes: `okResult`.
- Produces:
  - `net.interfaces`: params `{}` → result `{interfaces []{name string, mac string, addrs []string, flags []string}}`; системная ошибка → код `INTERFACES_FAILED`.

- [ ] **Step 1: Написать падающий тест**

Добавить в `net_test.go`:
```go
func TestNetInterfaces(t *testing.T) {
	res, code, err := Default["net.interfaces"](json.RawMessage(`{}`))
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		Interfaces []struct {
			Name  string   `json:"name"`
			Addrs []string `json:"addrs"`
			Flags []string `json:"flags"`
		} `json:"interfaces"`
	}
	json.Unmarshal(res, &out)
	if len(out.Interfaces) == 0 {
		t.Fatal("no network interfaces reported")
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestNetInterfaces -v`
Expected: FAIL — `Default["net.interfaces"]` = nil (паника).

- [ ] **Step 3: Реализовать хендлер**

В `net.go` добавить `"net.interfaces": netInterfaces` в `netCaps` и функцию:
```go
func netInterfaces(params json.RawMessage) (json.RawMessage, string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, "INTERFACES_FAILED", err
	}
	type ifaceOut struct {
		Name  string   `json:"name"`
		MAC   string   `json:"mac"`
		Addrs []string `json:"addrs"`
		Flags []string `json:"flags"`
	}
	flagNames := []struct {
		flag net.Flags
		name string
	}{
		{net.FlagUp, "up"},
		{net.FlagLoopback, "loopback"},
		{net.FlagBroadcast, "broadcast"},
		{net.FlagPointToPoint, "pointtopoint"},
		{net.FlagMulticast, "multicast"},
	}
	out := make([]ifaceOut, 0, len(ifaces))
	for _, i := range ifaces {
		addrs := []string{}
		if aa, e := i.Addrs(); e == nil {
			for _, a := range aa {
				addrs = append(addrs, a.String())
			}
		}
		flags := []string{}
		for _, f := range flagNames {
			if i.Flags&f.flag != 0 {
				flags = append(flags, f.name)
			}
		}
		out = append(out, ifaceOut{Name: i.Name, MAC: i.HardwareAddr.String(), Addrs: addrs, Flags: flags})
	}
	return okResult(map[string]any{"interfaces": out})
}
```

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestNetInterfaces -v`
Expected: PASS.

- [ ] **Step 5: Commit** (с ведома пользователя)

```bash
git add cores/duplex/internal/capreg/net.go cores/duplex/internal/capreg/net_test.go
git commit -m "feat(duplex/capreg): add net.interfaces"
```

---

### Task 7: sys.info + sys.env.list

**Files:**
- Create: `cores/duplex/internal/capreg/sys.go`
- Create: `cores/duplex/internal/capreg/sys_test.go`
- Modify: `cores/duplex/internal/capreg/capreg.go` (добавить `sysCaps` в `merge`)

**Interfaces:**
- Consumes: `badParams`, `okResult`, `merge`.
- Produces:
  - `var sysCaps map[string]Handler` (ключи `sys.info`, `sys.env.list`).
  - `sys.info`: params `{}` → result `{os, arch, hostname string, numCPU int, goVersion string}`.
  - `sys.env.list`: params `{prefix string}` (опционально) → result `{names []string}`; битый непустой JSON → `BAD_PARAMS`.

- [ ] **Step 1: Написать падающие тесты**

`cores/duplex/internal/capreg/sys_test.go`:
```go
package capreg

import (
	"encoding/json"
	"runtime"
	"testing"
)

func TestSysInfo(t *testing.T) {
	res, code, err := Default["sys.info"](json.RawMessage(`{}`))
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		OS     string `json:"os"`
		Arch   string `json:"arch"`
		NumCPU int    `json:"numCPU"`
	}
	json.Unmarshal(res, &out)
	if out.OS != runtime.GOOS || out.Arch != runtime.GOARCH || out.NumCPU < 1 {
		t.Fatalf("info = %+v", out)
	}
}

func TestSysEnvListPrefix(t *testing.T) {
	t.Setenv("WIRE_TEST_LISTVAR", "x")
	res, code, err := Default["sys.env.list"](json.RawMessage(`{"prefix":"WIRE_TEST_"}`))
	if code != "" || err != nil {
		t.Fatalf("code=%q err=%v", code, err)
	}
	var out struct {
		Names []string `json:"names"`
	}
	json.Unmarshal(res, &out)
	found := false
	for _, n := range out.Names {
		if n == "WIRE_TEST_LISTVAR" {
			found = true
		}
	}
	if !found {
		t.Fatalf("WIRE_TEST_LISTVAR not found in %v", out.Names)
	}
}

func TestSysEnvListBadParams(t *testing.T) {
	_, code, _ := Default["sys.env.list"](json.RawMessage(`{bad`))
	if code != "BAD_PARAMS" {
		t.Fatalf("code=%q, want BAD_PARAMS", code)
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestSys -v`
Expected: FAIL — `Default["sys.info"]` = nil (паника) / undefined `sysCaps`.

- [ ] **Step 3: Создать sys.go**

`cores/duplex/internal/capreg/sys.go`:
```go
// Под-реестр системных возможностей: инфо о хосте и список переменных окружения.
package capreg

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
)

var sysCaps = map[string]Handler{
	"sys.info":     sysInfo,
	"sys.env.list": sysEnvList,
}

func sysInfo(params json.RawMessage) (json.RawMessage, string, error) {
	host, _ := os.Hostname()
	return okResult(map[string]any{
		"os":        runtime.GOOS,
		"arch":      runtime.GOARCH,
		"hostname":  host,
		"numCPU":    runtime.NumCPU(),
		"goVersion": runtime.Version(),
	})
}

func sysEnvList(params json.RawMessage) (json.RawMessage, string, error) {
	var p struct {
		Prefix string `json:"prefix"`
	}
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return badParams("sys.env.list: invalid params")
		}
	}
	names := []string{}
	for _, kv := range os.Environ() {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if p.Prefix == "" || strings.HasPrefix(name, p.Prefix) {
			names = append(names, name)
		}
	}
	return okResult(map[string]any{"names": names})
}
```

- [ ] **Step 4: Подключить sysCaps в Default**

В `cores/duplex/internal/capreg/capreg.go` заменить:
```go
var Default = merge(envCaps, netCaps)
```
на:
```go
var Default = merge(envCaps, netCaps, sysCaps)
```

- [ ] **Step 5: Запустить — убедиться, что проходит**

Run: `cd cores/duplex && GOWORK=off go test ./internal/capreg/ -run TestSys -v`
Expected: PASS (SysInfo, SysEnvListPrefix, SysEnvListBadParams).

- [ ] **Step 6: Полная проверка модуля**

Run: `cd cores/duplex && GOWORK=off go build ./... && go vet ./... && go test ./...`
Expected: всё зелёное (весь модуль, включая exec/bridge/handshake).

- [ ] **Step 7: Commit** (с ведома пользователя)

```bash
git add cores/duplex/internal/capreg/sys.go cores/duplex/internal/capreg/sys_test.go cores/duplex/internal/capreg/capreg.go
git commit -m "feat(duplex/capreg): add sys.info and sys.env.list"
```

---

### Task 8: Демо-скрипт port-scan

**Files:**
- Create: `scripts/examples/port-scan/script.manifest`
- Create: `scripts/examples/port-scan/main.py`

**Interfaces:**
- Consumes: возможности `net.resolve`, `net.tcp.connect` (ключи из Task 3/4).
- Produces: скрипт для discovery; итог `done.result = {host, open:[...]}`.

- [ ] **Step 1: Создать script.manifest**

`scripts/examples/port-scan/script.manifest`:
```toml
name = "port-scan"
version = "0.1.0"
core = "duplex"
coreApi = 1
capabilities = ["net.resolve", "net.tcp.connect"]
link = "stdio"
language = "python"
cmd = ["python", "main.py"]
```

- [ ] **Step 2: Создать main.py**

`scripts/examples/port-scan/main.py`:
```python
import sys, json

def send(o): sys.stdout.write(json.dumps(o, separators=(",", ":")) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())

HOST = "127.0.0.1"
PORTS = [22, 80, 135, 443, 445, 3389, 8080, 8443]

_id = 0
def request(cap, params):
    global _id
    _id += 1
    send({"type": "request", "id": str(_id), "capability": cap, "params": params})
    return recv()

recv()  # hello — говорим на протоколе напрямую, без SDK
send({"type": "ready"})

r = request("net.resolve", {"host": HOST})
addrs = r.get("result", {}).get("addrs") or [HOST]
target = addrs[0]
send({"type": "log", "level": "info", "message": "scanning %s (%s)" % (HOST, target)})

open_ports = []
for port in PORTS:
    resp = request("net.tcp.connect", {"host": target, "port": port, "timeout_ms": 500})
    if resp.get("code"):
        send({"type": "log", "level": "error", "message": "port %d: %s" % (port, resp["code"])})
        continue
    res = resp.get("result", {})
    if res.get("status") == "open":
        open_ports.append(port)
        send({"type": "log", "level": "info",
              "message": "port %d OPEN (%dms)" % (port, res.get("latency_ms", 0))})

send({"type": "log", "level": "info", "message": "open ports: %s" % (open_ports or "none")})
send({"type": "done", "exitCode": 0, "result": {"host": target, "open": open_ports}})
```

- [ ] **Step 3: Прогнать скрипт через deview и убедиться, что отрабатывает**

`cores/duplex/cmd/core` — стриминговый мост (флаги `-core`/`-scripts`), не одиночный
запуск; скрипты гоняются через клиент. Основной путь — интерактивный deview:

Run: `go run ./apps/deview/cmd/deview` — в меню выбрать `port-scan`.

Без TTY можно подать мосту команды `list`/`run` пайпом JSON-строк на stdin ядра
(формат команд — в `guide/demo/running.md` и `guide/demo/app-core-bridge.md`); флага
одиночного запуска у duplex нет.

Expected: в живом логе строка `open ports: ...`, итоговое событие
`{"type":"result","status":"OK","exitCode":0}`. Список открытых портов зависит от
локальной машины (может быть `none` — это валидно).

- [ ] **Step 4: Commit** (с ведома пользователя)

```bash
git add scripts/examples/port-scan/script.manifest scripts/examples/port-scan/main.py
git commit -m "feat(scripts): add port-scan example using net capabilities"
```

---

### Task 9: Документация

**Files:**
- Create: `guide/demo/capabilities.md`
- Modify: `guide/demo/README.md` (строка индекса)
- Modify: `guide/demo/writing-a-script.md` (пример вызова capability)

**Interfaces:**
- Consumes: имена и контракты возможностей из Task 3–7.
- Produces: топик-файл + ссылка из индекса.

- [ ] **Step 1: Создать guide/demo/capabilities.md**

`guide/demo/capabilities.md`:
```markdown
# Возможности ядра (capabilities)

Ядро — провайдер примитивов, скрипт — оркестратор. Скрипт объявляет нужные
возможности в `script.capabilities`, они проходят гейт на рукопожатии и второй
гейт в `exec.dispatchRequest`, после чего вызываются через канал `request`/`response`.

## Реестр (capreg)

Каждое ядро хранит реестр `capreg.Default` — карту `capability → Handler`, собранную
слиянием под-реестров по категориям (`env` / `net` / `sys`). Ключи реестра —
источник правды для `provides`; `core.manifest` их не дублирует.

## Стартовый пак duplex

### env
| capability | params | result |
|---|---|---|
| `env.get` | `{name}` | `{value}` (или код `ENV_NOT_FOUND`) |

### net
| capability | params | result |
|---|---|---|
| `net.resolve` | `{host, timeout_ms?}` | `{addrs:[…]}` (или код `RESOLVE_FAILED`) |
| `net.tcp.connect` | `{host, port, timeout_ms?}` | `{status, latency_ms}`, `status` ∈ `open\|closed\|filtered` |
| `net.tcp.banner` | `{host, port, timeout_ms?, read_bytes?}` | `{banner, bytes}` (или код `CONNECT_FAILED`) |
| `net.interfaces` | `{}` | `{interfaces:[{name, mac, addrs, flags}]}` |

Классификация `net.tcp.connect`: соединение → `open`; отказ (RST) → `closed`;
таймаут → `filtered`.

### sys
| capability | params | result |
|---|---|---|
| `sys.info` | `{}` | `{os, arch, hostname, numCPU, goVersion}` |
| `sys.env.list` | `{prefix?}` | `{names:[…]}` (значения — через `env.get`) |

## Таймауты

`timeout_ms` — дефолт 1000, потолок 10000 (значения вне диапазона зажимаются).
`read_bytes` — дефолт 256, потолок 4096.

## Ограничение: последовательный диспатч

`dispatchRequest` вызывается синхронно в главном цикле `exec.Run`. Значит запросы
обслуживаются по одному: медленный `net.tcp.connect` (таймаут → `filtered`)
блокирует цикл до возврата. Прозвон большого диапазона портов — последовательный.
Конкурентный диспатч (горутина на запрос) — будущая работа.

## Пример: прозвон портов

См. `scripts/examples/port-scan/` — резолвит хост через `net.resolve`, крутит
цикл по портам через `net.tcp.connect`, собирает открытые в `done.result.open`.
```

- [ ] **Step 2: Добавить строку в индекс guide/demo/README.md**

В таблицу «Содержание» `guide/demo/README.md` добавить строку (после строки `cores.md`):
```markdown
| [capabilities.md](capabilities.md) | Реестр возможностей ядра: стартовый пак env/net/sys, params/result каждой capability, политика таймаутов, ограничение последовательного диспатча, пример прозвона портов |
```

- [ ] **Step 3: Дополнить writing-a-script.md примером вызова capability**

В `guide/demo/writing-a-script.md` в конце раздела «Правила написания скриптов» добавить:
```markdown
## Вызов возможностей ядра

Скрипт дёргает примитив ядра сообщением `request` и читает `response`:

```python
send({"type": "request", "id": "1", "capability": "net.tcp.connect",
      "params": {"host": "127.0.0.1", "port": 80, "timeout_ms": 500}})
resp = recv()
status = resp.get("result", {}).get("status")  # open | closed | filtered
```

Возможность должна быть объявлена в `script.capabilities`, иначе придёт
`response.code = "CAPABILITY_DENIED"`. Полный список — в [capabilities.md](capabilities.md).
```

- [ ] **Step 4: Commit** (с ведома пользователя)

```bash
git add guide/demo/capabilities.md guide/demo/README.md guide/demo/writing-a-script.md
git commit -m "docs(guide): document capability registry and net/sys starter pack"
```

---

## Итоговая проверка

- [ ] `cd cores/duplex && GOWORK=off go build ./... && go vet ./... && go test ./...` — зелёное.
- [ ] `Default` содержит 7 ключей: `env.get`, `net.resolve`, `net.tcp.connect`, `net.tcp.banner`, `net.interfaces`, `sys.info`, `sys.env.list`.
- [ ] Демо `port-scan` находится discovery и отрабатывает через `deview` (итог `status:OK`).
- [ ] `guide/demo/capabilities.md` создан и слинкован из индекса.

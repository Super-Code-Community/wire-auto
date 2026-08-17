# Duplex interactive prompts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Дать скрипту запрашивать у человека строку ввода по ходу выполнения через дуплекс-канал: `prompt` (скрипт→ядро) пробрасывается наверх клиенту, deview спрашивает пользователя, значение возвращается скрипту как `response`.

**Architecture:** Полный релей `deview → ядро → скрипт`. `prompt` — отдельная сущность, НЕ capability (не гейтится, не трогает capreg/handshake). В `exec.Run` ответ клиента приходит по новому каналу `Spec.Answers`, интегрированному в главный `select` (переиспользует существующие таймаут/отмену). Мост добавляет событие `prompt` (вверх) и команду `input` (вниз); deview показывает запрос и читает строку.

**Tech Stack:** Go (stdlib), Python (демо + testdata, без SDK), JSON Lines протокол v2. Модули: `cores/duplex`, `apps/deview`.

## Global Constraints

- **Только stdlib** в Go; без внешних зависимостей.
- **Prompt — НЕ capability:** не проходит гейт рукопожатия, не пишется в `script.capabilities`, не трогает `capreg`.
- **Один outstanding prompt за раз** (скрипт синхронный: отправил → ждёт `response`).
- **Канал ответов буферизован cap 1** (`make(chan exec.PromptAnswer, 1)`), чтобы отправка со стороны моста не блокировала.
- **Политика «нет ответа»:** prompt блокирует до ответа клиента либо до отмены/`RunTimeout` (существующие механизмы). Отдельного таймаута на prompt нет.
- **Wire-контракт:** скрипт→ядро `{"type":"prompt","id","message"}`; ядро→скрипт `{"type":"response","id","result":{"value"}}`; ядро→клиент событие `{"type":"prompt","id","message"}`; клиент→ядро команда `{"type":"input","id","value"}`.
- **Git (CLAUDE.md, жёсткое правило):** коммиты — ТОЛЬКО по явному указанию пользователя. Шаги «Commit» в этом плане **пропускаются** — реализуем+тестируем, изменения остаются в рабочем дереве.
- **Абсолютные Windows-пути** во всех файловых операциях.
- **Независимая сборка модулей:** `cd <module> && GOWORK=off go build ./... && go vet ./... && go test ./...` для `cores/duplex` и `apps/deview`.

Спек: `docs/superpowers/specs/2026-08-17-duplex-interactive-prompts-design.md`.

## File Structure

```
cores/duplex/internal/protocol/protocol.go   // MODIFY: + TypePrompt const
cores/duplex/internal/exec/event.go          // MODIFY: Event += ID
cores/duplex/internal/exec/exec.go           // MODIFY: PromptAnswer, Spec.Answers, loop handling
cores/duplex/internal/exec/testdata/ask/main.py        // CREATE
cores/duplex/internal/exec/testdata/ask-early/main.py  // CREATE
cores/duplex/internal/exec/exec_test.go      // MODIFY: + prompt tests
cores/duplex/internal/bridge/message.go      // MODIFY: Event += ID; Command += ID,Value
cores/duplex/internal/bridge/serve.go        // MODIFY: Deps.Run sig, answers, input case, prompt event
cores/duplex/internal/bridge/serve_test.go   // MODIFY: fix 2 Run closures + TestServePrompt
cores/duplex/cmd/core/main.go                // MODIFY: runStreaming answers param, wiring
cores/duplex/cmd/core/testdata/ask/{script.manifest,main.py}  // CREATE
cores/duplex/cmd/core/main_test.go           // MODIFY: + TestE2EPromptViaBridge
apps/deview/internal/bridge/message.go       // MODIFY: Event += ID; Command += ID,Value
apps/deview/internal/bridge/client.go        // MODIFY: Run prompt case + SendInput
apps/deview/internal/bridge/client_test.go   // MODIFY: + input/prompt tests
apps/deview/cmd/deview/main.go               // MODIFY: prompt handler reads line + SendInput
scripts/examples/ask-name/{script.manifest,main.py}  // CREATE
guide/demo/prompts.md                        // CREATE
guide/demo/README.md, guide/README.md        // MODIFY: index rows
guide/demo/protocol.md, guide/demo/app-core-bridge.md  // MODIFY: notes
```

---

### Task 1: protocol const + exec prompt handling (ядро↔скрипт)

**Files:**
- Modify: `cores/duplex/internal/protocol/protocol.go`
- Modify: `cores/duplex/internal/exec/event.go`
- Modify: `cores/duplex/internal/exec/exec.go`
- Create: `cores/duplex/internal/exec/testdata/ask/main.py`
- Create: `cores/duplex/internal/exec/testdata/ask-early/main.py`
- Test: `cores/duplex/internal/exec/exec_test.go`

**Interfaces:**
- Consumes: existing `protocol.Message` (fields `ID`, `Message`, `Result` already present), existing `exec.Run`/`Spec`.
- Produces:
  - `protocol.TypePrompt = "prompt"`.
  - `exec.Event` field `ID string`; event `Kind:"prompt"`.
  - `type PromptAnswer struct{ ID, Value string }`; `Spec.Answers <-chan PromptAnswer`.
  - On `prompt`: exec emits `Event{Kind:"prompt",ID,Message}`, awaits `Answers`, writes `response{id,result:{value}}`.

- [ ] **Step 1: Написать testdata-скрипты и падающие тесты**

Create `cores/duplex/internal/exec/testdata/ask/main.py`:
```python
import sys, json
def send(o): sys.stdout.write(json.dumps(o) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())
recv()  # hello
send({"type": "ready"})
send({"type": "prompt", "id": "1", "message": "name?"})
resp = recv()
name = resp.get("result", {}).get("value", "?")
send({"type": "log", "level": "info", "message": "hello " + name})
send({"type": "done", "exitCode": 0, "result": {"name": name}})
```

Create `cores/duplex/internal/exec/testdata/ask-early/main.py`:
```python
import sys, json
def send(o): sys.stdout.write(json.dumps(o) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())
recv()  # hello
send({"type": "prompt", "id": "1", "message": "too early"})  # before ready → violation
```

Add to `cores/duplex/internal/exec/exec_test.go`:
```go
func TestPromptRoundTrip(t *testing.T) {
	spec := v2Spec("testdata/ask")
	answers := make(chan PromptAnswer, 1)
	spec.Answers = answers
	var got Event
	spec.OnEvent = func(e Event) {
		if e.Kind == "prompt" {
			got = e
			answers <- PromptAnswer{ID: e.ID, Value: "Alice"}
		}
	}
	res := Run(context.Background(), spec)
	if res.Status != StatusOK {
		t.Fatalf("status=%s err=%s", res.Status, res.ErrorMessage)
	}
	if got.Kind != "prompt" || got.ID != "1" || got.Message != "name?" {
		t.Fatalf("prompt event=%+v", got)
	}
	if len(res.Logs) != 1 || !strings.Contains(res.Logs[0].Message, "hello Alice") {
		t.Fatalf("logs=%+v", res.Logs)
	}
}

func TestPromptBeforeReadyIsViolation(t *testing.T) {
	res := Run(context.Background(), v2Spec("testdata/ask-early"))
	if res.Status != StatusProtocolViolation {
		t.Fatalf("status=%s, want PROTOCOL_VIOLATION", res.Status)
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd cores/duplex && GOWORK=off go test ./internal/exec/ -run TestPrompt -v`
Expected: FAIL to compile — `spec.Answers` undefined, `PromptAnswer` undefined, `Event.ID` undefined.

- [ ] **Step 3: protocol.go — добавить константу**

In `cores/duplex/internal/protocol/protocol.go`, add `TypePrompt` to the const block:
```go
	TypeRequest  = "request"
	TypeResponse = "response"
	TypePrompt   = "prompt"
```

- [ ] **Step 4: exec/event.go — поле ID**

Replace the whole `cores/duplex/internal/exec/event.go`:
```go
package exec

// Event — единица живого потока прогона наружу (в мост/клиент).
// Kind = "ready" (скрипт поднялся), "log" (строка вывода) либо
// "prompt" (скрипт просит у человека строку ввода; ID коррелирует ответ).
type Event struct {
	Kind    string
	ID      string
	Level   string
	Message string
}
```

- [ ] **Step 5: exec/exec.go — тип, поле Spec, обработка prompt**

5a. Add `PromptAnswer` type and `Answers` field. In `cores/duplex/internal/exec/exec.go`, replace the tail of the `Spec` struct:
```go
	// v2: авторизованные capability и реестр обработчиков.
	Provides []string
	Registry map[string]capreg.Handler
}
```
with:
```go
	// v2: авторизованные capability и реестр обработчиков.
	Provides []string
	Registry map[string]capreg.Handler
	// prompt: канал ответов клиента на интерактивные запросы скрипта.
	Answers <-chan PromptAnswer
}

// PromptAnswer — ответ клиента на prompt скрипта (коррелируется по ID).
type PromptAnswer struct {
	ID    string
	Value string
}
```

5b. Declare the await-state vars. Right after `deadline := time.After(spec.StartupTimeout)` add:
```go
	var answers <-chan PromptAnswer // nil, пока не ждём ответ на prompt
	var promptID string
```

5c. Add the answer-delivery case to the MAIN select. Immediately after the `case <-ctx.Done():` block closes (before `case ev := <-ch:`), insert:
```go
		case ans := <-answers:
			result, _ := json.Marshal(map[string]string{"value": ans.Value})
			_ = protocol.Encode(stdin, protocol.Message{Type: protocol.TypeResponse, ID: promptID, Result: result})
			answers = nil // ответ доставлен — перестаём ждать
```

5d. Add the `prompt` message case. In the `switch ev.msg.Type` block, after the `case protocol.TypeRequest:` block and before `case protocol.TypeDone:`, insert:
```go
			case protocol.TypePrompt:
				if spec.Protocol < 2 {
					return kill(StatusProtocolViolation, StatusProtocolViolation, "prompt not allowed on protocol 1")
				}
				if !gotReady {
					return kill(StatusProtocolViolation, StatusProtocolViolation, "prompt before ready")
				}
				emit(Event{Kind: "prompt", ID: ev.msg.ID, Message: ev.msg.Message})
				promptID = ev.msg.ID
				answers = spec.Answers // начинаем ждать ответ клиента
```

- [ ] **Step 6: Запустить — убедиться, что проходит**

Run: `cd cores/duplex && GOWORK=off go test ./internal/exec/ -run TestPrompt -v`
Expected: PASS (TestPromptRoundTrip, TestPromptBeforeReadyIsViolation).
Then confirm no regressions: `cd cores/duplex && GOWORK=off go test ./internal/exec/ ./internal/protocol/`
Expected: all PASS.

- [ ] **Step 7: Commit** — SKIP (проектное правило: не коммитим).

---

### Task 2: bridge (cores/duplex) — событие prompt вверх, команда input вниз

**Files:**
- Modify: `cores/duplex/internal/bridge/message.go`
- Modify: `cores/duplex/internal/bridge/serve.go`
- Test: `cores/duplex/internal/bridge/serve_test.go`

**Interfaces:**
- Consumes: `exec.PromptAnswer`, `exec.Event.ID` (Task 1).
- Produces:
  - `bridge.Event` field `ID`; `bridge.Command` fields `ID`, `Value`.
  - `Deps.Run` new signature: `func(ctx context.Context, dir string, onEvent func(exec.Event), answers <-chan exec.PromptAnswer) (exec.Result, error)`.
  - Serve routes command `input` → `answers`; onEvent `prompt` → event `{type:prompt,id,message}`.

- [ ] **Step 1: Написать падающий тест**

Add to `cores/duplex/internal/bridge/serve_test.go` (add `"encoding/json"` to its imports):
```go
func TestServePrompt(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"type":"run","dir":"scripts/ask"}`,
		`{"type":"input","id":"1","value":"Bob"}`,
		`{"type":"exit"}`,
	}, "\n") + "\n")
	var out syncBuffer

	deps := Deps{
		List: func() ([]Script, error) { return []Script{}, nil },
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event), answers <-chan exec.PromptAnswer) (exec.Result, error) {
			onEvent(exec.Event{Kind: "prompt", ID: "1", Message: "name?"})
			ans := <-answers
			return exec.Result{Status: exec.StatusOK, Result: json.RawMessage(`{"name":"` + ans.Value + `"}`)}, nil
		},
	}

	if err := Serve(in, &out, deps); err != nil {
		t.Fatalf("Serve error: %v", err)
	}
	evs := collectEvents(t, out.String())
	if len(evs) < 2 {
		t.Fatalf("ожидали prompt+result, got %+v", evs)
	}
	if evs[0].Type != "prompt" || evs[0].ID != "1" || evs[0].Message != "name?" {
		t.Fatalf("prompt event неверен: %+v", evs[0])
	}
	last := evs[len(evs)-1]
	if last.Type != "result" || last.Status != exec.StatusOK {
		t.Fatalf("result неверен: %+v", last)
	}
	if !strings.Contains(string(last.Result), "Bob") {
		t.Fatalf("result не содержит ввод: %s", string(last.Result))
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd cores/duplex && GOWORK=off go test ./internal/bridge/ -run TestServePrompt -v`
Expected: FAIL to compile — `exec.PromptAnswer` param not in `Deps.Run` type; `Event.ID`/`Command.ID`/`Command.Value` undefined.

- [ ] **Step 3: message.go — поля**

In `cores/duplex/internal/bridge/message.go`, update `Command` and `Event`:
```go
type Command struct {
	Type  string `json:"type"`
	Dir   string `json:"dir,omitempty"`
	ID    string `json:"id,omitempty"`
	Value string `json:"value,omitempty"`
}
```
and add `ID` to `Event` (after `Type`):
```go
type Event struct {
	Type         string          `json:"type"`
	ID           string          `json:"id,omitempty"`
	Scripts      []Script        `json:"scripts,omitempty"`
	Level        string          `json:"level,omitempty"`
	Message      string          `json:"message,omitempty"`
	Status       string          `json:"status,omitempty"`
	ExitCode     int             `json:"exitCode,omitempty"`
	ErrorCode    string          `json:"errorCode,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
}
```

- [ ] **Step 4: serve.go — сигнатура Deps.Run, канал answers, кейсы**

4a. Update `Deps.Run` type:
```go
type Deps struct {
	List func() ([]Script, error)
	Run  func(ctx context.Context, dir string, onEvent func(exec.Event), answers <-chan exec.PromptAnswer) (exec.Result, error)
}
```

4b. In the `case "run":` block, create the buffered answers channel and pass it into `deps.Run`; add the `prompt` case to `onEvent`. Replace the goroutine launch:
```go
		case "run":
			ctx, cancel := context.WithCancel(context.Background())
			cancelCur = cancel
			answers := make(chan exec.PromptAnswer, 1)
			go func(dir string) {
				defer func() {
					if r := recover(); r != nil {
						write(Event{Type: "error", Message: fmt.Sprintf("run panicked: %v", r)})
					}
					done <- runDone{}
				}()
				res, err := deps.Run(ctx, dir, func(ev exec.Event) {
					switch ev.Kind {
					case "ready":
						write(Event{Type: "ready"})
					case "log":
						write(Event{Type: "log", Level: ev.Level, Message: ev.Message})
					case "prompt":
						write(Event{Type: "prompt", ID: ev.ID, Message: ev.Message})
					}
				}, answers)
				if err != nil {
					write(Event{Type: "error", Message: err.Error()})
					return
				}
				write(Event{
					Type:         "result",
					Status:       res.Status,
					ExitCode:     res.ExitCode,
					ErrorCode:    res.ErrorCode,
					ErrorMessage: res.ErrorMessage,
					Result:       res.Result,
				})
			}(c.Dir)
```

4c. In the `waitLoop`'s inner `switch nc.Type` (during a run), add an `input` case alongside `cancel`/`run`:
```go
					case "cancel":
						if cancelCur != nil {
							cancelCur()
						}
					case "input":
						// Доставить ответ на prompt в exec. Буфер cap 1 не даёт
						// подвиснуть; <-done страхует, если прогон уже кончился.
						select {
						case answers <- exec.PromptAnswer{ID: nc.ID, Value: nc.Value}:
						case <-done:
						}
					case "run":
						write(Event{Type: "error", Message: "busy: a script is already running"})
```

- [ ] **Step 5: Запустить — убедиться, что проходит + нет регрессий в bridge**

Run: `cd cores/duplex && GOWORK=off go test ./internal/bridge/ -v`
Expected: FAIL to compile at first — the two EXISTING tests (`TestServeListThenRunThenExit`, `TestServeSurvivesFailedRun`) still use the old 3-arg `Run` closure. Fix them: add the 4th param `_ <-chan exec.PromptAnswer` to each Run closure signature, e.g.:
```go
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event), _ <-chan exec.PromptAnswer) (exec.Result, error) {
```
Re-run: `cd cores/duplex && GOWORK=off go test ./internal/bridge/ -v`
Expected: PASS (TestServeListThenRunThenExit, TestServeSurvivesFailedRun, TestServePrompt).

- [ ] **Step 6: Commit** — SKIP (не коммитим).

---

### Task 3: cmd/core wiring + e2e через мост

**Files:**
- Modify: `cores/duplex/cmd/core/main.go`
- Create: `cores/duplex/cmd/core/testdata/ask/script.manifest`
- Create: `cores/duplex/cmd/core/testdata/ask/main.py`
- Test: `cores/duplex/cmd/core/main_test.go`

**Interfaces:**
- Consumes: `runStreaming` (extended), `bridge.Deps`/`bridge.Serve` (Task 2), `exec.PromptAnswer`.
- Produces: `runStreaming(ctx, coreManifestPath, scriptDir, onEvent, answers)`; `run()` passes `nil` answers; `main()` Deps.Run threads answers.

- [ ] **Step 1: Создать testdata prompt-скрипт**

`cores/duplex/cmd/core/testdata/ask/script.manifest`:
```toml
name = "ask"
version = "0.1.0"
core = "duplex"
coreApi = 1
capabilities = []
link = "stdio"
language = "python"
cmd = ["python", "main.py"]
```
`cores/duplex/cmd/core/testdata/ask/main.py`:
```python
import sys, json
def send(o): sys.stdout.write(json.dumps(o) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())
recv()  # hello
send({"type": "ready"})
send({"type": "prompt", "id": "1", "message": "name?"})
resp = recv()
name = resp.get("result", {}).get("value", "?")
send({"type": "log", "level": "info", "message": "hello " + name})
send({"type": "done", "exitCode": 0, "result": {"name": name}})
```

- [ ] **Step 2: Написать падающий e2e-тест**

Add to `cores/duplex/cmd/core/main_test.go` (imports become `bytes`, `context`, `path/filepath`, `strings`, `testing`, and `wire-auto/cores/duplex/internal/bridge`, `wire-auto/cores/duplex/internal/exec`):
```go
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
```

- [ ] **Step 3: Запустить — убедиться, что падает**

Run: `cd cores/duplex && GOWORK=off go test ./cmd/core/ -run TestE2EPromptViaBridge -v`
Expected: FAIL to compile — `runStreaming` takes 4 args, not 5.

- [ ] **Step 4: main.go — расширить runStreaming, run(), Deps.Run**

4a. Change `runStreaming` signature and the `exec.Spec` it builds. Update the signature line:
```go
func runStreaming(ctx context.Context, coreManifestPath, scriptDir string, onEvent func(exec.Event), answers <-chan exec.PromptAnswer) (exec.Result, error) {
```
and in the `spec := exec.Spec{...}` literal, add `Answers: answers,` (e.g. right after `OnEvent: onEvent,`):
```go
		OnEvent:        onEvent,
		Answers:        answers,
	}
```

4b. Update the `run()` wrapper to pass `nil`:
```go
func run(coreManifestPath, scriptDir string) (exec.Result, error) {
	return runStreaming(context.Background(), coreManifestPath, scriptDir, nil, nil)
}
```

4c. Update the `Deps.Run` closure in `main()`:
```go
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event), answers <-chan exec.PromptAnswer) (exec.Result, error) {
			return runStreaming(ctx, *coreManifest, dir, onEvent, answers)
		},
```

- [ ] **Step 5: Запустить — убедиться, что проходит + весь модуль**

Run: `cd cores/duplex && GOWORK=off go test ./cmd/core/ -run TestE2EPromptViaBridge -v`
Expected: PASS.
Then full module: `cd cores/duplex && GOWORK=off go build ./... && go vet ./... && go test ./...`
Expected: все пакеты зелёные.

- [ ] **Step 6: Commit** — SKIP (не коммитим).

---

### Task 4: deview client — поля сообщений, SendInput, prompt в Run

**Files:**
- Modify: `apps/deview/internal/bridge/message.go`
- Modify: `apps/deview/internal/bridge/client.go`
- Test: `apps/deview/internal/bridge/client_test.go`

**Interfaces:**
- Consumes: existing `Client`/`Transport`/`Event`/`Command`.
- Produces: `Event.ID`, `Command.ID`/`Command.Value`; `Client.SendInput(id, value string) error`; `Client.Run` calls `onEvent` for `prompt` too.

- [ ] **Step 1: Написать падающие тесты**

Add to `apps/deview/internal/bridge/client_test.go` (add imports `bytes`, `strings`):
```go
func TestInputCommandEncodes(t *testing.T) {
	var buf bytes.Buffer
	if err := encodeCommand(&buf, Command{Type: "input", ID: "1", Value: "Bob"}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	want := `{"type":"input","id":"1","value":"Bob"}`
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestPromptEventDecodes(t *testing.T) {
	d := newEventDecoder(strings.NewReader(`{"type":"prompt","id":"1","message":"name?"}` + "\n"))
	e, err := d.next()
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != "prompt" || e.ID != "1" || e.Message != "name?" {
		t.Fatalf("prompt event=%+v", e)
	}
}

func TestClientRunHandlesPrompt(t *testing.T) {
	ft := &fakeTransport{events: []Event{
		{Type: "prompt", ID: "1", Message: "name?"},
		{Type: "result", Status: "OK"},
	}}
	c := NewClient(ft)
	term, err := c.Run("scripts/x", func(e Event) {
		if e.Type == "prompt" {
			_ = c.SendInput(e.ID, "Bob")
		}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if term.Type != "result" {
		t.Fatalf("терминал неверен: %+v", term)
	}
	var found bool
	for _, s := range ft.sent {
		if s.Type == "input" && s.ID == "1" && s.Value == "Bob" {
			found = true
		}
	}
	if !found {
		t.Fatalf("input-команда не отправлена: %+v", ft.sent)
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd apps/deview && GOWORK=off go test ./internal/bridge/ -run 'TestInput|TestPrompt|TestClientRunHandlesPrompt' -v`
Expected: FAIL to compile — `Command.ID/Value`, `Event.ID`, `Client.SendInput` undefined.

- [ ] **Step 3: message.go — поля (зеркало серверных)**

In `apps/deview/internal/bridge/message.go`:
```go
type Command struct {
	Type  string `json:"type"`
	Dir   string `json:"dir,omitempty"`
	ID    string `json:"id,omitempty"`
	Value string `json:"value,omitempty"`
}
```
and add `ID` to `Event` right after `Type`:
```go
type Event struct {
	Type         string          `json:"type"`
	ID           string          `json:"id,omitempty"`
	Scripts      []Script        `json:"scripts,omitempty"`
	Level        string          `json:"level,omitempty"`
	Message      string          `json:"message,omitempty"`
	Status       string          `json:"status,omitempty"`
	ExitCode     int             `json:"exitCode,omitempty"`
	ErrorCode    string          `json:"errorCode,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
}
```

- [ ] **Step 4: client.go — SendInput + prompt в Run**

In `apps/deview/internal/bridge/client.go`, add `"prompt"` to the streaming case in `Run`:
```go
		switch e.Type {
		case "ready", "log", "prompt":
			if onEvent != nil {
				onEvent(e)
			}
		case "result", "error":
			return e, nil
		}
```
and add the method (after `Cancel`):
```go
// SendInput отвечает на prompt скрипта строкой value (коррелируется по id).
func (c *Client) SendInput(id, value string) error {
	return c.t.Send(Command{Type: "input", ID: id, Value: value})
}
```

- [ ] **Step 5: Запустить — убедиться, что проходит + модуль deview**

Run: `cd apps/deview && GOWORK=off go test ./internal/bridge/ -v`
Expected: PASS (все, включая новые три).

- [ ] **Step 6: Commit** — SKIP (не коммитим).

---

### Task 5: deview REPL — показать prompt и прочитать строку

**Files:**
- Modify: `apps/deview/cmd/deview/main.go`

**Interfaces:**
- Consumes: `Client.SendInput` (Task 4), event `prompt`.
- Produces: интерактивная обработка prompt в `runOne` (печать запроса, чтение строки из stdin пользователя, `SendInput`).

Примечание: REPL интерактивен и не покрывается юнит-тестом; корректность релея уже доказана e2e (Task 3) и клиентским тестом (Task 4). Здесь — проводка UI + проверка сборки.

- [ ] **Step 1: Прокинуть stdin-reader в runOne и обработать prompt**

In `apps/deview/cmd/deview/main.go`:

1a. Pass the existing stdin scanner into `runOne`. In `main()`, change the call site:
```go
			runOne(client, reader, scripts[n-1])
```

1b. Update `runOne` signature and add the `prompt` case that reads a line and answers:
```go
// runOne запускает выбранный скрипт и рисует его ход. Ctrl-C во время прогона
// шлёт cancel мосту. На событие prompt печатает запрос и читает строку из stdin.
func runOne(client *bridge.Client, reader *bufio.Scanner, s bridge.Script) {
	fmt.Printf("\n▶ %s\n", s.Name)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigCh)
		close(sigCh)
	}()
	go func() {
		for range sigCh {
			fmt.Println("\n… отмена")
			_ = client.Cancel()
		}
	}()

	term, err := client.Run(s.Dir, func(e bridge.Event) {
		switch e.Type {
		case "ready":
			fmt.Println("  ⏳ выполняется…")
		case "log":
			fmt.Println(ui.RenderLog(e))
		case "prompt":
			fmt.Printf("  ❔ %s ", e.Message)
			line := ""
			if reader.Scan() {
				line = strings.TrimSpace(reader.Text())
			}
			_ = client.SendInput(e.ID, line)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "обрыв связи с мостом:", err)
		return
	}
	fmt.Println(ui.RenderResult(term))
	fmt.Println()
}
```
(`bufio` and `strings` are already imported in this file.)

- [ ] **Step 2: Собрать и провётить модуль deview**

Run: `cd apps/deview && GOWORK=off go build ./... && go vet ./... && go test ./...`
Expected: build/vet clean; все тесты зелёные.

- [ ] **Step 3: Commit** — SKIP (не коммитим).

---

### Task 6: демо-скрипт ask-name

**Files:**
- Create: `scripts/examples/ask-name/script.manifest`
- Create: `scripts/examples/ask-name/main.py`

**Interfaces:**
- Consumes: prompt-протокол (Task 1). Не объявляет capabilities.
- Produces: скрипт для discovery; `done.result = {name}`.

- [ ] **Step 1: Создать script.manifest**

`scripts/examples/ask-name/script.manifest`:
```toml
name = "ask-name"
version = "0.1.0"
core = "duplex"
coreApi = 1
capabilities = []
link = "stdio"
language = "python"
cmd = ["python", "main.py"]
```

- [ ] **Step 2: Создать main.py**

`scripts/examples/ask-name/main.py`:
```python
import sys, json

def send(o): sys.stdout.write(json.dumps(o, separators=(",", ":")) + "\n"); sys.stdout.flush()
def recv(): return json.loads(sys.stdin.readline())

_id = 0
def prompt(message):
    global _id
    _id += 1
    send({"type": "prompt", "id": str(_id), "message": message})
    resp = recv()
    return resp.get("result", {}).get("value", "")

recv()  # hello — говорим на протоколе напрямую, без SDK
send({"type": "ready"})

name = prompt("Как тебя зовут?")
send({"type": "log", "level": "info", "message": "Привет, %s!" % (name or "аноним")})
send({"type": "done", "exitCode": 0, "result": {"name": name}})
```

- [ ] **Step 3: Проверить синтаксис Python**

Run: `python -c "import ast; ast.parse(open(r'D:\Projects\wire-auto\scripts\examples\ask-name\main.py').read()); print('syntax ok')"`
Expected: `syntax ok` (если `python` не найден — `py -c ...`).

- [ ] **Step 4: Commit** — SKIP (не коммитим).

---

### Task 7: документация

**Files:**
- Create: `guide/demo/prompts.md`
- Modify: `guide/demo/README.md`
- Modify: `guide/README.md`
- Modify: `guide/demo/protocol.md`
- Modify: `guide/demo/app-core-bridge.md`

**Interfaces:**
- Consumes: контракт prompt/input/response (Task 1–4).
- Produces: топик-файл + ссылки из индексов.

- [ ] **Step 1: Создать guide/demo/prompts.md**

`guide/demo/prompts.md`:
```markdown
# Интерактивный ввод (prompt)

Дуплекс двунаправлен, поэтому скрипт может **по ходу выполнения** попросить у
человека строку ввода. `prompt` — не capability: ввод идёт от человека, а не из
`provides` ядра, поэтому его не надо объявлять в `script.capabilities` и он не
проходит гейт рукопожатия.

## Поток

1. скрипт→ядро: `{"type":"prompt","id":"1","message":"Введите хост"}`
2. ядро→клиент (событие моста): `{"type":"prompt","id":"1","message":"Введите хост"}`
3. deview печатает запрос и читает строку у пользователя
4. клиент→ядро (команда моста): `{"type":"input","id":"1","value":"example.com"}`
5. ядро→скрипт: `{"type":"response","id":"1","result":{"value":"example.com"}}`

За раз висит максимум один prompt: скрипт синхронный — отправил и ждёт `response`
по тому же `id`. Код скрипта симметричен вызову capability (request→response).

## Политика

`prompt` блокирует прогон до ответа клиента либо до отмены/`RunTimeout`
(существующие механизмы; отдельного таймаута на prompt нет). Если отвечающего
клиента нет (например, ядро гоняют пайпом без ввода), prompt висит до `RunTimeout`.

## Пример

См. `scripts/examples/ask-name/` — спрашивает имя через `prompt` и печатает
приветствие. Значение возвращается в `done.result.name`.
```

- [ ] **Step 2: Ссылка в guide/demo/README.md**

Read `guide/demo/README.md` to match the table format, then add after the `capabilities.md` row:
```markdown
| [prompts.md](prompts.md) | Интерактивный ввод: событие prompt / команда input, поток через обе границы, политика блокировки, пример ask-name |
```

- [ ] **Step 3: Ссылка в guide/README.md (если там есть индекс топиков demo)**

Read `guide/README.md`. If it links to `guide/demo/` topics, add an analogous `prompts.md` row/link matching the existing format. If it only points to the demo folder/README, leave it unchanged and note that in the report.

- [ ] **Step 4: Заметка в guide/demo/protocol.md**

Read `guide/demo/protocol.md`; in the message-set section add a short line:
```markdown
- `prompt` (скрипт→ядро) — запрос строки ввода у человека; ответ приходит как
  `response` с тем же `id` и `result.value`. Подробнее — [prompts.md](prompts.md).
```

- [ ] **Step 5: Заметка в guide/demo/app-core-bridge.md**

Read `guide/demo/app-core-bridge.md`; note the new event/command in the app↔core listing:
```markdown
- Событие `prompt` (вверх) и команда `input` (вниз) — интерактивный ввод от
  пользователя во время прогона; см. [prompts.md](prompts.md).
```

- [ ] **Step 6: Commit** — SKIP (не коммитим).

---

## Итоговая проверка

- [ ] `cd cores/duplex && GOWORK=off go build ./... && go vet ./... && go test ./...` — зелёное.
- [ ] `cd apps/deview && GOWORK=off go build ./... && go vet ./... && go test ./...` — зелёное.
- [ ] e2e `TestE2EPromptViaBridge` проходит (prompt→input→response через мост).
- [ ] Демо `ask-name` находится discovery; при желании — ручная проверка через `go run ./apps/deview/cmd/deview` (ввести имя → приветствие → OK).
- [ ] `guide/demo/prompts.md` создан и слинкован из индекса.
- [ ] `capreg`, `handshake`, `core.manifest` не изменены.

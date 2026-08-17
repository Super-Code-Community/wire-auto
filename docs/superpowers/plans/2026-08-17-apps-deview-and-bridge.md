# apps/deview + двусторонний мост — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Превратить `runtime/basic` в вечно живущий двусторонний мост (JSON Lines по stdio) и построить консольный клиент `apps/deview` — минимальный браузер скриптов с живым отображением хода выполнения.

**Architecture:** `wire` всегда стартует как мост: читает команды (`list`/`run`/`cancel`/`exit`) из stdin, стримит события (`catalog`/`ready`/`log`/`result`/`error`) в stdout. Движок одного прогона (`exec.Run`) учится отдавать события по ходу и принимать внешнюю отмену через `context`. `deview` поднимает `wire` подпроцессом, говорит на протоколе моста и рисует меню + живой лог.

**Tech Stack:** Go 1.26, стандартная библиотека + `github.com/BurntSushi/toml` (уже в `runtime/basic`). Тесты — стандартный `testing`.

## Global Constraints

- Go 1.26.4; каждый модуль независимо собираем; `go.work` связывает их для dev.
- Принцип «ничего монолитного»: один файл — одна ответственность; маленькие пакеты.
- Git-дисциплина проекта (`CLAUDE.md`): **не создавать ветки, не коммитить/пушить без явной просьбы пользователя**. Шаги «Commit» ниже выполняются ТОЛЬКО когда пользователь явно это разрешил; иначе — пропустить коммит и продолжить.
- Протокол runtime↔script (`internal/protocol`) НЕ трогаем — меняем только верхнюю границу app↔runtime.
- stdout моста — исключительно канал JSON Lines; диагностика — в stderr.
- Doc-дисциплина (`guide/`): много маленьких файлов, README — только индекс.

---

## Обзор файлов

**runtime/basic (правки):**
- `internal/exec/exec.go` — `+context`, `+OnEvent`, `+Result.Result`, `+CANCELLED`.
- `internal/exec/event.go` (нов.) — тип `Event`.
- `internal/discovery/discovery.go` (нов.) — скан `scripts/`.
- `internal/bridge/message.go` (нов.) — типы `Command`/`Event`/`Script` + кодек JSON Lines.
- `internal/bridge/serve.go` (нов.) — вечный цикл моста.
- `cmd/wire/main.go` — всегда мост; `runStreaming()` + тонкий `run()`.

**apps/deview (новый модуль):**
- `go.mod`
- `internal/bridge/message.go` — зеркало типов протокола моста + кодек.
- `internal/bridge/transport.go` — интерфейс `Transport` + `ProcessTransport`.
- `internal/bridge/client.go` — `Client` (List/Run/Cancel/Close).
- `internal/ui/menu.go` — рендер меню, чтение выбора.
- `internal/ui/render.go` — рендер событий прогона.
- `cmd/deview/main.go` — REPL-цикл.

**Прочее:** `go.work` (+`use ./apps/deview`), doc-топики в `guide/demo/`.

---

## Task 1: `exec` — события, отмена, нагрузка результата

**Files:**
- Create: `runtime/basic/internal/exec/event.go`
- Modify: `runtime/basic/internal/exec/exec.go`
- Test: `runtime/basic/internal/exec/exec_test.go` (дополнить)

**Interfaces:**
- Produces:
  - `type Event struct { Kind, Level, Message string }` (Kind: `"ready"` | `"log"`)
  - `Spec.OnEvent func(Event)` — необязательный сток (nil = молчать)
  - `Result.Result json.RawMessage` — нагрузка из `done.result`
  - `StatusCancelled = "CANCELLED"`
  - `func Run(ctx context.Context, spec Spec) Result` — новая сигнатура

- [ ] **Step 1: Написать тип события**

Create `runtime/basic/internal/exec/event.go`:

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

- [ ] **Step 2: Написать падающий тест на стриминг и нагрузку**

Дополнить `runtime/basic/internal/exec/exec_test.go`. Изучи существующий файл на предмет уже используемого фейкового скрипта/хелпера; если запуск процессов там уже есть — переиспользуй его стиль. Добавь тест, который гоняет `hello`-скрипт через настоящий `exec.Run` не будем (нужен python); вместо этого проверим сигнатуру и сток на уровне уже имеющихся интеграционных хелперов. Если в файле нет process-хелпера, добавь этот модульный тест на прокидывание `ctx` и совместимость:

```go
func TestRunAcceptsContextAndSink(t *testing.T) {
	// Компиляционный контракт: Run принимает ctx, Spec имеет OnEvent,
	// Result имеет поле Result. Тест ловит регресс сигнатур.
	var got []Event
	spec := Spec{
		Command:        "nonexistent-binary-xyz",
		StartupTimeout: 10 * time.Millisecond,
		OnEvent:        func(e Event) { got = append(got, e) },
	}
	res := Run(context.Background(), spec)
	if res.Status != StatusCrashed {
		t.Fatalf("ожидали CRASHED для отсутствующего бинарника, got %s", res.Status)
	}
	_ = got // сток может быть пустым: процесс не стартовал
}
```

- [ ] **Step 3: Запустить — убедиться, что не компилируется/падает**

Run: `cd runtime/basic && go test ./internal/exec/ -run TestRunAcceptsContextAndSink`
Expected: FAIL — компиляция: `Run` не принимает ctx, у `Spec` нет `OnEvent`.

- [ ] **Step 4: Обновить `exec.go` — сигнатура, импорт, поля**

В `runtime/basic/internal/exec/exec.go`:

Добавь в import `"context"` и `"encoding/json"`.

Добавь константу к блоку статусов:

```go
	StatusCancelled         = "CANCELLED"
```

Добавь поле в `Spec` (рядом с остальными полями):

```go
	// OnEvent, если задан, получает ready/log по ходу прогона (для стриминга наружу).
	OnEvent func(Event)
```

Добавь поле в `Result`:

```go
	Result json.RawMessage
```

Смени сигнатуру: `func Run(spec Spec) Result` → `func Run(ctx context.Context, spec Spec) Result`.

- [ ] **Step 5: Обновить `exec.go` — эмиссия событий и обработка ctx/result**

Внутри `Run`, помощник эмиссии в начале функции:

```go
	emit := func(e Event) {
		if spec.OnEvent != nil {
			spec.OnEvent(e)
		}
	}
```

В ветке `case protocol.TypeReady:` добавь эмиссию после `gotReady = true` и переключения дедлайна:

```go
			emit(Event{Kind: "ready"})
```

В ветке `case protocol.TypeLog:` добавь эмиссию после аппенда в `res.Logs`:

```go
			emit(Event{Kind: "log", Level: ev.msg.Level, Message: ev.msg.Message})
```

В ветке `case protocol.TypeDone:` протащи нагрузку — после вычисления `code`:

```go
			res.Result = ev.msg.Result
```

Добавь обработку внешней отмены. В `select` внутри `loop` добавь новый case (рядом с `case <-deadline:`):

```go
		case <-ctx.Done():
			// Внешняя отмена клиента: тот же путь cancel→grace→kill, статус CANCELLED.
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
```

- [ ] **Step 6: Запустить тест exec**

Run: `cd runtime/basic && go test ./internal/exec/ -run TestRunAcceptsContextAndSink -v`
Expected: PASS.

- [ ] **Step 7: Убедиться, что весь пакет компилируется (вызовы Run поломаны — это ок, чиним в Task 4)**

Run: `cd runtime/basic && go build ./internal/exec/`
Expected: PASS (пакет `exec` самодостаточен; вызовы из `cmd/wire` починим в Task 4).

- [ ] **Step 8: Commit** (только если пользователь явно разрешил коммитить)

```bash
git add runtime/basic/internal/exec/
git commit -m "feat(exec): stream events, external cancel, result payload"
```

---

## Task 2: `discovery` — скан каталога скриптов

**Files:**
- Create: `runtime/basic/internal/discovery/discovery.go`
- Test: `runtime/basic/internal/discovery/discovery_test.go`

**Interfaces:**
- Consumes: `manifest.LoadScript(path string) (manifest.Script, error)`
- Produces:
  - `type Script struct { Name, Dir, Language, Version string; Capabilities []string }`
  - `func Scan(scriptsDir string) ([]Script, error)` — сортированный по Name список валидных скриптов; папки без валидного `script.manifest` пропускаются (не ошибка)

- [ ] **Step 1: Написать падающий тест**

Create `runtime/basic/internal/discovery/discovery_test.go`:

```go
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
	writeManifest(t, filepath.Join(root, "beta"), `name = "beta"
version = "0.2.0"
core = "regular"
coreApi = 1
link = "stdio"
cmd = ["python", "main.py"]
language = "python"
capabilities = []
`)
	writeManifest(t, filepath.Join(root, "alpha"), `name = "alpha"
core = "regular"
coreApi = 1
link = "stdio"
cmd = ["bash", "run.sh"]
language = "bash"
capabilities = ["serial"]
`)
	// Папка без манифеста — должна тихо игнорироваться.
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
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd runtime/basic && go test ./internal/discovery/`
Expected: FAIL — пакета/функции `Scan` нет.

- [ ] **Step 3: Реализовать `discovery.go`**

Create `runtime/basic/internal/discovery/discovery.go`:

```go
// Package discovery сканирует корень скриптов на script.manifest и отдаёт
// сводки для каталога моста. Симметрично registry.Discover для ядер.
package discovery

import (
	"os"
	"path/filepath"
	"sort"

	"wire-auto/runtime/basic/internal/manifest"
)

// Script — сводка одного скрипта для каталога клиента.
type Script struct {
	Name         string
	Dir          string
	Language     string
	Version      string
	Capabilities []string
}

// Scan читает <scriptsDir>/*/script.manifest. Папки без валидного манифеста
// тихо пропускаются (не ошибка); ошибкой является лишь нечитаемый корень.
// Результат отсортирован по Name.
func Scan(scriptsDir string) ([]Script, error) {
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		return nil, err
	}
	var out []Script
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(scriptsDir, e.Name())
		mpath := filepath.Join(dir, "script.manifest")
		if _, err := os.Stat(mpath); err != nil {
			continue // не папка скрипта
		}
		s, err := manifest.LoadScript(mpath)
		if err != nil {
			continue // невалидный манифест — пропускаем
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			abs = dir
		}
		caps := s.Capabilities
		if caps == nil {
			caps = []string{}
		}
		out = append(out, Script{
			Name:         s.Name,
			Dir:          abs,
			Language:     s.Language,
			Version:      s.Version,
			Capabilities: caps,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
```

- [ ] **Step 4: Запустить тесты**

Run: `cd runtime/basic && go test ./internal/discovery/ -v`
Expected: PASS (оба теста).

- [ ] **Step 5: Commit** (только с явного разрешения)

```bash
git add runtime/basic/internal/discovery/
git commit -m "feat(discovery): scan scripts dir into catalog summaries"
```

---

## Task 3: `bridge` (сервер) — протокол и вечный цикл

**Files:**
- Create: `runtime/basic/internal/bridge/message.go`
- Create: `runtime/basic/internal/bridge/serve.go`
- Test: `runtime/basic/internal/bridge/serve_test.go`

**Interfaces:**
- Consumes: `exec.Event`, `exec.Result`, `discovery.Script`
- Produces:
  - `type Command struct { Type, Dir string }`
  - `type Script` (те же поля, что `discovery.Script`, с json-тегами)
  - `type Event struct { Type string; Scripts []Script; Level, Message, Status string; ExitCode int; ErrorCode, ErrorMessage string; Result json.RawMessage }`
  - `func NewCommandDecoder(r io.Reader) *CommandDecoder`; `(*CommandDecoder).Next() (Command, error)`
  - `func EncodeEvent(w io.Writer, e Event) error`
  - `type Deps struct { List func() ([]Script, error); Run func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) }`
  - `func Serve(in io.Reader, out io.Writer, deps Deps) error`

- [ ] **Step 1: Написать типы и кодек (`message.go`)**

Create `runtime/basic/internal/bridge/message.go`:

```go
// Package bridge реализует верхнюю границу app↔runtime: JSON Lines поверх stdio.
// Команды приложения вниз (list/run/cancel/exit), события рантайма вверх
// (catalog/ready/log/result/error).
package bridge

import (
	"bufio"
	"encoding/json"
	"io"
)

const maxLineBytes = 1 << 20 // 1 MiB, как в protocol

// Command — сообщение приложение→рантайм.
type Command struct {
	Type string `json:"type"`
	Dir  string `json:"dir,omitempty"`
}

// Script — элемент каталога.
type Script struct {
	Name         string   `json:"name"`
	Dir          string   `json:"dir"`
	Language     string   `json:"language,omitempty"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities"`
}

// Event — сообщение рантайм→приложение. Одна структура на все типы; лишние
// поля опускаются через omitempty.
type Event struct {
	Type         string          `json:"type"`
	Scripts      []Script        `json:"scripts,omitempty"`
	Level        string          `json:"level,omitempty"`
	Message      string          `json:"message,omitempty"`
	Status       string          `json:"status,omitempty"`
	ExitCode     int             `json:"exitCode,omitempty"`
	ErrorCode    string          `json:"errorCode,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
}

// EncodeEvent пишет событие одной JSON-строкой с '\n'.
func EncodeEvent(w io.Writer, e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return writeLine(w, data)
}

func writeLine(w io.Writer, data []byte) error {
	data = append(data, '\n')
	_, err := w.Write(data)
	return err
}

// CommandDecoder читает команды из потока построчно.
type CommandDecoder struct{ sc *bufio.Scanner }

func NewCommandDecoder(r io.Reader) *CommandDecoder {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	return &CommandDecoder{sc: sc}
}

// Next читает следующую команду; io.EOF по концу потока.
func (d *CommandDecoder) Next() (Command, error) {
	for d.sc.Scan() {
		line := d.sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var c Command
		if err := json.Unmarshal(line, &c); err != nil {
			return Command{}, err
		}
		return c, nil
	}
	if err := d.sc.Err(); err != nil {
		return Command{}, err
	}
	return Command{}, io.EOF
}
```

- [ ] **Step 2: Написать падающий тест на цикл моста**

Create `runtime/basic/internal/bridge/serve_test.go`:

```go
package bridge

import (
	"context"
	"strings"
	"sync"
	"testing"

	"wire-auto/runtime/basic/internal/exec"
)

// collectEvents декодирует NDJSON-вывод моста обратно в события.
func collectEvents(t *testing.T, out string) []Event {
	t.Helper()
	var evs []Event
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var e Event
		if err := jsonUnmarshalLine(line, &e); err != nil {
			t.Fatalf("bad event line %q: %v", line, err)
		}
		evs = append(evs, e)
	}
	return evs
}

func TestServeListThenRunThenExit(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"type":"list"}`,
		`{"type":"run","dir":"scripts/x"}`,
		`{"type":"exit"}`,
	}, "\n") + "\n")
	var out syncBuffer

	deps := Deps{
		List: func() ([]Script, error) {
			return []Script{{Name: "x", Dir: "scripts/x", Capabilities: []string{}}}, nil
		},
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) {
			onEvent(exec.Event{Kind: "ready"})
			onEvent(exec.Event{Kind: "log", Level: "info", Message: "hi"})
			return exec.Result{Status: exec.StatusOK, ExitCode: 0}, nil
		},
	}

	if err := Serve(in, &out, deps); err != nil {
		t.Fatalf("Serve error: %v", err)
	}

	evs := collectEvents(t, out.String())
	types := make([]string, len(evs))
	for i, e := range evs {
		types[i] = e.Type
	}
	want := []string{"catalog", "ready", "log", "result"}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Fatalf("порядок событий = %v, ожидали %v", types, want)
	}
	if evs[0].Scripts[0].Name != "x" {
		t.Fatalf("каталог без скрипта x: %+v", evs[0])
	}
	if evs[3].Status != exec.StatusOK {
		t.Fatalf("result status = %s", evs[3].Status)
	}
}

func TestServeSurvivesFailedRun(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"type":"run","dir":"bad"}`,
		`{"type":"list"}`,
		`{"type":"exit"}`,
	}, "\n") + "\n")
	var out syncBuffer

	deps := Deps{
		List: func() ([]Script, error) { return []Script{}, nil },
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) {
			panic("boom")
		},
	}

	if err := Serve(in, &out, deps); err != nil {
		t.Fatalf("Serve error: %v", err)
	}
	evs := collectEvents(t, out.String())
	// Первое событие — error от упавшего прогона; затем мост жив и отдаёт catalog.
	if len(evs) < 2 || evs[0].Type != "error" {
		t.Fatalf("ожидали error затем catalog, got %+v", evs)
	}
	var sawCatalog bool
	for _, e := range evs {
		if e.Type == "catalog" {
			sawCatalog = true
		}
	}
	if !sawCatalog {
		t.Fatal("мост не пережил упавший прогон (нет catalog после)")
	}
}

// --- маленькие тест-хелперы ---

type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
```

Добавь также крошечный хелпер декодирования в отдельном файле теста, чтобы не тянуть json в каждый тест напрямую. Create `runtime/basic/internal/bridge/helpers_test.go`:

```go
package bridge

import "encoding/json"

func jsonUnmarshalLine(line string, v any) error {
	return json.Unmarshal([]byte(line), v)
}
```

- [ ] **Step 3: Запустить — убедиться, что падает**

Run: `cd runtime/basic && go test ./internal/bridge/`
Expected: FAIL — нет `Serve`/`Deps`.

- [ ] **Step 4: Реализовать вечный цикл (`serve.go`)**

Create `runtime/basic/internal/bridge/serve.go`:

```go
package bridge

import (
	"context"
	"io"
	"sync"

	"wire-auto/runtime/basic/internal/exec"
)

// Deps — инъекция зависимостей моста (для тестируемости).
type Deps struct {
	List func() ([]Script, error)
	Run  func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error)
}

// runDone — сигнал о завершении одного прогона.
type runDone struct{}

// Serve крутит вечный цикл моста, пока не придёт exit или EOF stdin.
// Прогоны последовательны (один за раз), но чтение команд не блокируется на
// прогоне — поэтому cancel доходит до бегущего скрипта. Паника внутри прогона
// изолируется: наружу уходит error, мост продолжает жить.
func Serve(in io.Reader, out io.Writer, deps Deps) error {
	var mu sync.Mutex
	write := func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		_ = EncodeEvent(out, e)
	}

	cmds := make(chan Command)
	go func() {
		dec := NewCommandDecoder(in)
		for {
			c, err := dec.Next()
			if err != nil {
				close(cmds)
				return
			}
			cmds <- c
		}
	}()

	done := make(chan runDone, 1)
	var cancelCur context.CancelFunc
	running := false

	finish := func() {
		if cancelCur != nil {
			cancelCur()
			cancelCur = nil
		}
	}

	for {
		select {
		case <-done:
			running = false
			cancelCur = nil

		case c, ok := <-cmds:
			if !ok {
				finish()
				return nil
			}
			switch c.Type {
			case "list":
				scripts, err := deps.List()
				if err != nil {
					write(Event{Type: "error", Message: err.Error()})
					continue
				}
				if scripts == nil {
					scripts = []Script{}
				}
				write(Event{Type: "catalog", Scripts: scripts})

			case "run":
				if running {
					write(Event{Type: "error", Message: "busy: a script is already running"})
					continue
				}
				running = true
				ctx, cancel := context.WithCancel(context.Background())
				cancelCur = cancel
				go func(dir string) {
					defer func() {
						if r := recover(); r != nil {
							write(Event{Type: "error", Message: "run panicked"})
						}
						done <- runDone{}
					}()
					res, err := deps.Run(ctx, dir, func(ev exec.Event) {
						switch ev.Kind {
						case "ready":
							write(Event{Type: "ready"})
						case "log":
							write(Event{Type: "log", Level: ev.Level, Message: ev.Message})
						}
					})
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

			case "cancel":
				if cancelCur != nil {
					cancelCur()
				}

			case "exit":
				finish()
				return nil
			}
		}
	}
}
```

- [ ] **Step 5: Запустить тесты моста**

Run: `cd runtime/basic && go test ./internal/bridge/ -v`
Expected: PASS (`TestServeListThenRunThenExit`, `TestServeSurvivesFailedRun`).

Примечание: тесты не проверяют гонки при cancel напрямую (это покрывает интеграция в Task 4/7). Для отлова гонок прогони с флагом:
Run: `cd runtime/basic && go test -race ./internal/bridge/`
Expected: PASS без предупреждений `-race`.

- [ ] **Step 6: Commit** (только с явного разрешения)

```bash
git add runtime/basic/internal/bridge/
git commit -m "feat(bridge): app<->runtime JSON Lines protocol and eternal serve loop"
```

---

## Task 4: `cmd/wire` — всегда мост

**Files:**
- Modify: `runtime/basic/cmd/wire/main.go`
- Test: `runtime/basic/cmd/wire/main_test.go` (не менять существующие тесты; добавить один)

**Interfaces:**
- Consumes: `bridge.Serve`, `bridge.Deps`, `bridge.Script`, `discovery.Scan`, `exec.Run`, все из Task 1–3
- Produces:
  - `func runStreaming(ctx context.Context, runtimePath, coresDir, scriptDir string, onEvent func(exec.Event)) (exec.Result, error)`
  - `func run(runtimePath, coresDir, scriptDir string) (exec.Result, error)` — тонкая обёртка (сигнатура сохранена для существующих тестов)

- [ ] **Step 1: Проверить, что существующие e2e-тесты пока падают компиляцией**

Run: `cd runtime/basic && go test ./cmd/wire/`
Expected: FAIL — `exec.Run` теперь требует ctx (после Task 1), `main.go` ещё старый.

- [ ] **Step 2: Переписать `main.go`**

Заменить содержимое `runtime/basic/cmd/wire/main.go` на:

```go
// Command wire — двусторонний мост: приложение шлёт команды в stdin, рантайм
// стримит события в stdout (JSON Lines). Живёт до exit/EOF, обслуживая запуск
// за запуском.
package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"time"

	"wire-auto/runtime/basic/internal/bridge"
	"wire-auto/runtime/basic/internal/discovery"
	"wire-auto/runtime/basic/internal/exec"
	"wire-auto/runtime/basic/internal/handshake"
	"wire-auto/runtime/basic/internal/manifest"
	"wire-auto/runtime/basic/internal/registry"
)

// runStreaming выполняет один прогон: discover→route→handshake→spawn→pump,
// прокидывая ctx (внешняя отмена) и onEvent (живой поток) в exec.Run.
func runStreaming(ctx context.Context, runtimePath, coresDir, scriptDir string, onEvent func(exec.Event)) (exec.Result, error) {
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
		OnEvent:        onEvent,
	}
	return exec.Run(ctx, spec), nil
}

// run — тонкая обёртка без стриминга (используется e2e-тестами).
func run(runtimePath, coresDir, scriptDir string) (exec.Result, error) {
	return runStreaming(context.Background(), runtimePath, coresDir, scriptDir, nil)
}

func main() {
	runtimePath := flag.String("runtime", "runtime/basic/runtime.manifest", "path to runtime manifest")
	coresDir := flag.String("cores", "cores", "path to the cores directory")
	scriptsDir := flag.String("scripts", "scripts", "path to the scripts directory")
	flag.Parse()

	deps := bridge.Deps{
		List: func() ([]bridge.Script, error) {
			found, err := discovery.Scan(*scriptsDir)
			if err != nil {
				return nil, err
			}
			out := make([]bridge.Script, len(found))
			for i, s := range found {
				out[i] = bridge.Script{
					Name:         s.Name,
					Dir:          s.Dir,
					Language:     s.Language,
					Version:      s.Version,
					Capabilities: s.Capabilities,
				}
			}
			return out, nil
		},
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) {
			return runStreaming(ctx, *runtimePath, *coresDir, dir, onEvent)
		},
	}

	if err := bridge.Serve(os.Stdin, os.Stdout, deps); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Запустить существующие e2e-тесты — должны снова зеленеть**

Run: `cd runtime/basic && go test ./cmd/wire/ -v`
Expected: PASS — `TestEndToEndHello`, `TestEndToEndCapabilityDenied`, `TestEndToEndUnknownCore` (они зовут `run()`, чья сигнатура сохранена).

- [ ] **Step 4: Добавить интеграционный тест моста через реальный `wire` над hello**

Дополнить `runtime/basic/cmd/wire/main_test.go`:

```go
func TestBridgeRunsHelloEndToEnd(t *testing.T) {
	root := repoRoot(t)
	var got exec.Result
	deps := bridge.Deps{
		List: func() ([]bridge.Script, error) { return nil, nil },
		Run: func(ctx context.Context, dir string, onEvent func(exec.Event)) (exec.Result, error) {
			return runStreaming(ctx,
				filepath.Join(root, "runtime", "basic", "runtime.manifest"),
				filepath.Join(root, "cores"),
				dir, onEvent)
		},
	}
	in := strings.NewReader(
		`{"type":"run","dir":"` + filepath.ToSlash(filepath.Join(root, "scripts", "examples", "hello")) + `"}` + "\n" +
			`{"type":"exit"}` + "\n")
	var out strings.Builder
	if err := bridge.Serve(in, &out, deps); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	_ = got
	if !strings.Contains(out.String(), `"type":"ready"`) ||
		!strings.Contains(out.String(), "hello from python") ||
		!strings.Contains(out.String(), `"status":"OK"`) {
		t.Fatalf("мост не отдал ожидаемые события:\n%s", out.String())
	}
}
```

Добавь нужные импорты в `main_test.go`: `"context"`, `"strings"`, `"wire-auto/runtime/basic/internal/bridge"`, `"wire-auto/runtime/basic/internal/exec"`.

- [ ] **Step 5: Запустить новый интеграционный тест**

Run: `cd runtime/basic && go test ./cmd/wire/ -run TestBridgeRunsHelloEndToEnd -v`
Expected: PASS (требует `python` в PATH — как и остальные e2e).

- [ ] **Step 6: Полная проверка модуля рантайма**

Run: `cd runtime/basic && go build ./... && go vet ./... && go test ./...`
Expected: всё PASS.

- [ ] **Step 7: Commit** (только с явного разрешения)

```bash
git add runtime/basic/cmd/wire/
git commit -m "feat(wire): always run as eternal bidirectional bridge"
```

---

## Task 5: `apps/deview` — модуль, протокол моста, транспорт, клиент

**Files:**
- Create: `apps/deview/go.mod`
- Create: `apps/deview/internal/bridge/message.go`
- Create: `apps/deview/internal/bridge/transport.go`
- Create: `apps/deview/internal/bridge/client.go`
- Test: `apps/deview/internal/bridge/client_test.go`
- Modify: `go.work`

**Interfaces:**
- Produces:
  - `type Command`, `type Event`, `type Script` (зеркало серверных, с json-тегами)
  - `type Transport interface { Send(Command) error; Recv() (Event, error); Close() error }`
  - `func NewProcessTransport(name string, args ...string) (*ProcessTransport, error)`
  - `type Client struct{...}`; `func NewClient(t Transport) *Client`
  - `(*Client).List() ([]Script, error)`
  - `(*Client).Run(dir string, onEvent func(Event)) (Event, error)` — возвращает терминальное событие (result или error)
  - `(*Client).Cancel() error`
  - `(*Client).Close() error`

- [ ] **Step 1: Создать модуль и включить его в go.work**

Create `apps/deview/go.mod`:

```
module wire-auto/apps/deview

go 1.26.4
```

Modify `go.work` — добавить строку после `use ./runtime/basic`:

```
use ./apps/deview
```

- [ ] **Step 2: Зеркальные типы + кодек (`message.go`)**

Create `apps/deview/internal/bridge/message.go`:

```go
// Package bridge — клиентская сторона протокола app↔runtime: кодирует команды,
// декодирует события (JSON Lines). Зеркало серверных типов в runtime/basic.
package bridge

import (
	"bufio"
	"encoding/json"
	"io"
)

const maxLineBytes = 1 << 20

type Command struct {
	Type string `json:"type"`
	Dir  string `json:"dir,omitempty"`
}

type Script struct {
	Name         string   `json:"name"`
	Dir          string   `json:"dir"`
	Language     string   `json:"language,omitempty"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities"`
}

type Event struct {
	Type         string          `json:"type"`
	Scripts      []Script        `json:"scripts,omitempty"`
	Level        string          `json:"level,omitempty"`
	Message      string          `json:"message,omitempty"`
	Status       string          `json:"status,omitempty"`
	ExitCode     int             `json:"exitCode,omitempty"`
	ErrorCode    string          `json:"errorCode,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
}

func encodeCommand(w io.Writer, c Command) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

type eventDecoder struct{ sc *bufio.Scanner }

func newEventDecoder(r io.Reader) *eventDecoder {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	return &eventDecoder{sc: sc}
}

func (d *eventDecoder) next() (Event, error) {
	for d.sc.Scan() {
		line := d.sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			return Event{}, err
		}
		return e, nil
	}
	if err := d.sc.Err(); err != nil {
		return Event{}, err
	}
	return Event{}, io.EOF
}
```

- [ ] **Step 3: Транспорт (`transport.go`)**

Create `apps/deview/internal/bridge/transport.go`:

```go
package bridge

import (
	"io"
	"os/exec"
)

// Transport — двусторонний канал к мосту. Абстракция ради тестируемости:
// прод — подпроцесс wire, тест — in-memory фейк.
type Transport interface {
	Send(Command) error
	Recv() (Event, error) // io.EOF по концу
	Close() error
}

// ProcessTransport поднимает `wire` подпроцессом и говорит по его stdin/stdout.
type ProcessTransport struct {
	cmd  *exec.Cmd
	in   io.WriteCloser
	dec  *eventDecoder
}

// NewProcessTransport запускает name с args (напр. "wire" или "go run ...").
// stderr процесса пробрасывается в текущий stderr для диагностики.
func NewProcessTransport(name string, args ...string) (*ProcessTransport, error) {
	c := exec.Command(name, args...)
	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	c.Stderr = io.Discard // сырую диагностику моста можно направить в os.Stderr при отладке
	if err := c.Start(); err != nil {
		return nil, err
	}
	return &ProcessTransport{cmd: c, in: stdin, dec: newEventDecoder(stdout)}, nil
}

func (p *ProcessTransport) Send(c Command) error { return encodeCommand(p.in, c) }
func (p *ProcessTransport) Recv() (Event, error) { return p.dec.next() }

func (p *ProcessTransport) Close() error {
	_ = encodeCommand(p.in, Command{Type: "exit"})
	_ = p.in.Close()
	return p.cmd.Wait()
}
```

- [ ] **Step 4: Написать падающий тест клиента (на фейковом транспорте)**

Create `apps/deview/internal/bridge/client_test.go`:

```go
package bridge

import (
	"io"
	"testing"
)

// fakeTransport — программируемый поток событий; фиксирует отправленные команды.
type fakeTransport struct {
	events []Event
	i      int
	sent   []Command
}

func (f *fakeTransport) Send(c Command) error { f.sent = append(f.sent, c); return nil }
func (f *fakeTransport) Recv() (Event, error) {
	if f.i >= len(f.events) {
		return Event{}, io.EOF
	}
	e := f.events[f.i]
	f.i++
	return e, nil
}
func (f *fakeTransport) Close() error { return nil }

func TestClientListReturnsCatalog(t *testing.T) {
	ft := &fakeTransport{events: []Event{
		{Type: "catalog", Scripts: []Script{{Name: "hello", Dir: "scripts/examples/hello"}}},
	}}
	c := NewClient(ft)
	scripts, err := c.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(scripts) != 1 || scripts[0].Name != "hello" {
		t.Fatalf("catalog неверен: %+v", scripts)
	}
	if len(ft.sent) != 1 || ft.sent[0].Type != "list" {
		t.Fatalf("ожидали отправку list, got %+v", ft.sent)
	}
}

func TestClientRunStreamsThenTerminal(t *testing.T) {
	ft := &fakeTransport{events: []Event{
		{Type: "ready"},
		{Type: "log", Level: "info", Message: "hi"},
		{Type: "result", Status: "OK", ExitCode: 0},
	}}
	c := NewClient(ft)
	var streamed []Event
	term, err := c.Run("scripts/x", func(e Event) { streamed = append(streamed, e) })
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if term.Type != "result" || term.Status != "OK" {
		t.Fatalf("терминал неверен: %+v", term)
	}
	if len(streamed) != 2 || streamed[0].Type != "ready" || streamed[1].Type != "log" {
		t.Fatalf("стрим неверен: %+v", streamed)
	}
	if ft.sent[0].Type != "run" || ft.sent[0].Dir != "scripts/x" {
		t.Fatalf("ожидали run{scripts/x}, got %+v", ft.sent[0])
	}
}

func TestClientRunTerminatesOnError(t *testing.T) {
	ft := &fakeTransport{events: []Event{{Type: "error", Message: "nope"}}}
	c := NewClient(ft)
	term, err := c.Run("bad", func(Event) {})
	if err != nil {
		t.Fatalf("Run вернул err вместо терминального события: %v", err)
	}
	if term.Type != "error" || term.Message != "nope" {
		t.Fatalf("ожидали error-терминал, got %+v", term)
	}
}
```

- [ ] **Step 5: Запустить — убедиться, что падает**

Run: `cd apps/deview && go test ./internal/bridge/`
Expected: FAIL — нет `NewClient`.

- [ ] **Step 6: Реализовать клиент (`client.go`)**

Create `apps/deview/internal/bridge/client.go`:

```go
package bridge

// Client — тонкая обёртка над Transport с операциями протокола моста.
type Client struct{ t Transport }

func NewClient(t Transport) *Client { return &Client{t: t} }

// List шлёт list и читает события до catalog.
func (c *Client) List() ([]Script, error) {
	if err := c.t.Send(Command{Type: "list"}); err != nil {
		return nil, err
	}
	for {
		e, err := c.t.Recv()
		if err != nil {
			return nil, err
		}
		if e.Type == "catalog" {
			return e.Scripts, nil
		}
		// прочие события до catalog игнорируем (в v1 их нет)
	}
}

// Run шлёт run{dir}, вызывает onEvent на ready/log и возвращает терминальное
// событие (result или error). Ошибку возвращает лишь при обрыве транспорта.
func (c *Client) Run(dir string, onEvent func(Event)) (Event, error) {
	if err := c.t.Send(Command{Type: "run", Dir: dir}); err != nil {
		return Event{}, err
	}
	for {
		e, err := c.t.Recv()
		if err != nil {
			return Event{}, err
		}
		switch e.Type {
		case "ready", "log":
			if onEvent != nil {
				onEvent(e)
			}
		case "result", "error":
			return e, nil
		}
	}
}

// Cancel просит мост отменить текущий прогон.
func (c *Client) Cancel() error { return c.t.Send(Command{Type: "cancel"}) }

// Close завершает мост и освобождает ресурсы транспорта.
func (c *Client) Close() error { return c.t.Close() }
```

- [ ] **Step 7: Запустить тесты клиента**

Run: `cd apps/deview && go test ./internal/bridge/ -v`
Expected: PASS (три теста).

- [ ] **Step 8: Commit** (только с явного разрешения)

```bash
git add apps/deview/go.mod apps/deview/internal/bridge/ go.work
git commit -m "feat(deview): bridge client module (transport, codec, client)"
```

---

## Task 6: `deview/ui` — рендер меню и событий

**Files:**
- Create: `apps/deview/internal/ui/menu.go`
- Create: `apps/deview/internal/ui/render.go`
- Test: `apps/deview/internal/ui/render_test.go`

**Interfaces:**
- Consumes: `bridge.Script`, `bridge.Event`
- Produces:
  - `func RenderMenu(scripts []bridge.Script) string`
  - `func RenderLog(e bridge.Event) string`
  - `func RenderResult(e bridge.Event) string`

- [ ] **Step 1: Написать падающий тест рендера**

Create `apps/deview/internal/ui/render_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	"wire-auto/apps/deview/internal/bridge"
)

func TestRenderMenuNumbersAndBadges(t *testing.T) {
	out := RenderMenu([]bridge.Script{
		{Name: "hello", Language: "python", Capabilities: []string{}},
		{Name: "flash", Language: "bash", Capabilities: []string{"serial"}},
	})
	if !strings.Contains(out, "1) hello") || !strings.Contains(out, "2) flash") {
		t.Fatalf("нет нумерации:\n%s", out)
	}
	if !strings.Contains(out, "python") || !strings.Contains(out, "serial") {
		t.Fatalf("нет бейджей языка/возможностей:\n%s", out)
	}
}

func TestRenderMenuEmpty(t *testing.T) {
	out := RenderMenu(nil)
	if !strings.Contains(strings.ToLower(out), "нет скриптов") {
		t.Fatalf("пустой каталог должен сообщать об отсутствии скриптов:\n%s", out)
	}
}

func TestRenderResultOKAndError(t *testing.T) {
	ok := RenderResult(bridge.Event{Type: "result", Status: "OK", ExitCode: 0})
	if !strings.Contains(ok, "OK") {
		t.Fatalf("нет OK: %s", ok)
	}
	bad := RenderResult(bridge.Event{Type: "result", Status: "SCRIPT_ERROR", ExitCode: 2})
	if !strings.Contains(bad, "SCRIPT_ERROR") || !strings.Contains(bad, "2") {
		t.Fatalf("нет статуса/кода ошибки: %s", bad)
	}
	er := RenderResult(bridge.Event{Type: "error", Message: "no such dir"})
	if !strings.Contains(er, "no such dir") {
		t.Fatalf("нет текста ошибки моста: %s", er)
	}
}

func TestRenderLogLevel(t *testing.T) {
	out := RenderLog(bridge.Event{Type: "log", Level: "warn", Message: "careful"})
	if !strings.Contains(out, "careful") {
		t.Fatalf("нет текста лога: %s", out)
	}
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd apps/deview && go test ./internal/ui/`
Expected: FAIL — нет пакета `ui`.

- [ ] **Step 3: Реализовать меню (`menu.go`)**

Create `apps/deview/internal/ui/menu.go`:

```go
// Package ui рендерит консольный вывод deview: меню и события прогона.
// Только форматирование строк — ввод/вывод живёт в cmd/deview.
package ui

import (
	"fmt"
	"strings"

	"wire-auto/apps/deview/internal/bridge"
)

// RenderMenu форматирует нумерованный список скриптов с бейджами.
func RenderMenu(scripts []bridge.Script) string {
	if len(scripts) == 0 {
		return "Нет скриптов в каталоге.\n"
	}
	var b strings.Builder
	b.WriteString("Доступные скрипты:\n")
	for i, s := range scripts {
		badges := s.Language
		if len(s.Capabilities) > 0 {
			badges += " · " + strings.Join(s.Capabilities, ",")
		}
		if badges != "" {
			badges = "  [" + badges + "]"
		}
		fmt.Fprintf(&b, "  %d) %s%s\n", i+1, s.Name, badges)
	}
	b.WriteString("\nНомер — запустить, q — выход: ")
	return b.String()
}
```

- [ ] **Step 4: Реализовать рендер событий (`render.go`)**

Create `apps/deview/internal/ui/render.go`:

```go
package ui

import (
	"fmt"

	"wire-auto/apps/deview/internal/bridge"
)

// ANSI-цвета; при желании можно отключить, если stdout не терминал.
const (
	cReset  = "\033[0m"
	cGray   = "\033[90m"
	cYellow = "\033[33m"
	cRed    = "\033[31m"
	cGreen  = "\033[32m"
)

func levelColor(level string) string {
	switch level {
	case "warn":
		return cYellow
	case "error":
		return cRed
	default:
		return cGray
	}
}

// RenderLog форматирует одну строку лога с цветом по уровню.
func RenderLog(e bridge.Event) string {
	lvl := e.Level
	if lvl == "" {
		lvl = "info"
	}
	return fmt.Sprintf("  %s%-5s%s %s", levelColor(lvl), lvl, cReset, e.Message)
}

// RenderResult форматирует терминальное событие прогона (result или error).
func RenderResult(e bridge.Event) string {
	if e.Type == "error" {
		return fmt.Sprintf("%s✗ ошибка моста:%s %s", cRed, cReset, e.Message)
	}
	if e.Status == "OK" {
		line := fmt.Sprintf("%s✔ OK%s", cGreen, cReset)
		if len(e.Result) > 0 {
			line += fmt.Sprintf("  результат: %s", string(e.Result))
		}
		return line
	}
	line := fmt.Sprintf("%s✗ %s%s (код %d)", cRed, e.Status, cReset, e.ExitCode)
	if e.ErrorCode != "" {
		line += fmt.Sprintf(" [%s] %s", e.ErrorCode, e.ErrorMessage)
	}
	return line
}
```

- [ ] **Step 5: Запустить тесты ui**

Run: `cd apps/deview && go test ./internal/ui/ -v`
Expected: PASS (четыре теста).

- [ ] **Step 6: Commit** (только с явного разрешения)

```bash
git add apps/deview/internal/ui/
git commit -m "feat(deview): menu and run-event renderers"
```

---

## Task 7: `cmd/deview` — REPL-цикл + сборка

**Files:**
- Create: `apps/deview/cmd/deview/main.go`
- Test: ручной (интеграция) — шаги ниже

**Interfaces:**
- Consumes: `bridge.NewProcessTransport`, `bridge.NewClient`, `ui.RenderMenu`, `ui.RenderLog`, `ui.RenderResult`

- [ ] **Step 1: Реализовать точку входа (`main.go`)**

Create `apps/deview/cmd/deview/main.go`:

```go
// Command deview — консольный браузер скриптов: поднимает мост wire, показывает
// нумерованное меню и рисует живой ход выполнения выбранного скрипта.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"wire-auto/apps/deview/internal/bridge"
	"wire-auto/apps/deview/internal/ui"
)

func main() {
	wireBin := flag.String("wire", "", "path to the wire bridge binary (default: go run ./runtime/basic/cmd/wire)")
	flag.Parse()

	tr, err := startBridge(*wireBin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "не удалось запустить мост wire:", err)
		os.Exit(1)
	}
	client := bridge.NewClient(tr)
	defer client.Close()

	reader := bufio.NewScanner(os.Stdin)
	for {
		scripts, err := client.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ошибка каталога:", err)
			return
		}
		fmt.Print(ui.RenderMenu(scripts))

		if !reader.Scan() {
			return
		}
		choice := strings.TrimSpace(reader.Text())
		if choice == "q" || choice == "quit" || choice == "exit" {
			return
		}
		n, err := strconv.Atoi(choice)
		if err != nil || n < 1 || n > len(scripts) {
			fmt.Println("Неверный выбор.")
			continue
		}
		runOne(client, scripts[n-1])
	}
}

// runOne запускает выбранный скрипт и рисует его ход. Ctrl-C во время прогона
// шлёт cancel мосту (а не убивает deview).
func runOne(client *bridge.Client, s bridge.Script) {
	fmt.Printf("\n▶ %s\n", s.Name)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
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

- [ ] **Step 2: Реализовать `startBridge` (выбор бинарника vs go run)**

Добавь в тот же `main.go` функцию:

```go
// startBridge выбирает, как поднять мост: указанный бинарник или dev-режим
// через `go run`. Возвращает готовый транспорт.
func startBridge(wireBin string) (bridge.Transport, error) {
	if wireBin != "" {
		return bridge.NewProcessTransport(wireBin)
	}
	if env := os.Getenv("WIRE_BIN"); env != "" {
		return bridge.NewProcessTransport(env)
	}
	// dev-умолчание: запускать из корня репозитория.
	return bridge.NewProcessTransport("go", "run", "./runtime/basic/cmd/wire")
}
```

- [ ] **Step 3: Собрать и проверить весь модуль deview**

Run: `cd apps/deview && go build ./... && go vet ./... && go test ./...`
Expected: PASS (тесты bridge/ui зелёные, main компилируется).

- [ ] **Step 4: Ручная интеграция — прогнать deview над hello**

Run (из корня репозитория): `go run ./apps/deview/cmd/deview`
Expected:
- печатает меню со скриптом `hello` (и любыми из `scripts/examples/`);
- ввод `1` → `▶ hello`, `⏳ выполняется…`, строка `info  hello from python`, затем `✔ OK`;
- возврат в меню; ввод `q` → выход.

Если `python` не в PATH — прогон завершится `STARTUP_TIMEOUT`/`CRASHED`; это корректно отражается как `✗ <status>`.

- [ ] **Step 5: Проверка гонок на интеграции моста (рантайм-сторона)**

Run: `cd runtime/basic && go test -race ./...`
Expected: PASS без `-race` предупреждений (проверяет конкурентный writer/cancel в `bridge.Serve`).

- [ ] **Step 6: Commit** (только с явного разрешения)

```bash
git add apps/deview/cmd/
git commit -m "feat(deview): interactive menu REPL with live run rendering and cancel"
```

---

## Task 8: Документация `guide/`

**Files:**
- Create: `guide/demo/app-runtime-bridge.md`
- Create: `guide/demo/apps-deview.md`
- Modify: `guide/demo/01-overview.md` (apps больше не пусты; первый клиент)
- Modify: `guide/demo/running.md` (wire — долгоживущий мост)
- Modify: `guide/demo/README.md` (добавить строки индекса)

> `guide/courses/01-welcome.md`, `guide/courses/README.md`, `guide/README.md` уже созданы на этапе брейншторминга — здесь не трогаем.

- [ ] **Step 1: Написать `app-runtime-bridge.md`**

Create `guide/demo/app-runtime-bridge.md` с содержанием: назначение верхней границы app↔runtime; таблицы команд (`list`/`run`/`cancel`/`exit`) и событий (`catalog`/`ready`/`log`/`result`/`error`) — скопировать из спеки §3; пример диалога:

```
app → {"type":"list"}
rt  → {"type":"catalog","scripts":[{"name":"hello","dir":"...","capabilities":[]}]}
app → {"type":"run","dir":"scripts/examples/hello"}
rt  → {"type":"ready"}
rt  → {"type":"log","level":"info","message":"hello from python"}
rt  → {"type":"result","status":"OK","exitCode":0}
app → {"type":"exit"}
```

Плюс раздел «стриминг необязателен»: ядро, отдающее данные разом, кладёт их в
`result.result`; для клиента форма одна — последовательность до `result`.

- [ ] **Step 2: Написать `apps-deview.md`**

Create `guide/demo/apps-deview.md`: что такое deview; как запустить
(`go run ./apps/deview/cmd/deview`, флаг `--wire`/env `WIRE_BIN`); раскладка
`internal/bridge` + `internal/ui`; поток работы (меню → номер → живой лог →
result → меню; `q` — выход; Ctrl-C — cancel).

- [ ] **Step 3: Обновить `01-overview.md`**

В `guide/demo/01-overview.md` заменить упоминание «Сейчас `apps/` пусты; первый
клиент — CLI-команда `wire`» на актуальное: `apps/` содержит `deview` — первый
клиент; граница app↔runtime теперь живой двусторонний мост (ссылка на
`app-runtime-bridge.md`). В блоке «Зоны репозитория» убрать `apps/` из «пока пусто».

- [ ] **Step 4: Обновить `running.md`**

В `guide/demo/running.md` заменить раздел про разовый `wire <script-dir>` на:
`wire` — долгоживущий мост (stdin команды / stdout события), напрямую руками не
запускается; для запуска скриптов — `deview` (пример) либо пайп JSON-команд:

```bash
printf '%s\n' '{"type":"list"}' '{"type":"exit"}' | go run ./runtime/basic/cmd/wire
```

Go-тесты/vet-команды оставить как есть.

- [ ] **Step 5: Обновить индекс `guide/demo/README.md`**

Добавить в таблицу содержания две строки:

```markdown
| [app-runtime-bridge.md](app-runtime-bridge.md) | Верхняя граница app↔runtime: команды list/run/cancel/exit, события catalog/ready/log/result/error, двусторонний мост |
| [apps-deview.md](apps-deview.md) | Консольный клиент deview: браузер скриптов + живой рендер; запуск, флаги, поток работы |
```

- [ ] **Step 6: Проверить ссылки и согласованность**

Прочитать созданные/изменённые файлы; убедиться, что все относительные ссылки
существуют, а описания соответствуют реализованному протоколу (§3 спеки).

- [ ] **Step 7: Commit** (только с явного разрешения)

```bash
git add guide/
git commit -m "docs: bridge protocol and deview client guide topics"
```

---

## Финальная проверка (после всех задач)

- [ ] **Полный прогон рантайма:** `cd runtime/basic && go build ./... && go vet ./... && go test -race ./...` → всё PASS.
- [ ] **Полный прогон deview:** `cd apps/deview && go build ./... && go vet ./... && go test ./...` → всё PASS.
- [ ] **Workspace-сборка из корня:** `go build ./... && go test ./...` → всё PASS.
- [ ] **Ручной smoke:** `go run ./apps/deview/cmd/deview` → меню → `1` → живой лог → `✔ OK` → `q`.

---

## Self-review плана против спеки

**Покрытие спеки:**
- §3 протокол моста → Task 3 (сервер) + Task 5 (клиент). ✓
- §4 правки рантайма (context/OnEvent/Result payload/CANCELLED) → Task 1; discovery → Task 2; bridge → Task 3; cmd/wire всегда мост → Task 4. ✓
- §5 клиент deview (модуль, транспорт, client, ui, REPL, поиск wire, go.work) → Tasks 5–7. ✓
- §6 документация → `guide/courses/*` уже готовы; demo-топики → Task 8. ✓
- §7 v1-минимум (один стандарт, один прогон за раз, плоский скан) → отражено в Tasks 2/3/6. ✓
- §8 тесты (discovery, цикл моста, стриминг, отмена, resilience, кодек, рендер) → Tasks 2/3/5/6 + `-race` в 7. ✓
- §9 риски (устаревание running.md, изоляция паники, SIGINT→cancel) → Task 8 step 4, Task 3 step 4 (recover), Task 7 step 1. ✓

**Согласованность типов:** `bridge.Script`/`bridge.Event`/`bridge.Command` совпадают по полям и json-тегам между `runtime/basic/internal/bridge` и `apps/deview/internal/bridge`. `exec.Event{Kind,Level,Message}` одинаково используется в Task 1/3/4. `runStreaming`/`run` сигнатуры совпадают между Task 4 и вызовами в тестах.

**Плейсхолдеры:** нет TBD/«обработать ошибки»/«аналогично Task N»; весь код приведён.

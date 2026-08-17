# port-scan → Go-скрипт (скан всех портов) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Переписать демо-скрипт `scripts/examples/port-scan/` с Python на «толстый» Go-скрипт, который сам конкурентно сканит все порты 1–65535 на localhost, и научить ядро `duplex` спавнить Go-скрипты герметично (`GOWORK=off`).

**Architecture:** Ядро получает одну минимальную правку — добавляет `GOWORK=off` в env спавнимого процесса, чтобы `go run .` работал в модуле вне корневого `go.work`. Сам скрипт становится самостоятельным Go-модулем на stdlib: конкурентный worker-pool через `net.DialTimeout` (не через ядерные `net.*` capability), общение с ядром — по сырому JSON-Lines-протоколу (hello → ready → log → done).

**Tech Stack:** Go 1.26 (stdlib only: `net`, `encoding/json`, `bufio`, `os`, `sync`, `sort`, `strconv`, `time`, `fmt`). Ядро — существующий `cores/duplex`.

## Global Constraints

- **Git-дисциплина (жёстко):** НЕ создавать ветки, НЕ коммитить, НЕ пушить без явной просьбы пользователя в том же сообщении. Шаги «Commit» в этом плане выполняются, только если пользователь явно разрешил коммитить; иначе — пропустить коммит, оставить изменения в рабочем дереве.
- **Каждый Go-модуль собирается независимо:** проверять `GOWORK=off go build ./... && go vet ./... && go test ./...` внутри модуля. Никаких скрытых кросс-модульных `replace`.
- **Go-скрипты запускаются с `GOWORK=off`** — переменную выставляет ядро при спавне (манифест env не несёт).
- **Версия Go в новых `go.mod`:** `go 1.26`.
- **Нет внешних зависимостей** — только stdlib.
- **Не спамим логами:** логируется каждый ОТКРЫТЫЙ порт + один итог. Закрытые/фильтрованные не логируются.
- **Не трогаем:** `capreg`, реестр возможностей, `handshake`, `protocol`, мост, exec-цикл, deview, ядерные `net.*`.
- **НЕ добавлять `port-scan` и `gohello` в корневой `go.work`** — весь смысл в том, что они запускаются standalone с `GOWORK=off`.
- **Пути к файлам:** всегда абсолютные Windows-пути с буквой диска и обратными слэшами для файловых операций.

---

## File Structure

**Ядро (правка + testdata):**
- Modify: `D:\Projects\wire-auto\cores\duplex\cmd\core\main.go` — добавить `"GOWORK=off"` в `Env`.
- Create: `D:\Projects\wire-auto\cores\duplex\cmd\core\testdata\gohello\go.mod` — минимальный Go-модуль.
- Create: `D:\Projects\wire-auto\cores\duplex\cmd\core\testdata\gohello\main.go` — ready+done, без сети.
- Create: `D:\Projects\wire-auto\cores\duplex\cmd\core\testdata\gohello\script.manifest` — `language="go"`, `cmd=["go","run","."]`.
- Modify: `D:\Projects\wire-auto\cores\duplex\cmd\core\main_test.go` — e2e-тест `TestE2EGoHello`.

**Скрипт port-scan (новый Go-модуль, заменяет Python):**
- Delete: `D:\Projects\wire-auto\scripts\examples\port-scan\main.py`
- Create: `D:\Projects\wire-auto\scripts\examples\port-scan\go.mod`
- Create: `D:\Projects\wire-auto\scripts\examples\port-scan\scan.go` — `scanPorts(...)`, тестируемая логика без протокола.
- Create: `D:\Projects\wire-auto\scripts\examples\port-scan\scan_test.go` — юнит-тест `TestScanPorts`.
- Create: `D:\Projects\wire-auto\scripts\examples\port-scan\main.go` — протокол + константы.
- Modify: `D:\Projects\wire-auto\scripts\examples\port-scan\script.manifest` — Go-версия.

**Документация:**
- Modify: `D:\Projects\wire-auto\guide\demo\writing-a-script.md` — раздел «Скрипт на Go».
- Modify: `D:\Projects\wire-auto\guide\demo\capabilities.md` — обновить пример port-scan.

---

## Task 1: Ядро выставляет `GOWORK=off` + Go-скрипт стартует через ядро

**Files:**
- Modify: `D:\Projects\wire-auto\cores\duplex\cmd\core\main.go:70`
- Create: `D:\Projects\wire-auto\cores\duplex\cmd\core\testdata\gohello\go.mod`
- Create: `D:\Projects\wire-auto\cores\duplex\cmd\core\testdata\gohello\main.go`
- Create: `D:\Projects\wire-auto\cores\duplex\cmd\core\testdata\gohello\script.manifest`
- Test: `D:\Projects\wire-auto\cores\duplex\cmd\core\main_test.go` (добавить `TestE2EGoHello`)

**Interfaces:**
- Consumes: `run(coreManifestPath, scriptDir string) (exec.Result, error)` — существующая тонкая обёртка (main.go:86); `coreManifest(t *testing.T) string` — хелпер в main_test.go:14; `exec.Result{Status, ErrorCode, ErrorMessage}`.
- Produces: рабочий Go-скрипт `testdata/gohello` (raw JSON-Lines: читает hello → шлёт `ready` → шлёт `done{result:{ok:true}}`), спавнимый через ядро с `GOWORK=off`.

Порядок: сначала пишем упавший e2e-тест (он падает без правки ядра из-за `go.work`), потом создаём testdata-скрипт, потом правим ядро — и тест зеленеет.

- [ ] **Step 1: Создать go.mod для testdata-скрипта**

Файл `D:\Projects\wire-auto\cores\duplex\cmd\core\testdata\gohello\go.mod`:

```
module gohello

go 1.26
```

- [ ] **Step 2: Создать main.go для testdata-скрипта**

Файл `D:\Projects\wire-auto\cores\duplex\cmd\core\testdata\gohello\main.go`:

```go
// Command gohello — минимальный Go-скрипт для e2e: читает hello, отвечает
// ready и done{ok:true}. Без сети — быстрый и детерминированный. Доказывает,
// что ядро проставляет GOWORK=off (иначе `go run .` упал бы на корневом go.work).
package main

import (
	"bufio"
	"encoding/json"
	"os"
)

func main() {
	r := bufio.NewReader(os.Stdin)
	r.ReadString('\n') // hello — говорим на протоколе напрямую, без SDK

	enc := json.NewEncoder(os.Stdout) // Encode пишет компактный JSON + '\n'
	enc.Encode(map[string]any{"type": "ready"})
	enc.Encode(map[string]any{
		"type":     "done",
		"exitCode": 0,
		"result":   map[string]any{"ok": true},
	})
}
```

- [ ] **Step 3: Создать script.manifest для testdata-скрипта**

Файл `D:\Projects\wire-auto\cores\duplex\cmd\core\testdata\gohello\script.manifest`:

```toml
name = "gohello"
version = "0.1.0"
core = "duplex"
coreApi = 1
capabilities = []
link = "stdio"
language = "go"
cmd = ["go", "run", "."]
```

- [ ] **Step 4: Написать упавший e2e-тест**

Добавить в конец `D:\Projects\wire-auto\cores\duplex\cmd\core\main_test.go` (перед этим убедиться, что импорты `filepath` и `testing` уже есть — они есть):

```go
func TestE2EGoHello(t *testing.T) {
	dir, err := filepath.Abs("testdata/gohello")
	if err != nil {
		t.Fatal(err)
	}
	res, err := run(coreManifest(t), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "OK" {
		t.Fatalf("status=%s err=%s %s", res.Status, res.ErrorCode, res.ErrorMessage)
	}
}
```

- [ ] **Step 5: Прогнать тест — убедиться, что падает**

Run: `cd D:/Projects/wire-auto/cores/duplex && GOWORK=off go test ./cmd/core/ -run TestE2EGoHello -v`
Expected: FAIL — `go run .` в `testdata/gohello` падает с `current directory is contained in a module that is not one of the workspace modules` (если активен корневой `go.work`), либо `res.Status` не `OK`. Это доказывает, что без правки ядра Go-скрипт не стартует в dev-окружении.

Примечание: если у исполнителя `go.work` не активен (переменная `GOWORK=off` в оболочке), тест может пройти и без правки. В этом случае всё равно выполнить Step 6 — правка обязательна по спеке, чтобы скрипт был герметичен в любом окружении.

- [ ] **Step 6: Добавить `GOWORK=off` в env ядра**

В `D:\Projects\wire-auto\cores\duplex\cmd\core\main.go`, функция `runStreaming`, строка 70 — заменить:

```go
		Env:            []string{"WIRE_SDK_DIR=" + sdkDir},
```

на:

```go
		Env:            []string{"WIRE_SDK_DIR=" + sdkDir, "GOWORK=off"},
```

(`exec.Run` делает `cmd.Env = append(os.Environ(), spec.Env...)`; при дубле ключа побеждает последнее значение, так что `GOWORK=off` перекрывает внешний `GOWORK`. Для Python/Node безвредно.)

- [ ] **Step 7: Прогнать тест — убедиться, что проходит**

Run: `cd D:/Projects/wire-auto/cores/duplex && GOWORK=off go test ./cmd/core/ -run TestE2EGoHello -v`
Expected: PASS.

- [ ] **Step 8: Прогнать все e2e ядра — python/node остались зелёными**

Run: `cd D:/Projects/wire-auto/cores/duplex && GOWORK=off go build ./... && go vet ./... && go test ./...`
Expected: PASS (в т.ч. `TestE2EEnvReport`, `TestE2EEnvReportNode`, `TestE2EPromptViaBridge`, `TestE2EGoHello`). Nested-модуль `testdata/gohello` игнорируется `./...` родителя (у него свой go.mod).

- [ ] **Step 9: Commit (только если пользователь явно разрешил)**

```bash
cd D:/Projects/wire-auto
git add cores/duplex/cmd/core/main.go cores/duplex/cmd/core/main_test.go cores/duplex/cmd/core/testdata/gohello
git commit -m "feat(duplex): spawn scripts with GOWORK=off; Go e2e testdata"
```

---

## Task 2: `scanPorts` — конкурентная логика скана (юнит-тест)

**Files:**
- Create: `D:\Projects\wire-auto\scripts\examples\port-scan\go.mod`
- Create: `D:\Projects\wire-auto\scripts\examples\port-scan\scan.go`
- Test: `D:\Projects\wire-auto\scripts\examples\port-scan\scan_test.go`

**Interfaces:**
- Consumes: stdlib `net`, `sync`, `sort`, `strconv`, `time`.
- Produces: `func scanPorts(host string, ports []int, workers int, timeout time.Duration) []int` (package `main`) — worker-pool, возвращает отсортированный список открытых портов; ошибка (refuse/timeout) = «не открыт», без паники. Используется в Task 3.

TDD: тест пишем первым.

- [ ] **Step 1: Создать go.mod модуля port-scan**

Файл `D:\Projects\wire-auto\scripts\examples\port-scan\go.mod`:

```
module port-scan

go 1.26
```

- [ ] **Step 2: Написать упавший юнит-тест**

Файл `D:\Projects\wire-auto\scripts\examples\port-scan\scan_test.go`:

```go
package main

import (
	"net"
	"testing"
	"time"
)

func TestScanPorts(t *testing.T) {
	// открытый порт: слушаем и НЕ закрываем
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	openPort := ln.Addr().(*net.TCPAddr).Port

	// заведомо закрытый порт: слушаем, узнаём номер, закрываем
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := ln2.Addr().(*net.TCPAddr).Port
	ln2.Close()

	open := scanPorts("127.0.0.1", []int{openPort, closedPort}, 8, 500*time.Millisecond)

	found := map[int]bool{}
	for _, p := range open {
		found[p] = true
	}
	if !found[openPort] {
		t.Errorf("открытый порт %d не найден в %v", openPort, open)
	}
	if found[closedPort] {
		t.Errorf("закрытый порт %d ошибочно найден в %v", closedPort, open)
	}
}
```

- [ ] **Step 3: Прогнать тест — убедиться, что не компилируется/падает**

Run: `cd D:/Projects/wire-auto/scripts/examples/port-scan && GOWORK=off go test ./...`
Expected: FAIL — `undefined: scanPorts`.

- [ ] **Step 4: Реализовать scanPorts**

Файл `D:\Projects\wire-auto\scripts\examples\port-scan\scan.go`:

```go
package main

import (
	"net"
	"sort"
	"strconv"
	"sync"
	"time"
)

// scanPorts конкурентно прозванивает порты host через worker-pool из workers
// горутин: каждая тянет порт из канала и делает net.DialTimeout. Успешный
// connect → порт открыт (conn закрываем). Ошибка (refuse/timeout) — просто
// «не открыт», без паники. Возвращает отсортированный список открытых портов.
func scanPorts(host string, ports []int, workers int, timeout time.Duration) []int {
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan int)
	results := make(chan int)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range jobs {
				addr := net.JoinHostPort(host, strconv.Itoa(port))
				conn, err := net.DialTimeout("tcp", addr, timeout)
				if err != nil {
					continue // отказ/таймаут — не открыт
				}
				conn.Close()
				results <- port
			}
		}()
	}

	go func() {
		for _, p := range ports {
			jobs <- p
		}
		close(jobs)
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	open := []int{}
	for p := range results {
		open = append(open, p)
	}
	sort.Ints(open)
	return open
}
```

- [ ] **Step 5: Прогнать тест — убедиться, что проходит**

Run: `cd D:/Projects/wire-auto/scripts/examples/port-scan && GOWORK=off go test ./... -v`
Expected: PASS (`TestScanPorts`).

- [ ] **Step 6: Commit (только если пользователь явно разрешил)**

```bash
cd D:/Projects/wire-auto
git add scripts/examples/port-scan/go.mod scripts/examples/port-scan/scan.go scripts/examples/port-scan/scan_test.go
git commit -m "feat(port-scan): concurrent scanPorts with unit test"
```

---

## Task 3: port-scan `main.go` — протокол + скан всех 65535 портов, удалить Python

**Files:**
- Create: `D:\Projects\wire-auto\scripts\examples\port-scan\main.go`
- Modify: `D:\Projects\wire-auto\scripts\examples\port-scan\script.manifest`
- Delete: `D:\Projects\wire-auto\scripts\examples\port-scan\main.py`

**Interfaces:**
- Consumes: `scanPorts(host string, ports []int, workers int, timeout time.Duration) []int` из Task 2.
- Produces: исполняемый `port-scan` Go-модуль: hello → `ready` → скан `1..65535` на `127.0.0.1` → лог по каждому открытому + два итоговых лога → `done{result:{host, open, scanned:65535}}`.

- [ ] **Step 1: Заменить script.manifest на Go-версию**

Перезаписать `D:\Projects\wire-auto\scripts\examples\port-scan\script.manifest`:

```toml
name = "port-scan"
version = "0.2.0"
core = "duplex"
coreApi = 1
capabilities = []
link = "stdio"
language = "go"
cmd = ["go", "run", "."]
```

- [ ] **Step 2: Написать main.go (протокол + константы)**

Файл `D:\Projects\wire-auto\scripts\examples\port-scan\main.go`:

```go
// Command port-scan — «толстый» Go-скрипт: сам конкурентно сканит все порты
// 1..65535 на localhost через scanPorts (net.DialTimeout в горутинах), не
// используя ядерные net.* capability. С ядром общается по сырому JSON-Lines-
// протоколу: hello → ready → log → done.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	host    = "127.0.0.1"
	workers = 1000
	timeout = 500 * time.Millisecond
)

func main() {
	r := bufio.NewReader(os.Stdin)
	enc := json.NewEncoder(os.Stdout) // Encode пишет компактный JSON + '\n'

	r.ReadString('\n') // hello — говорим на протоколе напрямую, без SDK
	enc.Encode(map[string]any{"type": "ready"})

	ports := make([]int, 0, 65535)
	for p := 1; p <= 65535; p++ {
		ports = append(ports, p)
	}

	open := scanPorts(host, ports, workers, timeout)

	for _, p := range open {
		enc.Encode(map[string]any{
			"type": "log", "level": "info",
			"message": fmt.Sprintf("port %d OPEN", p),
		})
	}
	enc.Encode(map[string]any{
		"type": "log", "level": "info",
		"message": fmt.Sprintf("open ports: %v", open),
	})
	enc.Encode(map[string]any{
		"type": "log", "level": "info",
		"message": "scanned 65535",
	})
	enc.Encode(map[string]any{
		"type":     "done",
		"exitCode": 0,
		"result":   map[string]any{"host": host, "open": open, "scanned": 65535},
	})
}
```

- [ ] **Step 3: Удалить Python-версию**

```bash
cd D:/Projects/wire-auto
git rm scripts/examples/port-scan/main.py 2>/dev/null || rm -f scripts/examples/port-scan/main.py
```

- [ ] **Step 4: Собрать/вётнуть/протестировать модуль независимо**

Run: `cd D:/Projects/wire-auto/scripts/examples/port-scan && GOWORK=off go build ./... && go vet ./... && go test ./...`
Expected: PASS — модуль собирается, `go vet` чист, `TestScanPorts` зелёный.

- [ ] **Step 5: Ручная e2e-проверка через ядро**

Run (из корня, с активным go.work — ядро само выставит GOWORK=off скрипту):

```bash
cd D:/Projects/wire-auto
printf '{"type":"run","dir":"scripts/examples/port-scan"}\n{"type":"exit"}\n' | go run ./cores/duplex/cmd/core -core cores/duplex/core.manifest
```

Expected: в stdout есть `"type":"ready"`, затем строки логов, и финальный `"status":"OK"` с `result`, содержащим `"host":"127.0.0.1"`, `"scanned":65535` и массив `"open"`. Полный скан должен уложиться в `RunTimeout` (60с) — на localhost закрытые порты дают мгновенный refuse.

Если открытых портов нет — это тоже валидно (`open: []`); главное `status: OK` и `scanned: 65535`.

- [ ] **Step 6: Commit (только если пользователь явно разрешил)**

```bash
cd D:/Projects/wire-auto
git add scripts/examples/port-scan/main.go scripts/examples/port-scan/script.manifest
git rm scripts/examples/port-scan/main.py
git commit -m "feat(port-scan): Go script scanning all 65535 ports; drop Python"
```

---

## Task 4: Документация

**Files:**
- Modify: `D:\Projects\wire-auto\guide\demo\writing-a-script.md`
- Modify: `D:\Projects\wire-auto\guide\demo\capabilities.md`

**Interfaces:**
- Consumes: факты из Task 1–3 (GOWORK=off ставит ядро; `language="go"`; `cmd=["go","run","."]`; толстый скрипт делает тяжёлую работу сам).
- Produces: раздел про Go-скрипты в writing-a-script.md; обновлённый пример port-scan в capabilities.md.

- [ ] **Step 1: Прочитать текущие доки**

Прочитать `D:\Projects\wire-auto\guide\demo\writing-a-script.md` и `D:\Projects\wire-auto\guide\demo\capabilities.md`, найти где упоминается port-scan / `net.tcp.connect`, чтобы вписать правки в правильные разделы (не создавать монолит — правило `guide/`: одна тема на файл, дописывать в нужный раздел).

- [ ] **Step 2: Добавить раздел «Скрипт на Go» в writing-a-script.md**

Добавить новый раздел (адаптировать заголовочный уровень под соседние разделы файла):

```markdown
## Скрипт на Go («толстый» скрипт)

Скрипт может быть на компилируемом языке. Пример — `scripts/examples/port-scan`:
самостоятельный Go-модуль (`go.mod` на stdlib, без внешних зависимостей),
который тяжёлую работу делает сам, а не через ядерные возможности.

Манифест:

​```toml
name = "port-scan"
version = "0.2.0"
core = "duplex"
coreApi = 1
capabilities = []          # capability не нужны — скрупулёзная работа своя
link = "stdio"
language = "go"
cmd = ["go", "run", "."]   # запуск модуля из папки скрипта
​```

**Изоляция через `GOWORK=off`.** При активном корневом `go.work` команда
`go run .` в папке скрипта (модуль не входит в workspace) падает. Поэтому ядро
при спавне любого скрипта выставляет `GOWORK=off` — Go-скрипт всегда герметичен
(изолирован от dev-workspace). Для Python/Node это безвредно: `GOWORK` — Go-
переменная, ими игнорируется. Env в манифесте задавать не нужно.

**Концепт «толстого» скрипта.** port-scan сам конкурентно прозванивает все
65535 портов через `net.DialTimeout` в горутинах (worker-pool), не используя
ядерные `net.*`. Так обходится последовательный синхронный диспатч `exec.Run`
(65535 портов через per-port capability не влезли бы в `RunTimeout`) — и видно,
зачем вообще может понадобиться скрипт на компилируемом языке.
```

(Символ `​` в примере выше — заглушка тройных бэктиков; при вставке заменить на ` ``` `.)

- [ ] **Step 3: Обновить пример port-scan в capabilities.md**

Найти раздел про пример прозвона портов и переписать: теперь `port-scan` — это
Go-скрипт, который сканит все 65535 портов сам, **без** `net.tcp.connect` и
`net.resolve`; ядерные `net.*` остаются для тонких скриптов, но этому примеру не
нужны. Убрать/поправить упоминание, что port-scan использует `net.tcp.connect`.

- [ ] **Step 4: Проверить консистентность и индекс**

Убедиться, что `guide/demo/README.md` (индекс) по-прежнему корректно ссылается на изменённые файлы; заголовки/ссылки не сломаны. Никаких новых монолитов не создано.

- [ ] **Step 5: Commit (только если пользователь явно разрешил)**

```bash
cd D:/Projects/wire-auto
git add guide/demo/writing-a-script.md guide/demo/capabilities.md
git commit -m "docs: Go scripts + port-scan now scans all ports without net.*"
```

---

## Финальная проверка (после всех задач)

- [ ] **Ядро зелёное независимо:** `cd D:/Projects/wire-auto/cores/duplex && GOWORK=off go build ./... && go vet ./... && go test ./...` → PASS.
- [ ] **Скрипт зелёный независимо:** `cd D:/Projects/wire-auto/scripts/examples/port-scan && GOWORK=off go build ./... && go vet ./... && go test ./...` → PASS.
- [ ] **Критерии готовности спеки выполнены:** GOWORK=off в env (Task 1); port-scan — Go, все 65535 портов, result OK (Task 3); scanPorts под юнит-тестом (Task 2); Go-testdata стартует через ядро (Task 1); оба модуля собираются независимо; доки обновлены (Task 4).

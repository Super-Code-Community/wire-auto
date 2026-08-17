# Duplex core — интерактивный ввод (prompt) через дуплекс-канал

- **Дата:** 2026-08-17
- **Ядро:** `cores/duplex` + клиент `apps/deview`
- **Статус:** дизайн утверждён, ждёт плана

## Цель

Дать скрипту возможность **по ходу выполнения** запросить у человека строку ввода
(аргумент), используя двунаправленность дуплекса. Скрипт шлёт `prompt`, ядро
пробрасывает запрос наверх клиенту (`deview`), клиент спрашивает пользователя,
значение возвращается скрипту. Это full-stack релей `deview → ядро → скрипт`:
у скрипта нет прямого доступа к терминалу (его stdin занят протоколом ядра).

## Модель (утверждено)

Подход **A**: `prompt` — отдельная сущность, НЕ capability. Ввод идёт от человека, а
не из `provides` ядра, поэтому prompt не проходит гейт рукопожатия, не требует
записи в `script.capabilities` и не трогает `capreg`. Код скрипта при этом
симметричен вызову capability (round-trip по `id`).

Поток одного prompt:
1. скрипт→ядро: `{"type":"prompt","id":"1","message":"Введите хост"}`
2. ядро→клиент (событие моста): `{"type":"prompt","id":"1","message":"Введите хост"}`
3. deview спрашивает пользователя, читает строку
4. клиент→ядро (команда моста): `{"type":"input","id":"1","value":"example.com"}`
5. ядро→скрипт: `{"type":"response","id":"1","result":{"value":"example.com"}}`

За раз висит максимум один prompt (скрипт синхронный: отправил — ждёт `response`).

## Объём по слоям

### 1. protocol.go (граница ядро↔скрипт)

- Новая константа `TypePrompt = "prompt"`.
- Скрипт шлёт `prompt`, используя существующие поля `ID` и `Message` (текст запроса).
- Ответ ядра переиспользует существующий `TypeResponse` с `ID` и
  `Result = {"value": "<строка>"}`. Новых полей в `protocol.Message` не добавляется
  (все нужные — `ID`, `Message`, `Result` — уже есть).

### 2. exec.go (насос ядро↔скрипт)

- `exec.Event` получает поле `ID string` (для события prompt).
- Новый вид события: `Event{Kind:"prompt", ID, Message}`.
- Новый тип и поле в `Spec`:
  ```go
  type PromptAnswer struct{ ID, Value string }
  // в Spec:
  Answers <-chan PromptAnswer
  ```
- В главном `select` `Run` добавляется обработка `case protocol.TypePrompt`:
  - при `spec.Protocol < 2` → `PROTOCOL_VIOLATION` (как для request);
  - при `!gotReady` → `PROTOCOL_VIOLATION` («prompt before ready»);
  - иначе: `emit(Event{Kind:"prompt", ID:req.ID, Message:req.Message})`, затем
    **ожидание ответа** во вложенном `select` на:
    - `answer := <-spec.Answers` → записать в stdin скрипта
      `response{ID:answer.ID, Result:{"value":answer.Value}}`, продолжить главный цикл;
    - `<-deadline` → как текущий RunTimeout (cancel+grace+kill);
    - `<-ctx.Done()` → как текущая отмена (cancel+grace+kill);
    - `ev := <-ch` (сообщение/ошибка от скрипта во время ожидания): `io.EOF` →
      `break loop` (crash-путь); иная ошибка → `PROTOCOL_VIOLATION`; любое
      сообщение от скрипта пока он должен ждать ответа → `PROTOCOL_VIOLATION`
      («unexpected message during prompt»).
  - Соответствие `answer.ID` и `req.ID`: ответ адресуется текущему prompt; если
    `answer.ID != req.ID`, всё равно отвечаем текущему prompt значением answer
    (за раз один outstanding prompt — mismatched id теоретически невозможен;
    матчинг не усложняем, но записываем в response `req.ID`).
- `Answers == nil` (когда prompt не ожидается): если скрипт шлёт `prompt`, а канал
  `nil`, ожидание на `nil`-канале блокирует навечно → защищает существующий
  RunTimeout. Для не-мостовых вызовов (`run()` в e2e без prompt) это безопасно,
  т.к. скрипт без prompt никогда не попадёт в этот путь.

### 3. bridge (cores/duplex, граница app↔core)

`message.go`:
- `Event` получает `ID string` (`json:"id,omitempty"`) — для события `prompt`
  (текст — в существующем `Message`).
- `Command` получает `ID string` (`json:"id,omitempty"`) и `Value string`
  (`json:"value,omitempty"`) — для команды `input`.

`serve.go`:
- `Deps.Run` меняет сигнатуру: добавляется параметр `answers <-chan exec.PromptAnswer`:
  ```go
  Run func(ctx context.Context, dir string, onEvent func(exec.Event), answers <-chan exec.PromptAnswer) (exec.Result, error)
  ```
- `Serve` на каждый прогон создаёт `answers := make(chan exec.PromptAnswer)` и
  передаёт его в `deps.Run`.
- `onEvent` получает кейс `"prompt"` → `write(Event{Type:"prompt", ID:ev.ID, Message:ev.Message})`.
- В `waitLoop` (во время прогона) добавляется `case "input"`:
  `answers <- exec.PromptAnswer{ID:nc.ID, Value:nc.Value}` — доставить ответ в exec.
  Отправка неблокирующе-безопасна: exec ждёт на `Answers` в момент prompt; но чтобы
  не подвиснуть, если prompt уже снят (гонка отмены), отправка идёт в `select` с
  `<-done` (прогон закончился → ответ выбрасываем).

`main.go` (`runStreaming`):
- Принимает `answers <-chan exec.PromptAnswer`, кладёт в `exec.Spec.Answers`.
- `run()` (обёртка без стриминга для e2e) передаёт `nil` в качестве answers.

### 4. apps/deview (клиент)

`internal/bridge/message.go`:
- `Event` получает `ID`; `Command` получает `ID` и `Value` (команда `input`).

`internal/bridge/client.go`:
- Клиент отдаёт наверх событие `prompt` (в существующем потоке событий) и получает
  метод отправки команды `input`:
  `func (c *Client) SendInput(id, value string) error`.

`internal/ui` + `cmd/deview/main.go`:
- На событие `prompt`: приостановить живой рендер лога, напечатать `message`,
  прочитать строку из stdin пользователя, вызвать `SendInput(id, line)`.
- Обработка помещается в существующий REPL/цикл событий прогона.

### 5. Демо-скрипт

`scripts/examples/ask-name/`:
- `script.manifest`: `core = "duplex"`, `capabilities = []` (prompt — не capability),
  `link = "stdio"`, `cmd = ["python","main.py"]`.
- `main.py` (протокол напрямую): `ready` → `prompt "Как тебя зовут?"` → читает
  `response.result.value` → `log "Привет, <имя>!"` → `done` с `result{name}`.

## Тестирование

- **exec:** `TestPromptRoundTrip` — скриптованный harness (как существующие exec-тесты):
  «скрипт» шлёт `prompt`, тест кладёт `PromptAnswer` в `Answers`, проверяет, что
  (а) событие `Kind:"prompt"` эмитнуто с нужными id/message, (б) в stdin скрипта
  записан `response{id, result:{value}}`. Плюс `TestPromptBeforeReady` →
  `PROTOCOL_VIOLATION`.
- **bridge (duplex):** `TestBridgePrompt` — фейковый `Run`, который через `onEvent`
  шлёт `prompt` и ждёт `answers`; тест подаёт команду `input`; проверяет, что вверх
  ушло событие `prompt` и что `Run` получил ответ и завершился.
- **deview:** round-trip сериализации `prompt` (event) и `input` (command) в
  `internal/bridge`.
- **e2e:** скрипт с prompt прогоняется через `cmd/core`; ввод подаётся пайпом
  команды `input` на stdin ядра; проверяется `result` со статусом OK и значением.
- Независимая сборка всех трёх модулей:
  `cores/duplex`, `apps/deview` — `GOWORK=off go build ./... && go vet ./... && go test ./...`.

## Документация

- Новый топик `guide/demo/prompts.md`: что такое интерактивный ввод, поток через обе
  границы (диаграмма шагов 1–5), политика (блокировка до ответа/таймаута), пример
  из `ask-name`.
- Ссылки из `guide/demo/README.md` и `guide/README.md` (индексы).
- Короткая заметка в `guide/demo/protocol.md` про тип `prompt` и в
  `guide/demo/app-core-bridge.md` про событие `prompt` / команду `input`.

## Не трогаем

- `capreg` и реестр возможностей (prompt — не capability).
- Рукопожатие (`handshake`) — prompt ничего не объявляет и не гейтится.
- `core.manifest`, `script.manifest`-поля (кроме нового демо-скрипта).

## Критерии готовности

1. Скрипт может отправить `prompt` и получить `response{value}`; за раз один outstanding.
2. `deview` на `prompt` спрашивает пользователя и возвращает ввод; живой лог
   корректно приостанавливается/возобновляется.
3. Демо `ask-name` отрабатывает через `deview` (интерактивно) и через пайп (e2e).
4. Все три модуля собираются/вётятся/тестируются независимо с `GOWORK=off`.
5. `guide/demo/prompts.md` создан и слинкован из индексов.
6. `capreg`, `handshake`, авторизация возможностей не затронуты.

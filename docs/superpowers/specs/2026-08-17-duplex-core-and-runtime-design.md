# cores/duplex + runtime/duplex — двусторонний контракт (v2)

Дата: 2026-08-17
Статус: черновик на согласование

## 1. Что это и зачем

v1 (`cores/regular` + `runtime/basic`) доказал «позвоночник»: три манифеста →
рукопожатие → спавн процесса → диалог по протоколу → результат. Но канал был
**односторонним** (`hello→ready→log→done`): скрипт умел только рапортовать. Поле
`provides` у ядра было декоративным — capability нечем было *вызвать*, потому что
в протоколе не было пары `request`/`response`.

v2 добавляет **двусторонний канал**. Скрипт шлёт `request` на объявленную
capability, хост исполняет и возвращает `response`. Это делает `provides` рабочим
гейтом авторизации и открывает место, к которому позже подключаются реальные бриджи
железа. Демо-capability v2 — `env.get` (чтение переменной окружения): кроссплатформенно,
без внешних зависимостей, показывает `request` с аргументом, `response` с payload и
путь ошибки.

**Старое не трогаем.** `cores/regular` и `runtime/basic` замораживаются как есть. v2 —
новые контейнеры рядом: ядро `cores/duplex` и рантайм `runtime/duplex`.

**Языковая универсальность — zero-shim.** Контракт настолько прост, что любой язык
говорит его сырьём (JSON Lines по stdio) без SDK. Шим опционален, не обязателен.
«Без подвязки к одному ЯП» доказывается **двумя** сырыми демо-скриптами на разных
языках (python и node), гоняемыми одним и тем же рантаймом без изменений в нём.

## 2. Раскладка модулей

```
cores/
├── regular/                 (v1 — не трогаем)
└── duplex/                  новое ядро
    └── core.manifest

runtime/
├── basic/                   (v1 — не трогаем)
└── duplex/                  новый Go-модуль wire-auto/runtime/duplex
    ├── runtime.manifest
    ├── go.mod
    ├── cmd/wire/            CLI: запустить скрипт по пути end-to-end
    └── internal/
        ├── manifest/        чтение трёх TOML-манифестов
        ├── discovery/       чтение script.manifest
        ├── registry/        admission: скан cores/*, допуск
        ├── handshake/       сведение контрактов
        ├── protocol/        сообщения JSON Lines + кодек (v2: +request/response)
        ├── capreg/          реестр capability-хендлеров (env.get)
        └── exec/            спавн, качание протокола, диспетчеризация request

scripts/examples/
├── env-report/              zero-shim демо, python, core = "duplex"
└── env-report-node/         zero-shim демо, node, core = "duplex"
```

**Переиспользование кода.** На ранних стадиях повторение допустимо (WET): `duplex`
несёт собственную копию логики в `internal/`, форкнутую из `basic` и расширенную под
v2. `basic` не трогается. Вынос общего в `packages/` — отдельная будущая задача, когда
станет готовность мигрировать и `basic`.

## 3. Манифесты

```toml
# cores/duplex/core.manifest
name = "duplex"
version = "0.1.0"
coreApi = 1
protocol = 2                 # новая версия wire: добавлены request/response
provides = ["env.get"]       # рабочий гейт авторизации, не декорация
links = ["stdio"]
```

```toml
# runtime/duplex/runtime.manifest
name = "wire-auto-runtime-duplex"
version = "0.1.0"
protocols = [1, 2]           # строгое надмножество basic: умеет и v1, и v2
transports = ["stdio"]
cores = ["duplex", "regular"] # информационно; допускает оба
```

```toml
# scripts/examples/env-report/script.manifest
name = "env-report"
version = "0.1.0"
core = "duplex"
coreApi = 1
capabilities = ["env.get"]
link = "stdio"
language = "python"
cmd = ["python", "main.py"]
```

Node-демо — тот же манифест, кроме `language = "node"` и `cmd = ["node", "main.js"]`.

## 4. Рантайм `duplex` — надмножество `basic`

Рантайм говорит `protocols = [1, 2]`, поэтому допускает **оба** ядра:

1. **Admission.** Скан `cores/*`. Допускается `regular` (protocol 1 ∈ [1,2], link stdio ✓)
   и `duplex` (protocol 2 ∈ [1,2] ✓). Оба в реестре допуска.
2. **Маршрутизация и рукопожатие — на ядро.** `core="regular"` сводится к protocol **1**;
   `core="duplex"` — к protocol **2**. Согласованная версия — «режим» цикла exec.
3. **Единый exec-loop с гейтом по версии.** Один цикл обслуживает оба режима; двусторонность
   включается тем, что рантайм послал в `hello` `protocol=2`.
4. **Обратная совместимость бесплатно.** v1-ядро `regular` со своим python-шимом едет на
   `duplex` без правок: `hello protocol=1` → классический односторонний поток.

Механика допуска/маршрутизации/рукопожатия переносится из `basic` как есть (admission
registry; сведение трёх паспортов; коды `UNKNOWN_CORE`/`CORE_INCOMPATIBLE`).

## 5. Протокол v2 — request/response

Транспорт прежний: **JSON Lines** (одно сообщение = одна строка компактного JSON + `\n`)
в обе стороны через stdin/stdout. К пяти типам v1 (`hello, ready, log, done, cancel`)
добавляются два. Новые поля — в ту же плоскую структуру `Message`, все `omitempty`,
поэтому v1-поток байт-в-байт не меняется.

**Новые поля `Message`:**
```go
ID         string          `json:"id,omitempty"`         // корреляция request↔response
Capability string          `json:"capability,omitempty"` // имя capability
Params     json.RawMessage `json:"params,omitempty"`     // вход capability
Code       string          `json:"code,omitempty"`       // код ошибки в response
// Result (уже есть в v1) переиспользуется как payload успешного response
```

**Сообщения:**

| Направление      | type       | Назначение                                            |
|------------------|------------|-------------------------------------------------------|
| runtime → script | `hello`    | старт: сведённые версии, аргументы                    |
| script → runtime | `ready`    | шим/скрипт поднялся                                    |
| script → runtime | `request`  | вызов capability: `id`, `capability`, `params`        |
| runtime → script | `response` | ответ: `id` + (`result` \| `code`+`message`)          |
| script → runtime | `log`      | строка вывода (`level`, `message`)                    |
| script → runtime | `done`     | завершение: `exitCode`, опционально `result`          |
| runtime → script | `cancel`   | просьба завершиться (таймаут/кнопка)                  |

**Примеры:**
```
runtime → {"type":"hello","protocol":2,"coreApi":1,"args":[]}
script  → {"type":"ready"}
script  → {"type":"request","id":"1","capability":"env.get","params":{"name":"USER"}}
runtime → {"type":"response","id":"1","result":{"value":"cyrille"}}
script  → {"type":"log","level":"info","message":"USER=cyrille"}
script  → {"type":"done","exitCode":0,"result":{"user":"cyrille"}}
```

**Правила:**
- `request` допустим **только после `ready`, до `done`, и только при protocol 2**.
  До `ready` или при protocol 1 → `PROTOCOL_VIOLATION` (kill).
- **Синхронная модель (v1-простота):** скрипт шлёт `request` и ждёт `response` перед
  следующим; рантайм обрабатывает по одному. `id` — формальность корреляции сейчас,
  но заложен, чтобы не ломать формат при будущей асинхронности.
- `response` — единственное сообщение хост→скрипт после `hello`, кроме `cancel`.
  Скрипт `response`/`hello` не шлёт (для него это violation).
- Ошибки capability возвращаются в `response.code` и **не убивают прогон** — в этом
  смысл двусторонности: скрипт сам решает, как реагировать.

## 6. Диспетчеризация capability (вариант A: обработчик в рантайме)

**Реестр** (`internal/capreg`):
```go
type Handler func(params json.RawMessage) (result json.RawMessage, code string, err error)

var registry = map[string]Handler{
    "env.get": envGet,
}
```

`env.get`: парсит `{"name": "..."}` → `os.LookupEnv`:
- нашлась → `result = {"value":"..."}`, `code = ""`;
- не нашлась → `code = "ENV_NOT_FOUND"`;
- битый `params` → `code = "BAD_PARAMS"`.

**Двойной гейт при `request`** (порядок важен):
```
request в exec-loop
  ├─ reconciled.Protocol < 2?            → PROTOCOL_VIOLATION           (kill)
  ├─ ready ещё не было?                  → PROTOCOL_VIOLATION           (kill)
  ├─ capability ∉ reconciled.Provides?   → response{code:CAPABILITY_DENIED}        (жив)
  ├─ нет handler в registry?             → response{code:CAPABILITY_UNIMPLEMENTED} (жив)
  └─ иначе → handler(params)             → response{result | code}                (жив)
```

`provides` из сведённого ядра — источник правды авторизации; registry лишь исполняет
уже разрешённое. Нарушение **протокола** убивает прогон; отказ/ошибка **capability**
возвращается в `response`, скрипт живёт.

## 7. Жизненный цикл (демо-прогон)

```
duplex run scripts/examples/env-report
 1. читает runtime.manifest (protocols=[1,2])
 2. admission: допускает regular(1) и duplex(2)
 3. читает script.manifest → core="duplex"
 4. handshake → reconciled{Protocol:2, CoreAPI:1, Provides:[env.get]}
 5. spawn cmd=[python, main.py], env: WIRE_SDK_DIR
 6. hello{protocol:2} → ready → request{env.get} → response{value}
    → log → done{exitCode:0, result}
 7. CLI печатает Status=OK, логи, result
```

Таймауты (`StartupTimeout`/`RunTimeout`/`CancelGrace`) и путь `cancel→grace→kill`
переносятся из `basic` без изменений. Обработка `request` синхронная и быстрая;
отдельного таймаута на handler в v2 нет (YAGNI — `env.get` мгновенна).

## 8. Обработка ошибок — коды

| Слой | Код | Kill? |
|---|---|---|
| Допуск | `UNKNOWN_CORE`, `CORE_INCOMPATIBLE` | — (до спавна) |
| Рукопожатие | `CORE_API_MISMATCH`, `CAPABILITY_DENIED`, `PROTOCOL_UNSUPPORTED`, `LINK_UNSUPPORTED` | — |
| Протокол | `PROTOCOL_VIOLATION` (request при v1 / до ready / неверный тип) | да |
| Прогон | `STARTUP_TIMEOUT`, `RUN_TIMEOUT`, `CRASHED`, `CANCELLED` | да |
| Capability (в `response.code`) | `CAPABILITY_DENIED`, `CAPABILITY_UNIMPLEMENTED`, `BAD_PARAMS`, `ENV_NOT_FOUND` | нет |

Итоговый Status прогона: `OK`, `SCRIPT_ERROR`, `HANDSHAKE_FAILED` (+код), `PROTOCOL_VIOLATION`,
`STARTUP_TIMEOUT`, `RUN_TIMEOUT`, `CRASHED`, `CANCELLED`.

## 9. Zero-shim демо-скрипты

Скрипт говорит протокол сырьём: читает строку `hello`, печатает `ready`/`request`/
`log`/`done`, читает `response`. Никакого импорта SDK.

`scripts/examples/env-report/main.py` (~20 строк): читает `hello`, шлёт `ready`,
шлёт `request{env.get, {name:"USER"}}`, читает `response`, логирует значение (или
`<denied:CODE>`), шлёт `done`.

`scripts/examples/env-report-node/main.js`: тот же диалог на чистом Node (`readline` +
`JSON`), без внешних пакетов. Доказывает «без подвязки к одному ЯП» на втором языке.

## 10. Тестирование (каждый модуль собирается/тестируется независимо)

- `protocol`: round-trip `request`/`response`; совместимость v1-полей.
- `capreg`: `env.get` — найдено / не найдено / битый params.
- `exec`: happy-path request→response; `CAPABILITY_DENIED` (вне provides);
  `CAPABILITY_UNIMPLEMENTED`; `request` при protocol 1 → violation; `request` до
  ready → violation; **классический v1-поток без request всё ещё работает** (регресс).
- `handshake`/`registry`: допуск обоих ядер; маршрутизация regular→v1, duplex→v2.
- E2E: `duplex` CLI гоняет `env-report/main.py` до `Status=OK`; отдельный E2E — старый
  `regular`-скрипт на новом рантайме (обратная совместимость вживую).

## 11. Границы v2 (что НЕ делаем)

- Нет реальных бриджей железа (serial/ssh/usb) — только `env.get` как доказательство канала.
- Обработчик capability живёт в рантайме (вариант A); отдельные процессы-бриджи (вариант B) —
  будущий этап, протокол под них уже готов.
- Асинхронные/параллельные `request` — модель синхронная; `id` заложен на будущее.
- Нет отдельного таймаута на handler; нет песочницы ОС — как в v1.
- Интеграция с `apps/deview` не входит в v2 — гоняем через `duplex` CLI. deview-мост — позже.
- `packages/`-вынос общего кода — будущая задача (сейчас WET-форк).

## 12. Критерии готовности v2

- `duplex` CLI гоняет `scripts/examples/env-report` → `request{env.get}` получает
  `response{value}`, лог с переменной, `Status=OK`.
- Node-демо `env-report-node` даёт тот же результат тем же рантаймом без его изменений.
- Скрипт просит `env.get` при `core.provides` без него → `response{CAPABILITY_DENIED}`,
  прогон продолжается (не kill).
- Старый `regular`-скрипт (protocol 1) на `duplex` → отрабатывает как v1 (`OK`).
- v1-скрипт, приславший `request`, → `PROTOCOL_VIOLATION` (kill).
- Каждый модуль: `go build ./... && go vet ./... && go test ./...` независимо.

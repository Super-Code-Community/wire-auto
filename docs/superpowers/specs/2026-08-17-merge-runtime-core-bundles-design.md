# Слияние runtime + core в бандлы (`cores/<name>`) — дизайн

Дата: 2026-08-17
Статус: черновик на согласование

## 1. Что и зачем

До сих пор платформа делила ответственность на **runtime** (Go-исполнитель,
`runtime/basic`, `runtime/duplex`) и **core** (паспорт + опциональный SDK,
`cores/regular`, `cores/duplex`). Между ними был отдельный handshake допуска
(admission): runtime сканировал `cores/*`, читал каждый `core.manifest` и решал
совместимость. Всего было три границы: app↔runtime, runtime↔core, runtime↔script.

**Пивот (решён 2026-08-17):** схлопнуть split runtime↔core. Runtime и core
становятся **одной сущностью — `core`**, поставляемой по модели «библиотека +
бандл»:

- **runtime = стандартная встраиваемая библиотека** (протокол, спавн скрипта,
  таймауты, диспетчеризация). Не варьируется; каждый бандл несёт её копию.
- **`core` = самодостаточный бинарь-бандл**, который эту библиотеку встраивает и
  регистрирует свои capability. Вся вариативность — здесь. Аналогия: `net/http`
  — библиотека; core — твой бинарь-сервер, который её импортирует и вешает
  обработчики. «Runtime» отдельным процессом никто не запускает.

**Границы сокращаются с 3 до 2.** Удаляется admission-handshake runtime↔core:
core и встроенный runtime — один трест-домен внутри одного бинаря, сверять
нечего. Остаются **app↔core** и **core↔script**.

**Что сохраняется:** скрипт по-прежнему спавнится **отдельным процессом**; провод
core↔script — тот же stdio JSON Lines. Песочница вокруг недоверенного *скрипта*,
не вокруг core. Именно поэтому слияние безопасно.

**Отправная точка:** v2-пара (`runtime/duplex` + `cores/duplex`) реализована в
рабочем дереве, все тесты зелёные, ничего не закоммичено. Эта спека
перерабатывает **обе** пары в merged-модель.

## 2. Ключевые решения

| # | Развилка | Решение |
|---|----------|---------|
| B | Общий runtime-код | Каждый бандл **вендорит свою копию** в `internal/` (WET осознанно). Единого shared-lib модуля нет. |
| A | Раскладка бандла | `cores/<name>/` = **один Go-модуль** = один бинарь `cmd/core`. Плоско. |
| A | Admission/registry | **Удаляется полностью.** Остаётся только handshake core↔script. |
| A | Источник правды по capability | **Код** (`capreg` registry). `provides` выводится как `keys(registry)`; поле `core.manifest.provides` убрано. |
| A′ | deview (app↔core) | Не перепиливаем мост. `cores/regular` вендорит serve-мост; deview меняет одну строку спавна на `./cores/regular/cmd/core`. Мультиcore-роутинг отложен. |
| — | Манифесты | `runtime.manifest` удаляется; `core.manifest` худеет; `script.manifest` без изменений. |

«Под любой ЯП» сохраняется: независимость языка — свойство *скрипта* (zero-shim,
сырой протокол по stdio), не зависит от того, откуда берётся `provides`. Правило
«provides = ключи зарегистрированных хендлеров» выразимо в runtime-lib любого
языка.

## 3. Целевая раскладка

```
cores/
  regular/                       ← слияние runtime/basic + нынешний cores/regular
    go.mod                       module wire-auto/cores/regular
    core.manifest                name, version, coreApi=1, protocol=1, links=["stdio"]
    cmd/core/main.go             СТРИМИНГОВЫЙ мост (serve) — бэкенд deview
    internal/
      bridge/ discovery/ exec/ handshake/ manifest/ protocol/   (вендор из basic)
    sdk/python/wire.py           (остаётся на месте)
  duplex/                        ← слияние runtime/duplex + нынешний cores/duplex
    go.mod                       module wire-auto/cores/duplex
    core.manifest                name, version, coreApi=1, protocol=2, links=["stdio"]
    cmd/core/main.go             SINGLE-SHOT прогонщик (как нынешний duplex/cmd/wire)
    internal/
      capreg/ exec/ handshake/ manifest/ protocol/              (вендор из duplex, БЕЗ registry)
```

**Удаляется целиком:** `runtime/basic/`, `runtime/duplex/`, оба
`runtime.manifest`, пакеты `registry` (в обоих бандлах), поле
`core.manifest.provides`, концепция списка `cores` в манифесте.

Каждый бандл собирается независимо: `cd cores/<name> && go build ./... &&
go vet ./... && go test ./...`. Root `go.work` не появляется (правило CLAUDE.md).

## 4. Бандл `regular` (protocol 1, бэкенд deview)

- `cmd/core` = дословный serve-мост из `basic/cmd/wire`: живёт до `exit`/EOF,
  обслуживает `list`/`run`/`cancel` → `catalog`/`ready`/`log`/`result`/`error`.
  Для deview поведение **байт-в-байт то же**.
- `runStreaming` больше **не сканирует чужие ядра**: бандл знает свой
  `core.manifest`. Скрипт с `core="regular"` → ок; с другим core → `UNKNOWN_CORE`.
- `list` (discovery.Scan) **фильтруется по своему ядру** — показывает только
  скрипты с `core="regular"`, чтобы deview не предлагал нерелевантное.
- Capability нет: provides-множество пустое; `script.capabilities` обязано быть
  пустым (иначе `CAPABILITY_DENIED`).
- **deview:** ровно одна правка — дефолтный спавн
  `go run ./runtime/basic/cmd/wire` → `go run ./cores/regular/cmd/core`
  (флаг `-wire` и `WIRE_BIN` продолжают работать). go.mod deview независим,
  bridge-протокол не меняется.

## 5. Бандл `duplex` (protocol 2, `env.get`)

- `cmd/core` = single-shot прогонщик из `duplex/cmd/wire`:
  discover-своего-ядра → handshake → spawn → pump → печать исхода.
- `capreg` — единственный источник правды по capability. Provides-гейт при
  `request` = `keys(capreg.Default)` (= `{"env.get"}`).
- `registry`/admission удалён; `handshake` сводит только `core.manifest` бандла +
  `script.manifest`.
- Zero-shim демо `scripts/examples/env-report` (python) и `env-report-node`
  (node) остаются рабочими.
- **Одностороннесть (protocol-1-style) доказывается на уровне exec-теста**
  `TestPlainV1FlowStillWorks` (`testdata/plain` — скрипт не шлёт `request`).
  Кросс-ядерный e2e «regular-на-duplex» **убирается**: без admission duplex-бинарь
  знает только своё ядро, скрипт с `core="regular"` даёт `UNKNOWN_CORE`. Regular-
  скрипты гоняет regular-бандл.

## 6. Handshake и манифесты

- `manifest`: `LoadRuntime` удаляется; из структуры `Core` уходит поле `Provides`.
  `protocols=[1,2]` — внутренняя константа вендоренного runtime-lib (бандл знает,
  что умеет), не поле манифеста.
- `handshake.Reconcile` меняет сигнатуру:
  `(rt, core, scr)` → `(core manifest.Core, scr manifest.Script, provides []string)`,
  где `provides` подаёт вызывающий: `keys(capreg.Default)` у duplex, `nil` у
  regular. Проверки прежние:
  - `script.core == core.name` → иначе `UNKNOWN_CORE`;
  - `script.coreApi == core.coreApi` → иначе `CORE_API_MISMATCH`;
  - каждый `script.capabilities` ∈ `provides` → иначе `CAPABILITY_DENIED`;
  - `script.link ∈ core.links` → иначе `LINK_UNSUPPORTED`;
  - `protocol` берётся из `core.manifest`.
- `script.manifest` не меняется (`name/version/core/coreApi/capabilities/link/
  language/cmd`).

## 7. Манифесты (итоговый вид)

```toml
# cores/regular/core.manifest
name = "regular"
version = "0.1.0"
coreApi = 1
protocol = 1
links = ["stdio"]
```

```toml
# cores/duplex/core.manifest
name = "duplex"
version = "0.1.0"
coreApi = 1
protocol = 2
links = ["stdio"]
```

`script.manifest` — без изменений (пример `env-report`: `core="duplex"`,
`capabilities=["env.get"]`, `link="stdio"`, `cmd=["python","main.py"]`).

## 8. Порядок миграции

По одному бандлу за раз, каждый доводим до зелёного перед следующим:

1. **`cores/regular`**: перенос кода из `runtime/basic`, роспуск
   `runtime.manifest` и `registry`, урезание `core.manifest`, фильтр `list` по
   своему ядру, правка сигнатуры handshake. Все тесты (bridge/exec/handshake/
   discovery/manifest/protocol + e2e) зелёные. Затем правка deview + ручной
   прогон deview против `cores/regular/cmd/core`.
2. **`cores/duplex`**: перенос из `runtime/duplex`, удаление `registry`,
   provides из `capreg`, правка handshake. Все пакеты + e2e (py/node) зелёные;
   exec-тест `TestPlainV1FlowStillWorks` остаётся (одностороннесть).
3. **Уборка**: удалить `runtime/`, обновить `guide/`-топики (cores/runtimes/
   manifests/handshake) под раскладку «нет отдельного runtime».

## 9. Критерии готовности

- Каждый бандл: `cd cores/<name> && go build ./... && go vet ./... &&
  go test ./...` независимо зелёный.
- deview руками отрабатывает `list` + прогон regular-скрипта против
  `cores/regular/cmd/core`.
- Каталога `runtime/` больше нет; `runtime.manifest` нигде нет.
- Root `go.work` не появился; `apps/deview` собирается независимо.

## 10. Вне объёма

- App↔core мультиcore-роутинг (deview, показывающий/гоняющий скрипты нескольких
  ядер) — отдельная сессия.
- Реальные бриджи железа (serial/ssh/usb) — только `env.get` как доказательство
  канала.
- Вынос общего кода в `packages/` — будущая задача (сейчас осознанный WET).
- Async/параллельные `request`, песочница ОС, отдельный таймаут на handler.
- Per-language runtime-lib (python/node core-авторам) — будущее; сейчас core на Go.

## 11. Замечания по дисциплине

- **Никаких git-операций** агентами: ни `commit`, ни `branch`, ни `add`, ни
  `push`. Пользователь коммитит сам (CLAUDE.md — hard rule).
- Каждый шаг завершается «его тесты зелёные», не «закоммичено».
- Все файловые операции — абсолютными Windows-путями.

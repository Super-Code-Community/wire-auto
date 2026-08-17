# Duplex core — стартовый пак сетевых и системных возможностей

- **Дата:** 2026-08-17
- **Ядро:** `cores/duplex`
- **Статус:** дизайн утверждён, ждёт плана

## Цель

Расширить реестр возможностей duplex-ядра (`capreg`) с единственной `env.get` до
нормального стартового пака провайдеров, чтобы скрипты могли выполнять сетевые
операции — в первую очередь **прозвон портов** — и снимать базовую информацию о
системе. Всё на чистом stdlib Go, кроссплатформенно, без внешних зависимостей и
без per-OS кода.

## Модель (контекст)

- Скрипт = оркестратор (логика), ядро = провайдер примитивов. Тяжёлая и
  привилегированная работа живёт в ядре на Go; скрипт лишь дёргает возможности
  через канал `request`/`response` (протокол 2).
- `capreg.Default` — карта `capability → Handler`. Ключи реестра автоматически
  становятся `provides` (`main.go: providesFromRegistry`), поэтому `core.manifest`
  менять не нужно. Скрипт объявляет нужные возможности в `script.capabilities`;
  они проходят гейт на рукопожатии и второй гейт в `exec.dispatchRequest`.
- Duplex-скрипты говорят на протоколе напрямую (без SDK), как `scripts/examples/env-report`.

## Объём

### Новые возможности (6, все stdlib)

**`net.*` — сеть**

| capability | params | result | реализация |
|---|---|---|---|
| `net.resolve` | `{host, timeout_ms?}` | `{addrs: [string]}` | `net.Resolver.LookupHost` с контекстом-таймаутом |
| `net.tcp.connect` | `{host, port, timeout_ms?}` | `{status, latency_ms}` | `net.DialTimeout("tcp", …)`; `status` ∈ `open\|closed\|filtered` |
| `net.tcp.banner` | `{host, port, timeout_ms?, read_bytes?}` | `{banner, bytes}` | dial + `SetReadDeadline` + чтение до `read_bytes` |
| `net.interfaces` | `{}` | `{interfaces: [{name, mac, addrs, flags}]}` | `net.Interfaces` + `Addrs` |

Классификация `net.tcp.connect`:
- соединение установлено → `open` (+ `latency_ms`);
- отказ (RST / connection refused) → `closed`;
- таймаут → `filtered`.

**`sys.*` — система**

| capability | params | result | реализация |
|---|---|---|---|
| `sys.info` | `{}` | `{os, arch, hostname, numCPU, goVersion}` | `runtime.GOOS/GOARCH/NumCPU/Version`, `os.Hostname` |
| `sys.env.list` | `{prefix?}` | `{names: [string]}` | `os.Environ`, только имена; при `prefix` — фильтр |

`env.get` остаётся как есть (значение одной переменной). `sys.env.list` отдаёт
только имена — значения по-прежнему через `env.get`.

### Политика таймаутов

- Общий разбор в `params.go`: `timeout_ms` с дефолтом **1000 мс** и потолком
  **10000 мс** (значения вне диапазона зажимаются, не ошибка).
- `read_bytes` для баннера: дефолт 256, потолок 4096.

### Явно вне объёма (YAGNI / будущая работа)

- `sys.disk`, `sys.mem`, детали CPU — требуют gopsutil или per-OS syscall; решено
  оставить только stdlib.
- UDP-проба, ICMP-пинг — ненадёжно / нужны привилегии.
- Конкурентный диспатч и батч-возможность `net.scan` — см. «Ограничение».

## Ограничение (честно фиксируем)

`dispatchRequest` вызывается **синхронно** в главном цикле `exec.Run`
(`exec.go:225`). Значит прозвон **последовательный**: медленный `connect`
(таймаут `filtered`) блокирует цикл до возврата. Для MVP это приемлемо — скрипт
управляет скоростью через `timeout_ms`. Конкурентный диспатч (горутина на запрос +
мьютекс на запись в stdin) — отдельная будущая работа, вне этого спека. Отмечается
в доке.

## Структура кода (ничего монолитного)

Реестр разбивается по категориям, один файл — одна ответственность:

```
cores/duplex/internal/capreg/
├── capreg.go      // тип Handler + сборка Default слиянием под-карт
├── env.go         // env.get (перенос существующего) + envCaps
├── net.go         // net.resolve, net.tcp.connect, net.tcp.banner, net.interfaces + netCaps
├── sys.go         // sys.info, sys.env.list + sysCaps
├── params.go      // разбор params, clampTimeout, общие хелперы
└── *_test.go      // тест на каждый файл
```

- `Default` собирается слиянием `envCaps`, `netCaps`, `sysCaps`. Добавить
  категорию = новый файл + одна строка слияния.
- Сигнатура `Handler` не меняется: `func(params json.RawMessage) (result json.RawMessage, code string, err error)`.
- Коды ошибок capability (в `response.code`, прогон не роняют): `BAD_PARAMS`
  (битые/неполные params), плюс доменные при необходимости (напр. `RESOLVE_FAILED`).
  Сетевые «отказ/таймаут» — это **не** ошибки capability, а нормальный `result`
  (`status: closed|filtered`).

## Демо-скрипт прозвона

`scripts/examples/port-scan/`:

- `script.manifest`:
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
- `main.py` (протокол напрямую, как `env-report`): `ready` → `net.resolve` хоста →
  цикл по списку портов (напр. 22, 80, 443, 8080, 8443) с `net.tcp.connect` →
  `log` по каждому открытому (порт + `latency_ms`) → `done` с
  `result = {"host":…, "open":[…]}`. Хост/порты — константы в скрипте (можно later
  из `args`).

## Тестирование

- Юнит на каждый хендлер:
  - `net.tcp.connect`: локальный `net.Listen("tcp","127.0.0.1:0")` → `open`;
    закрытый порт → `closed`; недостижимый адрес с малым таймаутом → `filtered`.
  - `net.tcp.banner`: листенер, пишущий известные байты → сверка `banner`.
  - `net.resolve`: `localhost` → непустой `addrs`.
  - `net.interfaces`: непустой список, у loopback валидные поля.
  - `sys.info`: `os/arch` совпадают с `runtime`, `numCPU>0`.
  - `sys.env.list`: выставить переменную в тесте → присутствует; `prefix` фильтрует.
  - `params`: `clampTimeout` зажимает границы; битые params → `BAD_PARAMS`.
- Независимая сборка модуля:
  `cd cores/duplex && GOWORK=off go build ./... && go vet ./... && go test ./...`.

## Документация

- Новый топик `guide/demo/capabilities.md`: что такое реестр возможностей,
  таблица всех возможностей (params/result), заметка про последовательный диспатч,
  как скрипт объявляет и вызывает capability.
- Ссылка на него из `guide/demo/README.md` (индекс).
- Короткое дополнение в `guide/demo/writing-a-script.md`: пример вызова capability
  из скрипта со ссылкой на `capabilities.md`.

## Не трогаем

- `core.manifest` (provides выводятся из реестра).
- Клиент `deview`.
- Протокол, handshake, exec (кроме возможного добавления возможностей — но и они
  идут только через `capreg`).

## Критерии готовности

1. 6 новых возможностей реализованы в `capreg`, реестр разбит по файлам.
2. Все юнит-тесты зелёные; модуль собирается/вётится/тестируется с `GOWORK=off`.
3. Демо-скрипт `port-scan` находится discovery и отрабатывает через `deview`,
   выдавая список открытых портов.
4. `guide/demo/capabilities.md` создан и слинкован из индекса.

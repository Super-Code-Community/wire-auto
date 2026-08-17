# Протокол моста (v1)

## Транспорт: JSON Lines

Один сеанс = один процесс скрипта. Коммуникация — через stdin/stdout процесса.

**Формат:** одно сообщение = одна строка компактного JSON + `\n` (без переносов внутри).
Направление работает в обе стороны по тем же трубам.

```
{"type":"hello","protocol":1,"coreApi":1,"args":[]}\n
{"type":"ready"}\n
{"type":"log","level":"info","message":"hello from python"}\n
{"type":"done","exitCode":0}\n
```

Формат человекочитаем, парсится в любом языке стандартными средствами.
В будущем тело можно заменить на бинарный формат (MessagePack) — рамка не изменится.

## Правило №2: stdout — это канал протокола

**Скрипт не должен делать `print()` / запись в stdout напрямую.**
stdout — исключительно для протокольных сообщений JSON Lines.
Хочешь вывести текст — шли сообщение `log`. Голый `print` сломает парсер рантайма.

stderr — не трогается протоколом; используется для сырой диагностики самого шима.

## Набор сообщений v1

| Направление      | `type`     | Назначение                                                  |
|------------------|------------|-------------------------------------------------------------|
| runtime → script | `hello`    | Старт сеанса: конфиг, сведённые версии, аргументы           |
| script → runtime | `ready`    | Шим поднялся, скрипт готов работать                         |
| script → runtime | `log`      | Строка вывода для клиента (поля: `level`, `message`)        |
| script → runtime | `done`     | Завершение: `exitCode`, опционально `result`                |
| runtime → script | `cancel`   | Просьба завершиться (по таймауту или запросу клиента)       |

## Описание сообщений

### hello (runtime → script)
```json
{"type":"hello","protocol":1,"coreApi":1,"args":[]}
```
- `protocol` — согласованная версия протокола
- `coreApi` — согласованная версия Core API
- `args` — аргументы запуска (передаются скрипту)

### ready (script → runtime)
```json
{"type":"ready"}
```
Шим успешно прочитал `hello` и готов. Должен прийти до истечения `startupTimeout` (10 с).

### log (script → runtime)
```json
{"type":"log","level":"info","message":"hello from python"}
```
- `level` — уровень (`info`, `warn`, `error`)
- `message` — текст строки

### done (script → runtime)
```json
{"type":"done","exitCode":0}
```
или с необязательным результатом:
```json
{"type":"done","exitCode":0,"result":{...}}
```
- `exitCode` — код завершения (`0` = успех, иное = ошибка скрипта)

### cancel (runtime → script)
```json
{"type":"cancel"}
```
Просьба скрипту завершиться. После `cancel` рантайм ждёт `CancelGrace` (2 с),
затем принудительно завершает процесс. Подробнее — в [lifecycle.md](lifecycle.md).

## Пример полного диалога

```
runtime → {"type":"hello","protocol":1,"coreApi":1,"args":[]}
script  → {"type":"ready"}
script  → {"type":"log","level":"info","message":"hello from python"}
script  → {"type":"done","exitCode":0}
```

## Зарезервированные типы: request / response

`request` и `response` зарезервированы для будущей возможности «запрос к железу»
(hardware bridge). В v1 **не используются**:
- `core.provides = []` — у ядра нет возможностей
- Любой `request` от скрипта при `provides = []` → ядро фиксирует **`PROTOCOL_VIOLATION`**

Подробнее о статусах результата — в [lifecycle.md](lifecycle.md).

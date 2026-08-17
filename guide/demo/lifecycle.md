# Жизненный цикл исполнения скрипта

## Полный цикл (фазы)

```
Discover → Route → Handshake → Spawn → Hello → Pump → Result
```

### 1. Discover — чтение манифестов

Бандл читает `script.manifest` из указанной папки скрипта и свой `core.manifest`.
Нет отдельного рантайм-манифеста — рантайм встроен в бандл как библиотека.

### 2. Route — маршрутизация по ядру

Скрипт маршрутизируется на ядро по полю `script.core`.
Провал: `UNKNOWN_CORE` или `CORE_INCOMPATIBLE`. Подробнее — в [cores.md](cores.md).

### 3. Handshake — сведение двух манифестов

Проверяется совместимость `coreApi`, `capabilities`, `link`.
Провал: `HANDSHAKE_FAILED` + конкретный код. Подробнее — в [handshake.md](handshake.md).

Если все проверки пройдены — формируется `reconciled` (согласованные версии), и
процесс переходит к спавну.

### 4. Spawn — запуск процесса

Бандл запускает `cmd` из `script.manifest`:
- Рабочая директория: папка скрипта
- stdin/stdout: труба протокола (JSON Lines)
- stderr: не перехватывается (сырая диагностика шима)
- Окружение: передаётся `WIRE_SDK_DIR=<coreDir>/sdk`

### 5. Hello — установка сеанса

Бандл отправляет `hello` и ждёт `ready` от скрипта.

```
core → {"type":"hello","protocol":1,"coreApi":1,"args":[]}
```

Ожидание ограничено `startupTimeout` = **10 секунд**.
Если `ready` не пришёл за это время → kill + статус **`STARTUP_TIMEOUT`**.

### 6. Pump — прокачка сообщений

Бандл читает поток сообщений от скрипта:
- `log` → передаётся наружу клиенту
- `done` → завершение сеанса
- `request` → в v1 фиксируется как **`PROTOCOL_VIOLATION`**
- Неизвестный `type` → **`PROTOCOL_VIOLATION`**
- Битый JSON → **`PROTOCOL_VIOLATION`**

Ожидание `done` ограничено `runTimeout` = **60 секунд** (считается с момента `ready`).

### 7. Result — итог

Бандл возвращает единый результат клиенту (JSON). Процесс завершён.

## Таймауты и остановка

| Таймаут        | Значение | Триггер                              | Действие              |
|----------------|----------|--------------------------------------|-----------------------|
| startupTimeout | 10 с     | `ready` не получен после `hello`     | kill → STARTUP_TIMEOUT |
| runTimeout     | 60 с     | `done` не получен после `ready`      | cancel + grace → kill → RUN_TIMEOUT |
| cancelGrace    | 2 с      | после отправки `cancel`              | ждать, затем kill     |

### Последовательность при таймауте runTimeout

```
1. Бандл отправляет {"type":"cancel"}
2. Ждёт CancelGrace (2 с) — даёт скрипту завершиться чисто
3. Если скрипт не завершился — принудительный kill
4. Результат: RUN_TIMEOUT
```

Та же последовательность при ручной отмене от клиента: `cancel` → grace → kill.

## Статусы результата

| Статус               | Условие                                                  |
|----------------------|----------------------------------------------------------|
| `OK`                 | `done` с `exitCode = 0`                                  |
| `SCRIPT_ERROR`       | `done` с `exitCode ≠ 0` (ошибка внутри скрипта)         |
| `HANDSHAKE_FAILED`   | Сведение манифестов не прошло (+ `ErrorCode`)            |
| `PROTOCOL_VIOLATION` | Битый JSON, неизвестный тип, `request` без разрешения    |
| `STARTUP_TIMEOUT`    | `ready` не получен за 10 с                               |
| `RUN_TIMEOUT`        | `done` не получен за 60 с                                |
| `CRASHED`            | Процесс завершился (EOF) без `done`                      |

## Формат результата (JSON)

Бандл печатает результат в stdout в формате JSON и завершается с кодом `0` при
`Status = OK`, иначе — с кодом `1`.

```json
{
  "Status": "OK",
  "Logs": [
    {"level": "info", "message": "hello from python"}
  ]
}
```

Пример при ошибке:
```json
{
  "Status": "HANDSHAKE_FAILED",
  "ErrorCode": "CORE_API_MISMATCH",
  "ErrorMessage": "script coreApi 2, core coreApi 1",
  "Logs": []
}
```

Подробнее о командах запуска и интерпретации результата — в [running.md](running.md).

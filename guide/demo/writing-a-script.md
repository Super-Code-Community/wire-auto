# Написание нового скрипта

## Пошаговая инструкция

### Шаг 1. Создать папку скрипта

```
scripts/
└── examples/
    └── my-script/       ← создаём здесь
```

Имя папки произвольное. Папка изолирована — скрипт обращается к ядру только через
SDK/протокол, не через внутренности `cores/`.

### Шаг 2. Написать script.manifest

Создайте файл `scripts/examples/my-script/script.manifest`:

```toml
name = "my-script"
version = "0.1.0"
core = "regular"
coreApi = 1
link = "stdio"
cmd = ["python", "main.py"]
capabilities = []
```

Необязательное поле `language = "python"` можно добавить как подсказку для клиента,
но оно не влияет на работу ядра.

Объяснение полей — в [manifests.md](manifests.md).

### Шаг 3. Написать main.py с шимом

Создайте файл `scripts/examples/my-script/main.py`:

```python
import os
import sys

sys.path.insert(0, os.path.join(os.environ["WIRE_SDK_DIR"], "python"))
from wire import Script

def main():
    s = Script()
    s.start()                        # читает hello, отвечает ready
    s.log("привет из моего скрипта") # лог для клиента
    # ... логика скрипта ...
    s.done(0)                        # завершение с exitCode=0

if __name__ == "__main__":
    main()
```

**Важно:** не используй `print()` для вывода — это сломает парсер протокола.
Весь вывод — через `s.log()`. Подробнее — в [protocol.md](protocol.md).

### Шаг 4. Запустить

Скрипт запускается через клиент — консольный `deview`:

```bash
go run ./apps/deview/cmd/deview
```

Обнаружение скриптов рекурсивно: скрипт, положенный под
`scripts/examples/<имя>/` или `scripts/community/<имя>/`, автоматически появится
в нумерованном меню. Выберите его номер — deview покажет живой лог и итог.
Подробнее — в [apps-deview.md](apps-deview.md) и [running.md](running.md).

При успехе итоговое событие будет примерно таким:
```json
{"type":"result","status":"OK","exitCode":0}
```

## Типичные ошибки и их причины

### CORE_API_MISMATCH

```json
{"Status": "HANDSHAKE_FAILED", "ErrorCode": "CORE_API_MISMATCH", ...}
```

Причина: `script.coreApi` не совпадает с `core.coreApi` (сейчас `1`).
Скрипт **не запускается** — бандл отказывает до спавна.

Решение: проверить `coreApi = 1` в `script.manifest`.

### CAPABILITY_DENIED

```json
{"Status": "HANDSHAKE_FAILED", "ErrorCode": "CAPABILITY_DENIED", ...}
```

Причина: в `script.capabilities` указана возможность, не разрешённая реестром бандла.
В v1 у ядра нет разрешённых возможностей, поэтому в скрипте тоже должно быть `capabilities = []`.

Решение: убрать всё из `capabilities` в `script.manifest`.

### UNKNOWN_CORE

```json
{"Status": "HANDSHAKE_FAILED", "ErrorCode": "UNKNOWN_CORE", ...}
```

Причина: в `script.core` указано имя, которого нет среди известных ядер.
Скрипт не запускается.

Решение: проверить `core = "regular"` в `script.manifest`.

### STARTUP_TIMEOUT

Скрипт запустился, но не прислал `ready` за 10 секунд.

Вероятные причины:
- Ошибка импорта шима (опечатка в пути к `WIRE_SDK_DIR`)
- Исключение в коде до `s.start()`
- Прямой `print()` до `s.start()` засорил stdout

Диагностика: посмотреть stderr скрипта — шим пишет туда сырую диагностику.

### PROTOCOL_VIOLATION

Скрипт отправил битый JSON или неизвестный тип сообщения.
Чаще всего — случайный `print()` попал в stdout.

## Правила написания скриптов

1. **Никакого `print()`** — только `s.log()`.
2. **`s.start()` первым** — до любой логики.
3. **`s.done(exitCode)` последним** — всегда явно завершать, даже при ошибке.
4. **`capabilities = []`** — в v1 это обязательно пустой список.
5. **Ошибки скрипта** — `s.done(1)`, не `raise Exception`.

Пример скрипта с обработкой ошибки:
```python
s = Script()
s.start()
try:
    # логика
    s.log("всё хорошо")
    s.done(0)
except Exception as e:
    s.log(f"ошибка: {e}", level="error")
    s.done(1)
```

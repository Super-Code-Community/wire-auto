# Запуск и тестирование

Все команды запускаются из корня репозитория `wire-auto/` (если не оговорено иное).

## Запуск скрипта

```bash
go run ./runtime/basic/cmd/wire ./scripts/examples/hello
```

CLI принимает необязательные флаги и один позиционный аргумент — путь к папке скрипта:

```
wire [--runtime <путь>] [--cores <путь>] <script-dir>
```

| Флаг        | По умолчанию                        | Назначение                             |
|-------------|-------------------------------------|----------------------------------------|
| `--runtime` | `runtime/basic/runtime.manifest`    | путь к манифесту рантайма              |
| `--cores`   | `cores`                             | путь к директории с ядрами             |

Примеры:
```bash
# запуск с явными путями (эквивалентно умолчаниям)
go run ./runtime/basic/cmd/wire \
    --runtime runtime/basic/runtime.manifest \
    --cores cores \
    ./scripts/examples/hello

# запуск своего скрипта
go run ./runtime/basic/cmd/wire ./scripts/examples/my-script
```

## Результат и код выхода

Рантайм печатает JSON в stdout и завершается:
- код выхода `0` — `Status` = `OK`
- код выхода `1` — любой другой статус

Пример успешного результата:
```json
{
  "Status": "OK",
  "Logs": [
    {"level": "info", "message": "hello from python"}
  ]
}
```

Пример при ошибке рукопожатия:
```json
{
  "Status": "HANDSHAKE_FAILED",
  "ErrorCode": "CORE_API_MISMATCH",
  "ErrorMessage": "script coreApi 2, core coreApi 1",
  "Logs": []
}
```

Полный список статусов — в [lifecycle.md](lifecycle.md).

## Go-тесты рантайма

```bash
# из корня репозитория (через go.work)
go test wire-auto/runtime/basic/...

# или из папки модуля
cd runtime/basic && go test ./...
```

## Сборка и проверка (vet)

```bash
go build wire-auto/runtime/basic/...
go vet   wire-auto/runtime/basic/...
```

## Тест Python-шима

```bash
python cores/regular/sdk/python/wire_test.py
```

> Используется `python`, не `python3` — в среде проекта `python3` не установлен.

## Диагностика сбоев

Если скрипт не запускается или падает:

1. **Посмотреть `ErrorCode`** в JSON-результате — все коды объяснены в [handshake.md](handshake.md)
   и [lifecycle.md](lifecycle.md).
2. **`STARTUP_TIMEOUT` / `CRASHED`**: проверить stderr скрипта — шим пишет туда
   сырую диагностику (импорт не нашёл файл, синтаксическая ошибка и т.п.).
3. **`PROTOCOL_VIOLATION`**: скорее всего в stdout скрипта попал голый `print()`.
   Весь вывод — только через `s.log()`.

Руководство по написанию скриптов и типичным ошибкам — в [writing-a-script.md](writing-a-script.md).

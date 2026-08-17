# Запуск и тестирование

Все команды запускаются из корня репозитория `wire-auto/` (если не оговорено иное).

## Запуск скрипта через deview

`core` (regular) — долгоживущий мост (stdin → команды, stdout → события); напрямую руками
его не запускают. Для интерактивного запуска скриптов используйте `deview`:

```bash
go run ./apps/deview/cmd/deview
```

`deview` сам поднимает бандл ядра, показывает нумерованное меню и рисует живой ход
выполнения. Подробнее — в [apps-deview.md](apps-deview.md).

### Запуск через пайп (отладка / скрипты)

Можно передавать команды напрямую, подавая JSON Lines в stdin:

```bash
printf '%s\n' '{"type":"list"}' '{"type":"exit"}' | go run ./cores/regular/cmd/core
```

Бандл читает команды до `exit` или EOF и закрывается. Это удобно для автоматизации
и диагностики; для повседневной работы предпочтительнее `deview`.

### Флаги core (regular)

| Флаг        | По умолчанию | Назначение                       |
|-------------|--------------|----------------------------------|
| `--cores`   | `cores`      | путь к директории с ядрами       |
| `--scripts` | `scripts`    | корень поиска скриптов           |

Протокол команд/событий — в [app-core-bridge.md](app-core-bridge.md).

## Go-тесты бандлов

```bash
# из папки модуля:
cd cores/regular && go test ./...
cd cores/duplex  && go test ./...
```

## Сборка и проверка (vet)

```bash
cd cores/regular && go build ./... && go vet ./...
cd cores/duplex  && go build ./... && go vet ./...
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

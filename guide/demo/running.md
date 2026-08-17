# Запуск и тестирование

Все команды запускаются из корня репозитория `wire-auto/` (если не оговорено иное).

## Запуск скрипта через deview

`wire` — долгоживущий мост (stdin → команды, stdout → события); напрямую руками
его не запускают. Для интерактивного запуска скриптов используйте `deview`:

```bash
go run ./apps/deview/cmd/deview
```

`deview` сам поднимает мост, показывает нумерованное меню и рисует живой ход
выполнения. Подробнее — в [apps-deview.md](apps-deview.md).

### Запуск через пайп (отладка / скрипты)

Можно передавать команды напрямую, подавая JSON Lines в stdin:

```bash
printf '%s\n' '{"type":"list"}' '{"type":"exit"}' | go run ./runtime/basic/cmd/wire
```

Мост читает команды до `exit` или EOF и закрывается. Это удобно для автоматизации
и диагностики; для повседневной работы предпочтительнее `deview`.

### Флаги wire

| Флаг        | По умолчанию                     | Назначение                       |
|-------------|----------------------------------|----------------------------------|
| `--runtime` | `runtime/basic/runtime.manifest` | путь к манифесту рантайма        |
| `--cores`   | `cores`                          | путь к директории с ядрами       |
| `--scripts` | `scripts`                        | корень поиска скриптов           |

Протокол команд/событий — в [app-runtime-bridge.md](app-runtime-bridge.md).

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

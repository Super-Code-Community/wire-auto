# Топология репозитория и модулей

## Корень репозитория — не Go-модуль

В корне `wire-auto/` **нет `go.mod`**. Корень — это только рабочее пространство
(workspace) для разработки.

## go.work — локальный dev-инструмент, НЕ коммитится

`go.work` — **строго локальный, помашинный** инструмент разработчика: он **не хранится
в репозитории** (в `.gitignore`), не является артефактом деплоя и **никогда не
коммитится**. Репозиторий обязан собираться без него — каждый модуль независим.

Если хочется, чтобы `go build/test ./...` работали из корня сразу по всем модулям,
создайте `go.work` у себя локально:

```bash
go work init ./cores/regular ./cores/duplex ./apps/deview
```

Он получится примерно таким (но останется только у вас, вне git):

```
// go.work  (локально, не в репозитории)
go 1.26.4

use ./cores/regular
use ./cores/duplex
use ./apps/deview
```

Без `go.work` каждый модуль собирается сам по себе — заходите в его папку
(`cd cores/regular`, `cd apps/deview`) и работаете там. См. «Команды сборки» ниже.

## cores/ — контейнер ядер-бандлов

Каждое ядро — самодостаточный Go-модуль в `cores/<имя>/`:

```
cores/
├── regular/          module: wire-auto/cores/regular
│   ├── go.mod
│   ├── core.manifest
│   ├── cmd/core/     точка входа — стриминговый мост app↔core
│   └── internal/     встроенная рантайм-библиотека
└── duplex/           module: wire-auto/cores/duplex
    ├── go.mod
    ├── core.manifest
    ├── cmd/core/     точка входа — одиночный запуск (-script <dir>)
    └── internal/     встроенная рантайм-библиотека
```

Каждый бандл — самодостаточный Go-модуль со своей точкой входа `cmd/core`.
Рантайм встроен в бандл как библиотека (`internal/`), а не запускается отдельным процессом.
Будущие ядра (например, `cores/cloud`) добавятся рядом как самостоятельные модули.

## scripts/ — сами скрипты

```
scripts/
└── examples/
    └── hello/
        ├── script.manifest
        └── main.py
```

Скрипты изолированы — обращаются к ядру только через публичный SDK/протокол,
не через внутренности `cores/`.

## Команды сборки и тестирования

Все команды запускаются из папки нужного модуля (если не оговорено иное).

**Быстрый smoke моста** (список скриптов и выход):
```bash
printf '%s\n' '{"type":"list"}' '{"type":"exit"}' | go run ./cores/regular/cmd/core
```

`core` (regular) — долгоживущий мост, а не разовая команда; для интерактивного запуска
скриптов используйте клиент `deview` (`go run ./apps/deview/cmd/deview`).
Подробнее — в [running.md](running.md) и [apps-deview.md](apps-deview.md).

**Go-тесты бандла regular:**
```bash
# из папки модуля:
cd cores/regular && go test ./...
```

**Go-тесты бандла duplex:**
```bash
cd cores/duplex && go test ./...
```

**Сборка и проверка:**
```bash
cd cores/regular && go build ./... && go vet ./...
cd cores/duplex  && go build ./... && go vet ./...
```

**Юнит-тест Python-шима:**
```bash
python cores/regular/sdk/python/wire_test.py
```

> Команда `python3` в среде проекта **не используется** — только `python`.

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
go work init ./runtime/basic ./apps/deview
```

Он получится примерно таким (но останется только у вас, вне git):

```
// go.work  (локально, не в репозитории)
go 1.26.4

use ./runtime/basic
use ./apps/deview
```

Без `go.work` каждый модуль собирается сам по себе — заходите в его папку
(`cd runtime/basic`, `cd apps/deview`) и работаете там. См. «Команды сборки» ниже.

## runtime/ — контейнер рантайм-моделей

Каждая рантайм-модель — отдельный Go-модуль в `runtime/<модель>/`:

```
runtime/
└── basic/            module: wire-auto/runtime/basic
    ├── go.mod
    ├── cmd/wire/     точка входа — команда wire
    └── internal/     manifest, handshake, registry, exec, protocol
```

Сейчас одна модель — `runtime/basic`. Будущие модели (например, `runtime/cloud`)
появятся рядом как самостоятельные модули.

## cores/ — контейнер ядер

Каждое ядро — папка `cores/<ядро>/` с `core.manifest` и SDK-шимами под языки:

```
cores/
└── regular/
    ├── core.manifest       паспорт ядра (TOML)
    └── sdk/
        └── python/
            ├── wire.py        тонкий шим (~30 строк)
            └── wire_test.py   юнит-тест шима
```

Ядро **не является** Go-модулем — это просто папка с манифестом и языковыми шимами.

## Команды сборки и тестирования

Все команды запускаются из корня репозитория `wire-auto/` (если не оговорено иное).

**Быстрый smoke моста** (список скриптов и выход):
```bash
printf '%s\n' '{"type":"list"}' '{"type":"exit"}' | go run ./runtime/basic/cmd/wire
```

`wire` — долгоживущий мост, а не разовая команда; для интерактивного запуска
скриптов используйте клиент `deview` (`go run ./apps/deview/cmd/deview`).
Подробнее — в [running.md](running.md) и [apps-deview.md](apps-deview.md).

**Go-тесты рантайма:**
```bash
go test wire-auto/runtime/basic/...
# или эквивалентно из папки модуля:
cd runtime/basic && go test ./...
```

**Сборка и проверка:**
```bash
go build wire-auto/runtime/basic/...
go vet   wire-auto/runtime/basic/...
```

**Юнит-тест Python-шима:**
```bash
python cores/regular/sdk/python/wire_test.py
```

> Команда `python3` в среде проекта **не используется** — только `python`.

## scripts/ — сами скрипты

```
scripts/
└── examples/
    └── hello/
        ├── script.manifest
        └── main.py
```

Скрипты изолированы — обращаются к ядру только через публичный SDK/протокол,
не через внутренности `cores/` или `runtime/`.

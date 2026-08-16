# Топология репозитория и модулей

## Корень репозитория — не Go-модуль

В корне `wire-auto/` **нет `go.mod`**. Корень — это только рабочее пространство
(workspace) для разработки.

## go.work — dev-time workspace pointer

Файл `go.work` в корне подключает все Go-модули репозитория для удобной разработки
(позволяет `go build/test/run` из корня без замены путей):

```
// go.work
go 1.26.4

use ./runtime/basic
```

`go.work` — инструмент разработчика, не артефакт деплоя. Каждый модуль внутри остаётся
независимо собираемым. Новые Go-модули добавляются сюда директивой `use`.

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

**Запустить пример:**
```bash
go run ./runtime/basic/cmd/wire ./scripts/examples/hello
```

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

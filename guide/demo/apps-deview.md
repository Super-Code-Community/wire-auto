# Консольный клиент deview

`deview` — первый клиент платформы wire-auto. Это интерактивный консольный браузер
скриптов: поднимает бандл ядра `cores/regular/cmd/core`, показывает нумерованное меню
доступных скриптов, запускает выбранный и рисует живой ход выполнения.

## Запуск

```bash
go run ./apps/deview/cmd/deview
```

При запуске без флагов deview сам поднимает бандл через `go run ./cores/regular/cmd/core`
(dev-умолчание). Для prod-окружения укажите готовый бинарник:

| Способ | Пример |
|--------|--------|
| Флаг `--wire` | `go run ./apps/deview/cmd/deview --wire /usr/local/bin/core` |
| Переменная `WIRE_BIN` | `WIRE_BIN=/usr/local/bin/core go run ./apps/deview/cmd/deview` |
| Dev-умолчание | `go run ./apps/deview/cmd/deview` (запускает `go run ./cores/regular/cmd/core`) |

## Раскладка модуля

```
apps/deview/
├── cmd/deview/
│   └── main.go          — REPL-цикл: меню → запуск → результат → меню
├── internal/bridge/
│   ├── message.go       — типы Command/Event/Script, JSON-кодирование
│   ├── transport.go     — ProcessTransport: поднимает core как подпроцесс
│   ├── client.go        — Client: List / Run / Cancel / Close
│   └── client_test.go   — unit-тесты клиента на фейковом транспорте
└── internal/ui/
    ├── menu.go          — RenderMenu: нумерованный список скриптов
    ├── render.go        — RenderLog / RenderResult: форматирование вывода
    └── render_test.go   — unit-тесты рендеринга
```

**`internal/bridge`** — транспортный слой. Знает только протокол app↔core:
кодирует команды в JSON Lines, декодирует события из JSON Lines.
Подробнее о самом протоколе — в [app-core-bridge.md](app-core-bridge.md).

**`internal/ui`** — слой отображения. Превращает события в строки для терминала;
не зависит от транспорта, легко тестируется.

## Поток работы

```
deview стартует
  └── поднимает бандл core как подпроцесс
  └── шлёт {"type":"list"} → получает catalog
  └── печатает меню:
        1. hello   (python)
        2. deploy  (bash)
        q. выход

пользователь вводит "1"
  └── deview шлёт {"type":"run","dir":"scripts/examples/hello"}
  └── бандл отвечает: ready → log* → result
  └── deview рисует живой лог и итог
  └── возвращается в меню

пользователь вводит "q"
  └── deview закрывает бандл ({"type":"exit"}) и выходит
```

### Ввод пользователя

| Ввод | Действие |
|------|----------|
| Число (например `1`) | Запустить соответствующий скрипт |
| `q` / `quit` / `exit` | Завершить deview |
| Ctrl-C **во время прогона** | Отправить `cancel` бандлу (deview не завершается) |
| Ctrl-C **в меню** | Завершить deview |

### Обнаружение скриптов

Бандл сканирует каталог скриптов рекурсивно — ищет файл `script.manifest` на любой
глубине (например `scripts/examples/hello/`). Корень поиска задаётся флагом
`--scripts` при запуске `core` (по умолчанию `scripts`).

## Go-команды

```bash
# тесты
cd apps/deview && go test ./...

# сборка
cd apps/deview && go build ./...

# vet
cd apps/deview && go vet ./...
```

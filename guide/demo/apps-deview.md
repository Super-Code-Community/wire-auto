# Консольный клиент deview

`deview` — первый клиент платформы wire-auto. Это интерактивный консольный браузер
скриптов: поднимает мост `wire`, показывает нумерованное меню доступных скриптов,
запускает выбранный и рисует живой ход выполнения.

## Запуск

```bash
go run ./apps/deview/cmd/deview
```

При запуске без флагов deview сам поднимает мост через `go run ./runtime/basic/cmd/wire`
(dev-умолчание). Для prod-окружения укажите готовый бинарник:

| Способ | Пример |
|--------|--------|
| Флаг `--wire` | `go run ./apps/deview/cmd/deview --wire /usr/local/bin/wire` |
| Переменная `WIRE_BIN` | `WIRE_BIN=/usr/local/bin/wire go run ./apps/deview/cmd/deview` |
| Dev-умолчание | `go run ./apps/deview/cmd/deview` (запускает `go run ./runtime/basic/cmd/wire`) |

## Раскладка модуля

```
apps/deview/
├── cmd/deview/
│   └── main.go          — REPL-цикл: меню → запуск → результат → меню
├── internal/bridge/
│   ├── message.go       — типы Command/Event/Script, JSON-кодирование
│   ├── transport.go     — ProcessTransport: поднимает wire как подпроцесс
│   ├── client.go        — Client: List / Run / Cancel / Close
│   └── client_test.go   — unit-тесты клиента на фейковом транспорте
└── internal/ui/
    ├── menu.go          — RenderMenu: нумерованный список скриптов
    ├── render.go        — RenderLog / RenderResult: форматирование вывода
    └── render_test.go   — unit-тесты рендеринга
```

**`internal/bridge`** — транспортный слой. Знает только протокол app↔runtime:
кодирует команды в JSON Lines, декодирует события из JSON Lines.
Подробнее о самом протоколе — в [app-runtime-bridge.md](app-runtime-bridge.md).

**`internal/ui`** — слой отображения. Превращает события в строки для терминала;
не зависит от транспорта, легко тестируется.

## Поток работы

```
deview стартует
  └── поднимает мост (wire как подпроцесс)
  └── шлёт {"type":"list"} → получает catalog
  └── печатает меню:
        1. hello   (python)
        2. deploy  (bash)
        q. выход

пользователь вводит "1"
  └── deview шлёт {"type":"run","dir":"scripts/examples/hello"}
  └── мост отвечает: ready → log* → result
  └── deview рисует живой лог и итог
  └── возвращается в меню

пользователь вводит "q"
  └── deview закрывает мост ({"type":"exit"}) и выходит
```

### Ввод пользователя

| Ввод | Действие |
|------|----------|
| Число (например `1`) | Запустить соответствующий скрипт |
| `q` / `quit` / `exit` | Завершить deview |
| Ctrl-C **во время прогона** | Отправить `cancel` мосту (deview не завершается) |
| Ctrl-C **в меню** | Завершить deview |

### Обнаружение скриптов

Мост сканирует каталог скриптов рекурсивно — ищет файл `script.manifest` на любой
глубине (например `scripts/examples/hello/`). Корень поиска задаётся флагом
`--scripts` при запуске `wire` (по умолчанию `scripts`).

## Go-команды

```bash
# тесты
cd apps/deview && go test ./...

# сборка
cd apps/deview && go build ./...

# vet
cd apps/deview && go vet ./...
```

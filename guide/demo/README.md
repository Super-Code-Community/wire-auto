# wire-auto — документация

Платформа для запуска скриптов автоматизации на разных языках через единую шину:
три манифеста → рукопожатие → спавн процесса → JSON Lines протокол → результат.

## Содержание

| Файл | Что внутри |
|------|------------|
| [01-overview.md](01-overview.md) | Что такое wire-auto, принцип «ничего монолитного», две пары стыковки, зоны репозитория, общий поток данных |
| [02-repo-and-modules.md](02-repo-and-modules.md) | Топология: корень без go.mod, go.work, runtime/basic как Go-модуль, cores/regular; команды сборки и тестирования |
| [manifests.md](manifests.md) | Три TOML-паспорта: runtime.manifest, core.manifest, script.manifest; все поля с описанием |
| [handshake.md](handshake.md) | Сведение трёх манифестов: порядок проверок, все коды ошибок, «причина заранее» для клиента |
| [protocol.md](protocol.md) | JSON Lines, набор сообщений, диалог hello→ready→log→done, правило «stdout — канал протокола», зарезервированные request/response |
| [transports-and-language-independence.md](transports-and-language-independence.md) | Концепт «связь» (link), почему ядро языко-независимо, WIRE_SDK_DIR, тонкий шим |
| [cores-and-runtimes.md](cores-and-runtimes.md) | Контейнеры моделей; полиморфная стыковка по контракту; реестр допуска; UNKNOWN_CORE vs CORE_INCOMPATIBLE; «пересаживание» |
| [lifecycle.md](lifecycle.md) | Полный цикл исполнения (discover→route→handshake→spawn→hello→pump→result), таймауты, cancel+grace, статусы результата |
| [writing-a-script.md](writing-a-script.md) | Пошаговое создание скрипта: папка, script.manifest, main.py с шимом, запуск; типичные ошибки |
| [running.md](running.md) | Как запускать и тестировать: deview, пайп JSON-команд, флаги wire, команды test/build/vet |
| [app-runtime-bridge.md](app-runtime-bridge.md) | Верхняя граница app↔runtime: команды list/run/cancel/exit, события catalog/ready/log/result/error, двусторонний мост |
| [apps-deview.md](apps-deview.md) | Консольный клиент deview: браузер скриптов + живой рендер; запуск, флаги, поток работы |

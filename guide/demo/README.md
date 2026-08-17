# wire-auto — документация

Платформа для запуска скриптов автоматизации на разных языках через единую шину:
два манифеста → рукопожатие → спавн процесса → JSON Lines протокол → результат.

## Содержание

| Файл | Что внутри |
|------|------------|
| [01-overview.md](01-overview.md) | Что такое wire-auto, принцип «ничего монолитного», две границы стыковки, зоны репозитория, общий поток данных |
| [02-repo-and-modules.md](02-repo-and-modules.md) | Топология: корень без go.mod, go.work, cores/regular и cores/duplex как Go-модули; команды сборки и тестирования |
| [manifests.md](manifests.md) | Два TOML-паспорта: core.manifest, script.manifest; все поля с описанием |
| [handshake.md](handshake.md) | Сведение двух манифестов: порядок проверок, все коды ошибок, «причина заранее» для клиента |
| [protocol.md](protocol.md) | JSON Lines, набор сообщений, диалог hello→ready→log→done, правило «stdout — канал протокола», зарезервированные request/response |
| [transports-and-language-independence.md](transports-and-language-independence.md) | Концепт «связь» (link), почему ядро языко-независимо, WIRE_SDK_DIR, тонкий шим |
| [cores.md](cores.md) | Ядра-бандлы: self-contained Go-модули; два рукопожатия; реестр возможностей; UNKNOWN_CORE vs CORE_INCOMPATIBLE; «пересаживание» |
| [lifecycle.md](lifecycle.md) | Полный цикл исполнения (discover→route→handshake→spawn→hello→pump→result), таймауты, cancel+grace, статусы результата |
| [writing-a-script.md](writing-a-script.md) | Пошаговое создание скрипта: папка, script.manifest, main.py с шимом, запуск; типичные ошибки |
| [running.md](running.md) | Как запускать и тестировать: deview, пайп JSON-команд, флаги core, команды test/build/vet |
| [app-core-bridge.md](app-core-bridge.md) | Граница app↔core: команды list/run/cancel/exit, события catalog/ready/log/result/error, двусторонний мост |
| [apps-deview.md](apps-deview.md) | Консольный клиент deview: браузер скриптов + живой рендер; запуск, флаги, поток работы |

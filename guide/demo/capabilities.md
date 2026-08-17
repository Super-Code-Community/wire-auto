# Возможности ядра (capabilities)

Ядро — провайдер примитивов, скрипт — оркестратор. Скрипт объявляет нужные
возможности в `script.capabilities`, они проходят гейт на рукопожатии и второй
гейт в `exec.dispatchRequest`, после чего вызываются через канал `request`/`response`.

## Реестр (capreg)

Каждое ядро хранит реестр `capreg.Default` — карту `capability → Handler`, собранную
слиянием под-реестров по категориям (`env` / `net` / `sys`). Ключи реестра —
источник правды для `provides`; `core.manifest` их не дублирует.

## Стартовый пак duplex

### env
| capability | params | result |
|---|---|---|
| `env.get` | `{name}` | `{value}` (или код `ENV_NOT_FOUND`) |

### net
| capability | params | result |
|---|---|---|
| `net.resolve` | `{host, timeout_ms?}` | `{addrs:[…]}` (или код `RESOLVE_FAILED`) |
| `net.tcp.connect` | `{host, port, timeout_ms?}` | `{status, latency_ms}`, `status` ∈ `open\|closed\|filtered` |
| `net.tcp.banner` | `{host, port, timeout_ms?, read_bytes?}` | `{banner, bytes}` (или код `CONNECT_FAILED`) |
| `net.interfaces` | `{}` | `{interfaces:[{name, mac, addrs, flags}]}` |

Классификация `net.tcp.connect`: соединение → `open`; отказ (RST) → `closed`;
таймаут → `filtered`.

### sys
| capability | params | result |
|---|---|---|
| `sys.info` | `{}` | `{os, arch, hostname, numCPU, goVersion}` |
| `sys.env.list` | `{prefix?}` | `{names:[…]}` (значения — через `env.get`) |

## Таймауты

`timeout_ms` — дефолт 1000, потолок 10000 (значения вне диапазона зажимаются).
`read_bytes` — дефолт 256, потолок 4096.

## Ограничение: последовательный диспатч

`dispatchRequest` вызывается синхронно в главном цикле `exec.Run`. Значит запросы
обслуживаются по одному: медленный `net.tcp.connect` (таймаут → `filtered`)
блокирует цикл до возврата. Прозвон большого диапазона портов — последовательный.
Конкурентный диспатч (горутина на запрос) — будущая работа.

## Пример: прозвон портов

`scripts/examples/port-scan/` — «толстый» Go-скрипт (самостоятельный Go-модуль).
Он спрашивает адрес через `prompt` (пустой ответ → `127.0.0.1`), затем конкурентно
прозванивает все 65535 портов сам через `net.DialTimeout` в горутинах (worker-pool)
и собирает открытые в `done.result.open`. Ядерные `net.tcp.connect` и
`net.resolve` при этом **не используются** — `capabilities = []` в манифесте.

Ядерные `net.*` остаются доступны для тонких скриптов, которым нужно прозвонить
несколько конкретных портов без написания собственного Go-кода.

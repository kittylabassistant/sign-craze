# sign-craze: Архитектура

## Диаграмма слоёв

```plain
┌─────────────────────────────────────────────────────┐
│  cmd/sign-craze/main.go  (точка входа, сигналы)     │
└───────────────────────┬─────────────────────────────┘
                        │
┌───────────────────────▼─────────────────────────────┐
│  internal/cli  (диспетчер + обработчики подкоманд)  │
│  install · update · service · ports · geo           │
│  backup · ui · diag                                 │
└──┬────────────────┬────────────────┬────────────────┘
   │                │                │
   ▼                ▼                ▼
internal/       internal/       internal/
singbox         dpi             firewall
(загрузка       (nfqws2:        (iptables/
 установка       загрузка        ipset
 конфиг          установка       режимы:
 версия)         конфиг          tproxy
                 nfqueue         redirect
                 lifecycle)      hybrid)
   │                │                │
   └────────────────┴────────────────┘
                        │
┌───────────────────────▼─────────────────────────────┐
│  internal/exectx  (exec + context + структ. лог)    │
└─────────────────────────────────────────────────────┘
                        │
          ОС: iptables · ipset · ip · opkg
```

## Вспомогательные пакеты

| Пакет | Роль |
|---|---|
| `internal/service` | генерация init.d shim; интерфейс `Lifecycle`, связывающий singbox и nfqws2 |
| `internal/geo` | загрузка SRS из sign-craze-dats; конвертация IP-листа → ipset |
| `internal/web` | встроенный HTTP-сервер (Zashboard + admin UI) |
| `internal/locks` | эксклюзивный flock против параллельных запусков |
| `internal/log` | глобальный `slog.Logger` с ротацией по размеру |
| `internal/atomicfs` | атомарная запись: write → fsync → rename |
| `internal/version` | встроенная `VERSION`, build info через `runtime/debug` |
| `internal/errors` | sentinel-ошибки (`ErrNotInstalled`, `ErrLockHeld`, …) |
| `pkg/types` | общие типы (`Mode`, `Arch`, `Port`, …) |

## Поток данных: `--install`

```plain
cli/install.Run
  → locks.Acquire          (запрет параллельной установки)
  → singbox.Download       (GitHub releases, ETag-кэш, проверка SHA256)
  → singbox.Install        (бэкап текущего, untar, chmod, атомарное переименование)
  → singbox.config.Render  (text/template → tproxy.json)
  → service.shim.Write     (генерация /opt/etc/init.d/S05signcraze)
  → firewall.modes.Apply   (цепочки iptables + ipset + mark-маршрутизация)
  → locks.Release
```

## Поток данных: `--start`

```plain
cli/service.Run
  → locks.Acquire
  → service.compositeLifecycle.Start
      → singbox.lifecycle.Start    (exec detach, запись PID, опрос /proc)
      → nfqws2.lifecycle.Start     (только если режим dpi включён)
  → locks.Release
```

## Режимы маршрутизации

| Режим | sing-box | nfqws2 | iptables |
|---|---|---|---|
| `proxy` | да | нет | TPROXY/REDIRECT → sing-box |
| `dpi` | нет | да | NFQUEUE → nfqws2 |
| `hybrid` | да | да | per-domain: proxy-marked → TPROXY; dpi-marked → NFQUEUE |

Приоритет цепочек в `mangle:PREROUTING`:

1. `signcraze_dpi` (NFQUEUE, `--queue-bypass`) — обрабатывается первой
2. `signcraze` (mark-маршрутизация для TPROXY) — обрабатывается второй

Это гарантирует, что пакеты с меткой proxy не попадают в NFQUEUE.

## Идентификаторы

| Элемент | Значение |
|---|---|
| Цепочки iptables | `signcraze`, `signcraze_full`, `signcraze_dpi` |
| ipset-наборы | `signcraze_ipv4`, `signcraze_ipv6` |
| fwmark | `0x53` |
| ID таблицы маршрутизации | `0x53` |
| Файл блокировки | `/opt/var/lock/sign-craze.lock` |
| PID-файлы | `/opt/var/run/sign-craze-{singbox,nfqws2}.pid` |
| init.d shim | `/opt/etc/init.d/S05signcraze` |
| Корень конфигов | `/opt/etc/sign-craze/` |
| Директория состояния | `/opt/var/lib/sign-craze/` |
| Директория логов | `/opt/var/log/sign-craze/` |

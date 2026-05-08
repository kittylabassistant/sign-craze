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
│  backup · ui · diag · dpi-targets                   │
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
| ----- | ---- |
| `internal/singbox` | загрузка, установка, генерация конфига, версия sing-box |
| `internal/core` | registry + абстрактный интерфейс `Core`; регистрирует ядра: sing-box, xray, mihomo |
| `internal/core/xray` | адаптер ядра xray |
| `internal/core/mihomo` | адаптер ядра mihomo |
| `internal/service` | генерация init.d shim; интерфейс `Lifecycle`, связывающий singbox и nfqws2 |
| `internal/geo` | загрузка SRS из sign-craze-dats; конвертация IP-листа → ipset |
| `internal/web` | встроенный HTTP-сервер (Zashboard :9090 + admin REST API :9091 + Routing Editor :9092 + DPI targets API) |
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
  → singbox.config.Render  (text/template → config.json)
  → service.shim.Write     (генерация /opt/etc/init.d/S99signcraze)
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

## Порты Web UI

| Порт | Назначение |
| ------ | ----------- |
| `9090` | Zashboard — Clash-совместимый dashboard (управление прокси, мониторинг трафика) |
| `9091` | Admin REST API sign-craze |
| `9092` | Routing Editor SPA (Preact) |

Все порты слушают на `0.0.0.0`. Правила в `filter/INPUT` (owner-comment, idempotent) дропают входящий трафик на порт 9090 от WAN-интерфейса.

## Поток данных: `--ui on` (watchdog)

```plain
cli/startUI
  → go firewall.NewWatchdog(0, reconcileFirewall).Run(ctx)   (фон, 30 с)
      loop:
        → IPTables.CheckCriticalRules (iptables -C, дешёвая проверка)
        → если правила отсутствуют → Applier.Reconcile (idempotent re-apply)
  → web.Server.ListenAndServe (блокирует до SIGTERM/ctx.Done)
```

## Режимы маршрутизации

| Режим | sing-box | nfqws2 | iptables |
| ----- | -------- | ------ | -------- |
| `policy` (default) | да | опционально | fwmark 0xffffaaXX → MARK 0x53 → TUN |
| `full` | да | опционально | ipset dst-match → MARK 0x53 → TUN |

Legacy-имена `proxy`, `dpi`, `hybrid` принимаются для обратной совместимости и автоматически конвертируются.

Приоритет цепочек в `mangle:PREROUTING`:

1. `signcraze_dpi` (NFQUEUE, `--queue-bypass`) — обрабатывается первой (только full+DPI)
2. `signcraze` / `signcraze_policy` (mark-маршрутизация) — обрабатывается второй

Цепочка `signcraze_policy_dpi` устанавливается в `mangle:POSTROUTING` (только policy+DPI).

## Идентификаторы

| Элемент | Значение |
| ----- | -------- |
| Цепочки iptables | `signcraze`, `signcraze_full`, `signcraze_dpi`, `signcraze_ports`, `signcraze_policy`, `signcraze_policy_dpi` |
| ipset-наборы | `signcraze_ipv4`, `signcraze_ipv6` |
| fwmark | `0x53` (= 83 dec) |
| ID таблицы маршрутизации | `83` (decimal) |
| Файл блокировки | `/opt/var/lock/sign-craze.lock` |
| PID-файлы | `/opt/var/run/sign-craze-{singbox,nfqws2}.pid` |
| init.d shim | `/opt/etc/init.d/S99signcraze` |
| Корень конфигов | `/opt/etc/sign-craze/` |
| DPI hostlist | `/opt/etc/sign-craze/dpi-hostlist.txt` |
| Директория состояния | `/opt/var/lib/sign-craze/` |
| Директория логов | `/opt/var/log/sign-craze/` |

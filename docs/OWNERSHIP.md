# OWNERSHIP.md — System-state ownership sign-craze

> Версия: 2026-05-03. Автор: @kittylabassistant.

---

## 1. Принцип ownership

Sign-craze удаляет только те системные сущности, которые создал сам — определяемые по точному префиксу имени, полному пути или PID из собственного pidfile. Никаких широких операций: нет `iptables -F`, нет `ipset destroy` без имени, нет `rm -rf /opt/var/run/*`. Если сущность создана другим инструментом (XKeen, Entware-пакеты, ручная настройка) — sign-craze её не трогает, даже если она «выглядит похоже». Правило одно: нет префикса `signcraze_` или известного пути из этого документа — не удалять.

---

## 2. iptables chains (таблица mangle)

Все user-defined chains sign-craze живут исключительно в таблице **mangle**.

| Chain                  | Режим      | Создаётся в                                             | Jump из системной цепочки | Удаление                        | Источник                                      |
|------------------------|------------|---------------------------------------------------------|---------------------------|---------------------------------|-----------------------------------------------|
| `signcraze`            | full       | `applyFullMode`                                         | `PREROUTING`              | `FlushAndDeleteChain` + jump -D | `internal/firewall/applier.go:212`            |
| `signcraze_full`       | full       | `applyFullMode`                                         | `PREROUTING`              | `FlushAndDeleteChain` + jump -D | `internal/firewall/applier.go:212`            |
| `signcraze_dpi`        | full (DPI) | `applyFullMode`                                         | `PREROUTING`              | `FlushAndDeleteChain` + jump -D | `internal/firewall/applier.go:212`            |
| `signcraze_ports`      | full       | `applyFullMode`, `PortRules`                            | `PREROUTING`              | `FlushAndDeleteChain` + jump -D | `applier.go:212`, `modes/ports.go`            |
| `signcraze_policy`     | policy     | `applyPolicyMode`                                       | `PREROUTING`              | `FlushAndDeleteChain` + jump -D | `modes/policy.go:8`                           |
| `signcraze_policy_dpi` | policy+DPI | `applyPolicyMode` (DPIEnabled)                          | `PREROUTING`              | `FlushAndDeleteChain` + jump -D | `modes/policy.go:12`                          |
| `signcraze_probe`      | preflight  | `CheckRequiredIptablesModules` (временная, удаляется в defer) | нет                 | `FlushAndDeleteChain` в defer   | `internal/firewall/preflight.go`              |

**Правило удаления:** для каждой chain сначала `DeleteJumpAll(mangle, PREROUTING, chain)`, затем `FlushAndDeleteChain(mangle, chain)`. Оба вызова идемпотентны. Реализовано в `Remove()` → `internal/firewall/applier.go:276-336`.

---

## 3. iptables jumps (PREROUTING mangle)

Sign-craze устанавливает jump-правила только в `mangle PREROUTING`. Никаких правил в `OUTPUT`, `nat`, `filter`, `raw`.

| Системная цепочка     | Направление трафика     | Условие jump | Target (user chain)       | Режим      |
|-----------------------|-------------------------|--------------|---------------------------|------------|
| `mangle PREROUTING`   | входящий/транзитный     | безусловно   | `-j signcraze`            | full       |
| `mangle PREROUTING`   | входящий/транзитный     | безусловно   | `-j signcraze_full`       | full       |
| `mangle PREROUTING`   | входящий/транзитный     | безусловно   | `-j signcraze_dpi`        | full+DPI   |
| `mangle PREROUTING`   | входящий/транзитный     | безусловно   | `-j signcraze_ports`      | full+ports |
| `mangle PREROUTING`   | входящий/транзитный     | безусловно   | `-j signcraze_policy`     | policy     |
| `mangle PREROUTING`   | входящий/транзитный     | безусловно   | `-j signcraze_policy_dpi` | policy+DPI |

Удаление: `iptables -t mangle -D PREROUTING -j <target>` по точному совпадению args (`DeleteJumpAll`). Цикл не более 16 итераций.

> Loop-prevention для исходящих от sing-box обеспечивается через `ip rule` (fwmark 0x53 lookup 83), не через правила в OUTPUT — поэтому в OUTPUT sign-craze ничего не пишет.

---

## 4. ipset

| Набор                | Тип        | Family         | Используется                                       | Источник                                      |
|----------------------|------------|----------------|----------------------------------------------------|-----------------------------------------------|
| `signcraze_ipv4`     | `hash:net` | `inet`         | Адреса для проксирования в режиме full             | `pkg/types/types.go`, `applier.go:189`        |
| `signcraze_ipv6`     | `hash:net` | `inet6`        | IPv6-адреса для проксирования в режиме full        | `pkg/types/types.go`, `applier.go:192`        |
| `signcraze_excludes` | `hash:net` | `inet`         | CIDR-исключения (RETURN из signcraze), full        | `applier.go:54`, `applier.go:195`             |
| `signcraze_*_tmp`    | `hash:net` | `inet`/`inet6` | Временный atomic swap при `AtomicReplace`          | `internal/firewall/ipset.go:39`               |

**Правило удаления:** `ipset destroy <name>` по точному имени. `_tmp`-наборы удаляются в начале `AtomicReplace` (sweep stale) и после swap. Общий cleanup: `DestroySet` в `Remove()` → `applier.go:324-333`.

**Персистентный дамп:** `/opt/share/sign-craze/ipset.dump` — `SaveIPSet` сохраняет, `RestoreIPSet -!` восстанавливает при старте. Источник: `internal/firewall/ipset_persist.go:16`.

---

## 5. Routing (ip rule / ip route)

| Сущность                                       | Значение                  | Владелец              | Удаление                                  | Источник                              |
|------------------------------------------------|---------------------------|-----------------------|-------------------------------------------|---------------------------------------|
| fwmark                                         | `0x53` (83 dec)           | sign-craze            | `DeleteIPRule` при Remove                 | `applier.go:35`, `route.go:64`        |
| Routing table                                  | `83`                      | sign-craze            | маршрут удаляется, таблица не создаётся явно | `applier.go:36`                    |
| `ip rule fwmark 0x53 lookup 83 priority 32765` | —                         | sign-craze            | `ip rule del fwmark 0x53 table 83`        | `route.go:46-60`                      |
| `ip route table 83 default dev signbox-tun`    | loop-prevention           | sign-craze            | `DeleteTUNRoute` при Remove               | `route.go:100-117`                    |
| TUN netdev `signbox-tun`                       | создаётся sing-box при старте | sign-craze (managed) | `ForceDeleteTUNDevice` перед/после Stop  | `route.go:131`                        |
| `0xffffaaXX` marks                             | Keenetic policy marks     | Keenetic              | **НЕ ТРОГАТЬ** — только через RCI API     | `ndm/policy.go`                       |

**Pre-flight:** перед Apply — `CheckFWMarkAvailable`. Если fwmark `0x53` уже использует другую таблицу (XKeen или кастом) — Apply прерывается с `ErrFWMarkConflict`, не перезаписывая чужое правило. Источник: `internal/firewall/route.go:14-43`.

---

## 6. Файлы и директории

### 6.1 Persistent (переживают restart, удаляются при --uninstall)

| Путь                                   | Содержимое                                          | Удаляется при   | Источник                                           |
|----------------------------------------|-----------------------------------------------------|------------------|----------------------------------------------------|
| `/opt/etc/sign-craze/`                 | `state.json`, sing-box `config.json`, `nfqws2.conf`, `admin.creds` | `--uninstall` | `state/state.go:18`, `singbox/install.go:27`, `dpi/install.go:19` |
| `/opt/var/lib/sign-craze/`             | `cache/`, `geo/` (.srs), `backups/`                 | `--uninstall`   | `singbox/install.go:30`, `geo/srs.go:21`, `backup/backup.go:15` |
| `/opt/var/log/sign-craze/`             | `sing-box.log`, `sing-box.stderr.log`, `boot.log`   | `--uninstall`   | `singbox/lifecycle.go:14`, `cli/cmd_lifecycle.go:231` |
| `/opt/share/sign-craze/ipset.dump`     | Дамп ipset для восстановления после ребута          | `--uninstall`   | `firewall/ipset_persist.go:16`                     |

### 6.2 Volatile (создаются при start, удаляются при stop/uninstall)

| Путь                                       | Содержимое                | Удаляется при                | Источник                                        |
|--------------------------------------------|---------------------------|------------------------------|-------------------------------------------------|
| `/opt/var/run/sign-craze-singbox.pid`      | PID процесса sing-box     | stop, uninstall, stale cleanup | `singbox/lifecycle.go:9`, `cli/cmd_uninstall.go:79` |
| `/opt/var/run/sign-craze-nfqws2.pid`       | PID процесса nfqws2       | stop, uninstall              | `dpi/lifecycle.go:9`, `cli/cmd_uninstall.go:80` |
| `/opt/var/lock/sign-craze.lock`            | flock-файл (не удаляется, снимается блокировка) | процесс завершается | `locks/file.go:16` |

### 6.3 Binaries

| Путь                    | Владелец              | Удаляется при         | Предостережение                                                       | Источник                              |
|-------------------------|-----------------------|-----------------------|-----------------------------------------------------------------------|---------------------------------------|
| `/opt/sbin/sign-craze`  | sign-craze            | `--uninstall`         | —                                                                     | `service/shim.go:18`                  |
| `/opt/sbin/sing-box`    | sign-craze (managed)  | `--uninstall`         | **CAUTION:** XKeen может использовать тот же путь. Не удалять при detected XKeen | `singbox/install.go:23` |
| `/opt/sbin/nfqws2`      | sign-craze (managed)  | `--uninstall`         | **CAUTION:** nfqws2-keenetic может быть установлен независимо         | `dpi/install.go:16`                   |

---

## 7. Service hooks

| Файл                                          | Назначение                                                           | Создаётся   | Удаляется при           | Источник                                  |
|-----------------------------------------------|----------------------------------------------------------------------|-------------|-------------------------|-------------------------------------------|
| `/opt/etc/init.d/S05signcraze`                | Entware init.d shim: start/stop/restart/status → sign-craze         | `--install` | `--uninstall` | `service/shim.go:16`                     |
| `/opt/etc/ndm/netfilter.d/50-sign-craze`      | NDM hook: реаплай mangle-правил после rebuild iptables Keenetic     | `--install` | `--uninstall` | `service/netfilter_hook.go:19`           |

Оба файла пишутся атомарно с проверкой SHA256 (идемпотентно). NDM hook реагирует только на `type=iptables/table=mangle` и `type=ipset`, проверяет PID-файл перед `--reapply`.

---

## 8. Процессы

| Процесс    | PID-файл                                   | Остановка                                          | Запрет                                                                      | Источник                          |
|------------|--------------------------------------------|----------------------------------------------------|-----------------------------------------------------------------------------|-----------------------------------|
| `sing-box` | `/opt/var/run/sign-craze-singbox.pid`      | SIGTERM (grace 10s) → SIGKILL по PID из файла      | **Запрещено** `killall sing-box` — может убить sing-box под управлением XKeen | `singbox/lifecycle.go`, `service/lifecycle.go:152` |
| `nfqws2`   | `/opt/var/run/sign-craze-nfqws2.pid`       | SIGTERM (grace 10s) → SIGKILL по PID из файла      | **Запрещено** `killall nfqws2` — может убить независимый nfqws2-keenetic    | `dpi/lifecycle.go`, `service/lifecycle.go:152` |

**Инвариант:** перед сигналом всегда:
1. PID-файл существует и содержит валидный PID.
2. `/proc/<PID>/status` подтверждает что процесс жив (не зомби).

Stale PID-файл (процесс не в `/proc`) удаляется автоматически при `Status()`.

---

## 9. Keenetic RCI policy

| Сущность              | Имя по умолчанию | Mark (Keenetic)                                   | Создание                       | Удаление                     | Источник              |
|-----------------------|------------------|---------------------------------------------------|--------------------------------|------------------------------|-----------------------|
| Keenetic IP-policy    | `sign-craze`     | `0xffffaaXX` (присваивается Keenetic)             | `ndm.CreatePolicy` + `WaitForMark` | `ndm.DeletePolicy` + `SaveConfig` | `ndm/policy.go`       |

**Правила:**
- Policy создаётся и удаляется **только** через Keenetic RCI API (`/rci/`).
- Mark `0xffffaaXX` **присваивается Keenetic'ом** инкрементально (XKeen=`ffffaaa`, sign-craze=`ffffaab`, ...) — sign-craze его только читает через `GetPolicy`.
- iptables-правила в Keenetic, привязанные к этому mark, **не трогать** напрямую.
- При удалении policy через RCI Keenetic сам убирает свои правила.
- Имя policy хранится в `state.json` → `policy_name` (default `"sign-craze"`). `doUninstall` читает его и вызывает `DeletePolicy` перед удалением state-файла.

---

## 10. Что НЕ владеем

- Цепочки и правила iptables без префикса `signcraze_` в имени target/chain.
- ipset-наборы без префикса `signcraze_`.
- `ip rule` с fwmark, отличным от `0x53` (83).
- `/opt/sbin/sing-box` если обнаружен XKeen: детект — наличие `/opt/etc/init.d/S0*xkeen*` или `sing-box` в ps с PID не из `/opt/var/run/sign-craze-singbox.pid`.
- `/opt/sbin/nfqws2` если установлен независимо (тот же критерий по PID-файлу).
- Процессы `sing-box` / `nfqws2` с PID, не совпадающим с pidfile.
- Routing table, ip rule, маршруты Keenetic, привязанные к marks `0xffffaaXX`.
- Любые файлы вне путей секции 6.

---

## 11. Pre-flight проверки

Выполняются в начале `Apply` до любых изменений системы.

| Проверка                    | Функция                         | При ошибке                                   | Источник                               |
|-----------------------------|---------------------------------|----------------------------------------------|----------------------------------------|
| Доступность `/dev/net/tun`  | `CheckTUNAvailable`             | fail с сообщением про `modprobe tun`         | `firewall/preflight.go`                |
| Наличие xt_set (libxt_set)  | `CheckRequiredIptablesModules`  | fail: «установите ipset через opkg»          | `firewall/preflight.go`                |
| fwmark `0x53` свободен      | `CheckFWMarkAvailable`          | fail с `ErrFWMarkConflict`, не overwrite     | `firewall/route.go:14-43`              |

`CheckFWMarkAvailable` считает конфликтом наличие в `ip rule show` строки с `fwmark 0x53`, указывающей на таблицу, отличную от `83`. Если правило указывает на нашу таблицу 83 — это идемпотентность, не конфликт. На средах без `CAP_NET_ADMIN` проверка пропускается с WARN (не блокирует Docker-тесты).

---

## 12. Cleanup-инвариант

- Каждая операция удаления **идемпотентна**: повторный вызов на чистом состоянии — no-op.
- Удаление только по **точному совпадению**: chain, ipset, полного пути, PID из owned pidfile.
- Порядок `Remove()`: jumps из PREROUTING → flush+delete user chains → TUN route → ip rule → ipset destroy.
- Запрещены широкие операции:
  - `iptables -F` (flush всей таблицы)
  - `ipset destroy` без имени
  - `rm -rf /opt/var/run/*`
  - `killall sing-box` / `killall nfqws2`
- `_tmp`-наборы ipset подметаются в начале `AtomicReplace` и после swap.
- При ошибке во время `Apply` автоматически вызывается `Remove` для отката (`applier.go:116-122`).

---

## 13. Связь с CODEOWNERS

`.github/CODEOWNERS`: `* @kittylabassistant` — **code-review ownership** (кто аппрувит PR).

`docs/OWNERSHIP.md` (этот файл) — **system-state ownership**: какие kernel/filesystem/process-сущности принадлежат sign-craze и могут быть безопасно удалены при uninstall или аварийном cleanup.

Документы ортогональны: CODEOWNERS отвечает «кто проверяет код», OWNERSHIP — «что можно сносить на роутере без риска сломать соседние инструменты».

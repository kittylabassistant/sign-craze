# sign-craze — фиксы безопасности перед первым запуском на роутере

Источник: аудит безопасности от 2026-04-29 (3 параллельных Explore-агента: firewall, install/lifecycle/lock, память/размер).

Решения пользователя зафиксированы по каждому пункту. К фиксам не приступать без отдельной команды.

---

## 🔴 CRIT — может убить роутер при первом запуске

### 1. SSH lockout — excludes используют `-A` вместо `-I`
- **Файлы:** `internal/firewall/applier.go:115-122`, `internal/firewall/iptables.go:28`
- **Проблема:** Excludes аппендятся в конец цепочки через `-A`. На первом install работает случайно (цепочка пуста), но при повторном Apply трафик помечается ДО проверки exclude → SSH в TPROXY → потеря доступа.
- **Fix:** Использовать `-I` (insert head) для exclude-правил, либо валидировать порядок правил перед Apply.
- **Статус:** DONE

### 2. Нет дефолтного исключения локальных сетей и SSH
- **Файлы:** `internal/firewall/applier.go`
- **Проблема:** ipset `signcraze_ipv4` может содержать `0.0.0.0/0` или агрегаты, покрывающие LAN. SSH-пакеты и трафик на LAN-устройства маркируются → tproxy на 7895 → мгновенный лок и потеря локальных сервисов.
- **Fix:** Автоматом добавлять в excludes:
  - `127.0.0.0/8` (loopback)
  - `169.254.0.0/16` (link-local)
  - `224.0.0.0/4` (multicast)
  - `10.0.0.0/8` (RFC1918)
  - `172.16.0.0/12` (RFC1918)
  - `192.168.0.0/16` (RFC1918)
  - port 22 (SSH)
  - CLI флаг `--admin-port` для дополнительного исключения
- **Статус:** DONE

### 3. Нет проверки конфликта fwmark 0x53
- **Файлы:** `internal/firewall/applier.go:67`
- **Проблема:** XKeen / кастомные скрипты могут уже использовать fwmark 0x53. `EnsureIPRule` молча скипает существующее правило — мы садимся в чужую таблицу маршрутизации и ломаем её владельца.
- **Fix:** Pre-flight проверка через `ip rule show` → fail с понятной ошибкой если 0x53 уже занят.
- **Статус:** DONE

### 4. ~~SHA256 верификация sing-box~~
- **Решение пользователя:** оставить отключённой. Из списка фиксов исключено.

### 5. init-shim S05signcraze без error handling
- **Файлы:** `internal/service/shim.go:22-32`
- **Проблема:** Шаблон шелл-скрипта использует голый `exec` без `set -e` и без обработки ошибок. Если `--service-start` падает, init молчит → нет автостарта после ребута, юзер думает что работает.
- **Fix:** Добавить `set -e` в начало шима + обернуть exec: `exec "$SC" --service-start || { echo "sign-craze start failed" >&2; exit 1; }`
- **Статус:** DONE

---

## 🟠 HIGH

### 6. tproxy без bypass loopback/link-local
- **Файлы:** `internal/firewall/modes/tproxy.go:60-65`
- **Проблема:** PREROUTING TPROXY-правила не имеют `! -s 127.0.0.0/8 ! -s 169.254.0.0/16 ! -i lo`. TPROXY на localhost → ломает TCP/UDP-стек.
- **Fix:** Добавить эти исключения к PREROUTING TPROXY rules.
- **Статус:** DONE

### 7. DoS через unbounded HTTP body
- **Файлы:** `internal/web/server.go` (Server config), `internal/web/api.go:56` (apiConfigPost), `:88` (apiPortsAdd), `:142` (apiExcludesAdd)
- **Проверено:** `MaxHeaderBytes` не задан → Go default 1MB (приемлемо). НО все POST-обработчики используют `json.NewDecoder(r.Body).Decode()` без лимита размера тела. POST `/api/config` принимает произвольный JSON — 50MB malformed JSON убивает роутер с 128MB RAM.
- **Fix:**
  - В каждом POST-хендлере обернуть body: `r.Body = http.MaxBytesReader(w, r.Body, 1<<20)` (1MB лимит).
  - Опционально явно зафиксировать `MaxHeaderBytes = 8192` в `ServerConfig`.
- **Статус:** DONE

### 8. Stale lock не детектится
- **Файлы:** `internal/locks/file.go:27-54`, `internal/cli/lock.go:18-23`
- **Проблема (исходная):** flock использует только ctx-таймаут. Если процесс убит через `kill -9`, defer `Release()` не выполняется, lockfile остаётся навсегда.
- **Проверено при имплементации:** `flock(LOCK_EX|LOCK_NB)` привязан к file descriptor — при kill -9 ОС закрывает fd, и flock автоматически освобождается. Lockfile остаётся, но новый Acquire успешно берёт flock на ту же inode. Stale lock в текущей реализации **невозможен**.
- **Статус:** NOT NEEDED (аудит был неточен относительно семантики flock)

### 9. Hybrid mode — DPI rules мертвы
- **Файлы:** `internal/firewall/modes/hybrid.go:14-37`
- **Проблема (исходная):** аудит утверждал что DPI-правила аппендятся ПОСЛЕ mark.
- **Проверено при имплементации:** `hybrid.go:37` возвращает `append(dpiRules, rules...)` — DPI правила идут ПЕРВЫМИ. Тест `TestHybridRules_DPIПравилаИдутПервыми` подтверждает. Аудит был неточен.
- **Статус:** NOT NEEDED (уже корректно реализовано)

### 10. Partial install — бинарь остаётся при битом конфиге
- **Файлы:** `internal/cli/cmd_install.go:119-127`
- **Проблема:** Валидация конфига выполняется ПОСЛЕ атомарной записи бинаря. Если генерация конфига падает после успешной установки sing-box — бинарь установлен, конфиг сломан, состояние неконсистентно.
- **Fix:** Валидировать конфиг ДО установки бинаря, либо откатывать бинарь при ошибке генерации конфига.
- **Статус:** DONE

### 11. install.sh без disk space check и без SHA256
- **Файлы:** `scripts/install.sh:47`
- **Проблема:** wget на роутер с <100MB свободного места заполнит диск. SHA256 в установщике не проверяется (даже хотя ghrelease поддерживает). Скомпрометированный или повреждённый бинарь не детектится.
- **Fix:**
  - Добавить statfs-проверку перед wget (тот же порог что в Go-бинаре, ~30MB).
  - Скачивать .sha256-файл рядом с бинарём + `sha256sum -c` после загрузки.
- **Статус:** DONE

---

## 🟡 MED

### 12. Персистентность через re-apply из init.d (entware/USB)
- **Файлы:** `internal/service/shim.go`, `internal/cli/cmd_lifecycle.go:116-141`, `internal/firewall/applier.go`
- **Решение пользователя:** установка только на entware (USB-флешка). Проверить, что состояние восстанавливается после ребута.
- **Проверено:** ip rule, ip route, iptables-цепочки и ipset — это runtime-состояние ядра, в файл не пишутся (на любой системе). Персистентность достигается через re-apply при загрузке:
  1. USB монтируется → entware запускает `/opt/etc/init.d/rc.unslung` → `S05signcraze start`
  2. Шим (`/opt/etc/init.d/S05signcraze`, лежит на USB) → `sign-craze --service-start`
  3. `--service-start` → `waitDefaultRoute(30s)` → `doStart` → `applier.Apply` → пересоздаёт всё состояние в ядре
- **Артефакты на USB (переживают ребут):**
  - `/opt/sbin/sign-craze` — бинарь
  - `/opt/etc/init.d/S05signcraze` — шим автостарта
  - `/opt/etc/sign-craze/` — config.json, state.json, admin.creds
  - `/opt/sbin/sing-box`, `/opt/sbin/nfqws2` — внешние бинари
  - `/opt/share/sign-craze/geo/*.srs` — гео-файлы
- **Остаточные риски (требуют проверки на железе):**
  - 30s таймаут `waitDefaultRoute` может быть мал на медленной загрузке роутера → автостарт упадёт молча.
  - Если `state.json` повреждён или отсутствует → `doStart` падает на `loadState`, шим возвращает ошибку, но init не уведомит юзера.
  - Если USB примонтирован ПОСЛЕ запуска `/opt/etc/init.d/*` (асинхронный mount) → шима нет в `/opt/etc/init.d/` на момент старта → автостарт не сработает. Зависит от entware-конфигурации `mount-opt` / `usb-events`.
- **Fix (опционально):**
  - Увеличить таймаут до 60-90s или сделать настраиваемым.
  - Логировать причину фейла `--service-start` в `/opt/var/log/sign-craze/boot.log` (отдельный файл, чтобы был виден после следующего успешного старта).
  - Документировать в README порядок монтирования USB на Keenetic / OpenWRT.
- **Статус:** PARTIAL — таймаут 60s сделан настраиваемым через state.BootTimeoutSec; boot.log реализован в shim (Phase 2.1). README-секция и тест на железе — отдельной задачей.

### 13. ipset swap не cleanup при OOM
- **Файлы:** `internal/firewall/ipset.go:38-66`
- **Проблема:** Если `ipset swap` падает из-за OOM, tmp-set выживает, `DestroySet()` пытается удалить (line 62) но падает молча. При повторных Apply копятся `signcraze_*_tmp*` в памяти.
- **Fix:** Строго проверять return code swap; в Apply делать предварительный sweep tmp-сетов; документировать ручную очистку `ipset destroy signcraze_*_tmp*`.
- **Статус:** DONE

### 14. Geo-файлы целиком в RAM
- **Файлы:** `internal/geo/srs.go:152,157`
- **Проблема:** `io.ReadAll(resp.Body)` загружает .srs (10-30MB каждый) полностью в RAM перед `WriteFileAtomic`. На update пик 20-30MB — критично для 128MB роутера.
- **Fix:** Использовать `io.CopyN` с буфером 512KB или streaming write через temp-файл с переименованием.
- **Статус:** DONE

### 15. WebSocket `/traffic` без idle timeout
- **Файлы:** `internal/web/clash.go:59-78`
- **Проблема:** Каждое соединение спавнит горутину `for range tick.C` без idle timeout. 10 завернувшихся клиентов = 10 живых горутин (~64KB каждая) + аллокации тикера.
- **Fix:** Добавить per-connection idle timeout (например `time.NewTimer(90s)`) для разрыва при бездействии.
- **Статус:** DONE

### 16. `--install` не идемпотентен
- **Файлы:** `internal/cli/cmd_install.go:38-50`
- **Проблема:** Двойной запуск `--install` молча перезаписывает state.json и все файлы. Нет проверки существующей установки.
- **Fix:** Проверять наличие state.json или sing-box → требовать явный `--reinstall`/`--force` для перезаписи.
- **Статус:** DONE

---

## 🔴 CRIT (отдельный, найден при проверке #12)

### 17. ipset signcraze_ipv4/ipv6 создаются пустыми и НИКОГДА не заполняются
- **Файлы:** `internal/firewall/applier.go:83-88`, `internal/geo/ipset.go:39-43`
- **Проблема:** `applier.Apply` создаёт ipset `signcraze_ipv4` и `signcraze_ipv6` через `EnsureSet` пустыми. `geo.ApplyToIPSet` (заполнение из .srs) вызывается ТОЛЬКО из `geo/ipset_test.go` — нигде в продакшен-коде. `cmd_update.go` вызывает только `geo.Update` (загрузка .srs на диск), но не population ipset.
- **Последствия:**
  - После `--start` или `--service-start` (после ребута): ipset существует, но пустой → `iptables -m set --match-set signcraze_ipv4 dst` ничего не матчит → трафик не маркируется → tproxy не активируется → sign-craze тихо стартует, но НИЧЕГО не проксирует. Юзер думает что работает.
  - В hybrid и proxy режимах одинаково — оба зависят от этих ipset (см. `modes/tproxy.go:22-35`).
- **Возможные интерпретации дизайна:**
  - sing-box использует свои `.srs` через `route_set` правила в собственном движке маршрутизации — возможно, `signcraze_ipv4`/`ipv6` задумывались как пользовательские (наполняются вручную через `--exclude-add` или CLI, который пока не написан).
  - ИЛИ: пропущен этап интеграции `geo.ApplyToIPSet` в `doStart` после `applier.Apply` и в `cmd_update.go --update-geo` после `geo.Update`.
- **Fix (требует решения по дизайну):**
  - Вариант A: Вызывать `geo.ApplyToIPSet` в `doStart` после `applier.Apply` (читать `.srs` → парсить → загружать в ядро). Дорого по RAM на старте.
  - Вариант B: Вызывать `geo.ApplyToIPSet` в `cmd_update.go --update-geo` (после загрузки .srs обновить ipset) + персистентный дамп ipset в файл, восстановление при `--service-start` через `ipset restore`.
  - Вариант C: Документировать что `signcraze_ipv4`/`ipv6` — пользовательские, и трафик проксируется только через port-rules / excludes / явные CIDR.
- **Статус:** DONE — выбран вариант B: `geo.DecompileSRS` (sing-box rule-set decompile) → `ApplyToIPSet` → `firewall.SaveIPSet` (`/opt/share/sign-craze/ipset.dump`). При `--start` / `--service-start` вызывается `firewall.RestoreIPSet`. Если sing-box ещё не установлен — warn, продолжаем (первый запуск до --update-geo).

---

## Сводка приоритетов

| Приоритет | Кол-во | Комментарий |
|-----------|--------|-------------|
| CRIT | 5 | #1, #2, #3, #5, #17 — все DONE |
| HIGH | 6 | #6, #7, #10, #11 — DONE; #8, #9 — NOT NEEDED (проверено при имплементации) |
| MED | 5 | #13, #14, #15, #16 — DONE; #12 — PARTIAL (код DONE, доки + тест на железе отдельно) |

**Исключено по решению пользователя:** #4 (SHA256 verify).

**Минимум перед запуском на проде:** все CRIT (1, 2, 3, 5, 17). #17 — сначала решить дизайн, потом фиксить.

**Порядок применения (TDD):** тест → fail → fix → refactor. Не начинать без явной команды.

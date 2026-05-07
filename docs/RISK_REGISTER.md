# RISK_REGISTER.md — sign-craze

> Версия: 2026-05-03
> Методология: Severity × Likelihood.
> Содержит только **активные** риски (open / mitigated / accepted / partial).
> Закрытые (DONE) риски — в §5 с отсылкой на `tasks/safety-fixes.md`.

---

## 1. Обзор

Риск — любое условие, при реализации которого пользователь получает: потерю доступа к роутеру, утечку трафика мимо прокси, повреждение постороннего сетевого стека, отказ автостарта без уведомления, или деградацию производительности на целевом железе (Keenetic, 128 MB RAM, MIPS/ARM).

Регистр обновляется раз в фазу (см. §7) и при добавлении новой подсистемы. Закрытые риски не удаляются — переносятся в §5.

---

## 2. Шкала severity

| Уровень | Определение                                                                       | Примеры                                              |
|---------|-----------------------------------------------------------------------------------|------------------------------------------------------|
| CRIT    | Необратимый ущерб: утечка данных, brick роутера, SSH lockout без recovery         | SSH lockout (Issue #1), ipset пустой → весь трафик мимо (#17) |
| HIGH    | Функциональный отказ: feature не работает, требует ручного вмешательства          | Автостарт не срабатывает, XKeen-конфликт, E2E не верифицирован |
| MED     | Деградация: feature частично работает или работает с условием                     | Watchdog отсутствует, утечка в узком окне reapply, redirect-stub |
| LOW     | Неудобство: не влияет на безопасность и функциональность при нормальном сценарии  | GitHub rate-limit при install, TPROXY module не проверяется явно |

---

## 3. Шкала likelihood

| Уровень | Определение                                                                | Примеры                                             |
|---------|----------------------------------------------------------------------------|-----------------------------------------------------|
| H       | Воспроизводится у >50% пользователей при нормальном использовании         | —                                                   |
| M       | Специфический сценарий: нужна конкретная конфигурация или действие         | XKeen установлен параллельно; пользователь удаляет policy в UI |
| L       | Редкие edge-cases: нестандартное железо, экзотическая конфигурация, race   | Медленный USB >60s; MIPS big-endian; fwmark занят чужим |

---

## 4. Активные риски

| ID   | Категория          | Описание                                                                                                                         | Severity | Likelihood | Impact                                                                     | Mitigation                                                                                                       | Status    | Источник                                                                |
|------|--------------------|---------------------------------------------------------------------------------------------------------------------------------|----------|------------|----------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|-----------|-------------------------------------------------------------------------|
| R001 | Сеть / утечка      | В окне reapply при WAN-fallback назначенном вручную в Keenetic UI (`permit` с interface) table 4098 получает default через провайдера → возможен leak | MED      | L          | Трафик клиентов в policy идёт напрямую несколько секунд reapply            | `ndm.EnsurePolicy` создаёт policy без `permit`-канала; документировано как known limitation                       | accepted  | `docs/BEHAVIOR_SPEC.md:467-471`; `tasks/safety-fixes.md`                |
| R002 | Watchdog           | Пользователь удаляет policy «sign-craze» через Keenetic Web UI → sign-craze продолжает работу, Keenetic не маркирует трафик → policy-mode тихо перестаёт проксировать | MED      | M          | Пользователь думает что защита активна; трафик идёт напрямую без уведомления | Нет. Детектируется только при следующем `--start` (re-read mark из RCI)                                          | open      | `tasks/phase9-policy.md:153-157`                                        |
| R003 | Автостарт          | init.d шим запускается до USB-монтирования (асинхронный mount >60s) → шим не существует в момент старта rc.unslung → автостарт не срабатывает | HIGH     | L          | После ребута роутер без прокси; пользователь не получает уведомления       | `BootTimeoutSec` настраиваемый (60–90s); boot.log в shim; README + тест на железе — отдельно                      | partial   | `tasks/safety-fixes.md:95-115` (Issue #12)                              |
| R004 | Firewall / fwmark  | Конфликт fwmark 0x53 с другим инструментом (XKeen, кастомные скрипты) → `CheckFWMarkAvailable` → `ErrFWMarkConflict`             | MED      | L          | Fail-fast с понятной ошибкой; сеть не ломается                              | Pre-flight `CheckFWMarkAvailable` через `ip rule show`; при конфликте — ошибка, не перезапись                    | mitigated | `internal/firewall/route.go:14`; Issue #3 в `safety-fixes.md`           |
| R005 | Конфликт бинарей   | XKeen и sign-craze установлены одновременно, оба используют `/opt/sbin/sing-box` → один процесс может перезаписать бинарь другого при update | HIGH     | M          | Один из инструментов работает с несовместимой версией; SIGILL или silent misconfiguration | Детект конфликта; пользователю предупреждение; разрешение — вручную                                              | partial   | `tasks/todo.md`                                                         |
| R006 | Персистентность    | После ребута mark, присвоенный Keenetic policy, эмпирически не проверен на стабильность при `system configuration save` → mark может измениться | MED      | M          | Policy-mode тихо перестаёт работать после ребута                            | `SaveConfig` вызывается в RCI; эмпирическая проверка на живом роутере не выполнена                              | open      | `tasks/phase9-policy.md:155-157`                                        |
| R007 | Режим redirect     | Пользователь выбирает `--mode redirect` → `RedirectRules()` возвращает `nil` → ни одного правила; трафик напрямую без предупреждения | LOW      | L          | Тихий no-op для пользователя                                                | Stub документирован в комментарии файла                                                                          | accepted  | `internal/firewall/modes/redirect.go`                                   |
| R008 | Установка / update | GitHub API rate-limit (60 req/hour без токена) при `--install` или `--update-core` → HTTP 403                                  | LOW      | M          | Установка/обновление невозможны до сброса лимита                            | `--install-offline` передаёт локальный архив; документировано в help                                            | mitigated | `cmd/sign-craze/main.go`; Phase 7 release pipeline                     |
| R009 | Память / geo       | Geo-файлы (.srs) 10–30 MB на роутере 128 MB RAM → пиковое потребление при update                                                | MED      | L          | OOM killer убивает sign-craze или sing-box                                  | Streaming через temp-файл с `io.CopyN` (буфер 512 KB) + atomic rename; Issue #14 DONE                          | mitigated | `tasks/safety-fixes.md:123-127` (Issue #14)                             |
| R010 | Сборка / MIPS      | Случайная сборка с `GOMIPS=hardfloat` → SIGILL на KN-1810 (MIPS без FPU)                                                       | HIGH     | L          | Бинарь крашится при запуске на целевом роутере                              | `release.yml` и `Makefile` форсируют `GOMIPS=softfloat` для mipsle/mips; CI matrix гарантирует                 | mitigated | `.github/workflows/release.yml:27-29`                                   |
| R011 | Web UI / безопасность | Web UI без TLS, POST `/api/config` открыт в LAN → MITM или злоумышленник в LAN может изменить конфигурацию                 | MED      | M          | Изменение прокси-конфигурации, redirect трафика на свой сервер              | bcrypt (cost=12); `MaxBytesReader(1MB)` на всех POST; Issue #7 DONE                                              | mitigated | `tasks/safety-fixes.md:57-62` (Issue #7)                                |
| R012 | Release / flash    | UPX не сжимает MIPS big-endian → бинарь mips остаётся >4 MB → может не поместиться на flash                                    | MED      | L          | Установка на MIPS BE с малым flash невозможна                               | `release.yml` использует `upx ... \|\| true` (silent skip); пользователь видит размер в release                  | accepted  | `.github/workflows/release.yml:64`                                      |
| R013 | Vendor API         | Keenetic 5.x → 6.x: RCI breaking change → `internal/ndm` не парсит ответы → policy-mode не стартует                            | HIGH     | L          | Полный отказ policy-mode; full-mode как fallback                            | Нет SLA от vendor; нет мониторинга changelog RCI; sign-craze логирует ошибку                                     | accepted  | `internal/ndm/policy.go`; `tasks/phase9-policy.md`                     |
| R014 | Kernel modules     | `xt_TPROXY` kernel module не проверяется явно в preflight → apply может упасть без понятной диагностики                        | LOW      | L          | Непонятная ошибка при первом `--start`; ручная диагностика `dmesg`         | `CheckRequiredIptablesModules` проверяет `xt_set`; TPROXY probe — отдельная задача                              | open      | `internal/firewall/preflight.go:46`                                     |
| R015 | Конкурентность     | Concurrent `--start` от init.d и manual → race на pidfile / двойное применение firewall-правил                                  | MED      | L          | Дублированные правила; второй старт `ErrLockHeld` и упасть                  | `flock /opt/var/lock/sign-craze.lock` (LOCK_EX) гарантирует исключительный доступ                                | mitigated | `internal/locks/file.go`; Issue #8                                      |
| R016 | E2E / верификация  | Phase 9.8 E2E на живом Keenetic 5.x не выполнен → unknown unknowns в production: любой из R001–R015 может оказаться CRIT      | HIGH     | M          | Непредвиденные сбои при первом реальном запуске; потенциальная потеря SSH   | Симуляция через httptest mock; Docker integration tests; финальное на железе — блокер                            | open      | `tasks/todo.md:109-115` (Phase 9.8)                                     |

---

## 5. Закрытые риски (история)

Все перечисленные имеют статус **DONE** в `tasks/safety-fixes.md`. Детали, код и тесты — там.

| Ref        | Краткое описание                                          | Severity |
|------------|-----------------------------------------------------------|----------|
| Issue #1   | SSH lockout: excludes через `-A` вместо `-I`              | CRIT     |
| Issue #2   | Нет дефолтных исключений LAN/loopback/SSH                 | CRIT     |
| Issue #3   | fwmark 0x53 конфликт (pre-flight добавлен)                | CRIT     |
| Issue #5   | init-shim без `set -e` / error handling                   | CRIT     |
| Issue #6   | TPROXY без bypass loopback/link-local в PREROUTING        | HIGH     |
| Issue #7   | DoS через unbounded HTTP body (MaxBytesReader 1MB)        | HIGH     |
| Issue #10  | Partial install: бинарь остаётся при битом конфиге        | HIGH     |
| Issue #11  | install.sh без disk space check и SHA256                  | HIGH     |
| Issue #13  | ipset swap не cleanup при OOM (tmp-sets leak)             | MED      |
| Issue #14  | Geo-файлы целиком в RAM → OOM при update                  | MED      |
| Issue #15  | WebSocket `/traffic` без idle timeout                     | MED      |
| Issue #16  | `--install` не идемпотентен (overwrite state.json)        | MED      |
| Issue #17  | ipset signcraze_ipv4/ipv6 создавались пустыми → no proxy  | CRIT     |

Полные описания, affected files и статусы: `tasks/safety-fixes.md`.

---

## 6. Принципы mitigation

- **Pre-flight перед mutating**: любая операция, изменяющая сетевое состояние ядра (apply, install, update), выполняется только после серии проверок (fwmark availability, iptables modules, disk space, TUN device). Ошибка pre-flight → отказ с понятным сообщением, никаких частичных изменений.
- **Fail-fast при конфликте, не overwrite**: при обнаружении чужого fwmark, чужого sing-box бинаря или занятого порта — sign-craze сообщает об ошибке и останавливается. Молча перезаписать чужую конфигурацию нельзя.
- **Cleanup-инвариант**: каждая мутация имеет парную cleanup-операцию (Remove для Apply, DeletePolicy для EnsurePolicy, RestoreBackup для BackupAndReplace). Remove вызывается в defer при ошибке. См. `docs/OWNERSHIP.md`.
- **TDD для firewall-логики**: изменения в `internal/firewall/` требуют сначала упавшего теста, затем фикса, затем рефактора. Нет тестируемого инварианта → нет мерджа.
- **Документировать accepted-риски явно**: риски `accepted` фиксируются здесь с обоснованием. «Мы знаем, приняли решение жить с этим» — не то же что «мы не знали».

---

## 7. Review cadence

- **Раз в фазу**: при закрытии фазы (commit с тегом `phase-N-done`) — просмотр регистра, обновление статусов, добавление новых рисков от новой подсистемы.
- **При добавлении подсистемы**: новый пакет `internal/X` или новая CLI-команда → обязательная секция «риски» в PR-описании + запись в регистр, даже если статус сразу `mitigated`.
- **При vendor-событии**: новый major KeeneticOS / sing-box / nfqws2 release → проверить R013, R004, R005 на актуальность.
- **Owner**: автор изменения, затрагивающего риск, несёт ответственность за обновление строки.

# RISK_REGISTER.md — sign-craze

> Версия: 2026-06-19 (v1.6.1)
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
| R011 | Web UI / безопасность | Web UI без TLS, POST `/api/config` открыт в LAN → MITM или злоумышленник в LAN может изменить конфигурацию                 | MED      | M          | Изменение прокси-конфигурации, redirect трафика на свой сервер              | bcrypt (cost=12 на arm64/arm7; cost=10 на mips/mipsle через auth_cost_lowmem.go, v1.4.0). См. R029.; `MaxBytesReader(1MB)` на всех POST; Issue #7 DONE | mitigated | `tasks/safety-fixes.md:57-62` (Issue #7)                                |
| R012 | Release / flash    | UPX не сжимает MIPS big-endian → бинарь mips остаётся >4 MB → может не поместиться на flash                                    | MED      | L          | Установка на MIPS BE с малым flash невозможна                               | `release.yml` использует `upx ... \|\| true` (silent skip); пользователь видит размер в release                  | accepted  | `.github/workflows/release.yml:64`                                      |
| R013 | Vendor API         | Keenetic 5.x → 6.x: RCI breaking change → `internal/ndm` не парсит ответы → policy-mode не стартует                            | HIGH     | L          | Полный отказ policy-mode; full-mode как fallback                            | Нет SLA от vendor; нет мониторинга changelog RCI; sign-craze логирует ошибку                                     | accepted  | `internal/ndm/policy.go`; `tasks/phase9-policy.md`                     |
| R014 | Kernel modules     | `xt_TPROXY` kernel module не проверяется явно в preflight → apply может упасть без понятной диагностики                        | LOW      | L          | Непонятная ошибка при первом `--start`; ручная диагностика `dmesg`         | `CheckRequiredIptablesModules` проверяет `xt_set`; TPROXY probe — отдельная задача                              | open      | `internal/firewall/preflight.go:46`                                     |
| R015 | Конкурентность     | Concurrent `--start` от init.d и manual → race на pidfile / двойное применение firewall-правил                                  | MED      | L          | Дублированные правила; второй старт `ErrLockHeld` и упасть                  | `flock /opt/var/lock/sign-craze.lock` (LOCK_EX) гарантирует исключительный доступ                                | mitigated | `internal/locks/file.go`; Issue #8                                      |
| R016 | E2E / верификация  | Phase 9.8 E2E на живом Keenetic 5.x не выполнен → unknown unknowns в production: любой из R001–R015 может оказаться CRIT      | HIGH     | M          | Непредвиденные сбои при первом реальном запуске; потенциальная потеря SSH   | Симуляция через httptest mock; Docker integration tests; финальное на железе — блокер                            | open      | `tasks/todo.md:109-115` (Phase 9.8)                                     |
| R017 | DPI / семантика    | `state.DPIExcludeIPs` содержит IP не VPN-сервера (ошибка пользователя или резолв изменился) → nfqws2 десинкает Reality-handshake → маскировка ломается → ISP может определить VPN-туннель | HIGH     | M          | Трафик идёт через nfqws2 к нецелевому IP; возможна деанонимизация VPN      | `Validate` проверяет parseable IP, семантическое соответствие (что IP принадлежит VPN-серверу) не проверяется; документация в `--help` рекомендует `tcpdump host <vpn-server>` | open      | `internal/state/state.go`; `docs/BEHAVIOR_SPEC.md`                      |
| R018 | DPI / цепочка поставок | `dpi_update_urls` скачивает hostlist через HTTPS из публичных репо (bol-van/zapret, Flowseal/zapret-discord-youtube) без signature verification → компрометация upstream может подсунуть зловредный hostlist → nfqws2 десинкает легитимный трафик | MED      | L          | Банк/госуслуги и другие домены из подменённого hostlist могут отваливаться  | TLS минимум 1.2, проверка HTTP 2xx; PGP-подписи на upstream нет; контроль через `--dpi-update-urls`             | open      | `internal/dpi/update.go`                                                 |
| R019 | DPI / трафик       | Watchdog-горутина скачивает ≤2 MB на URL × 3+ URL × каждый интервал → на медленном MIPS-роутере с metered cellular может создавать заметную нагрузку | LOW      | L          | Неожиданный расход трафика и CPU на слабом железе                           | Интервал по умолчанию 0 (обновление выключено); пользователь явно задаёт `--dpi-update-interval`               | open      | `internal/dpi/update.go`                                                 |
| R020 | Firewall / NDM     | NDM rebuild loop приводил к 263 reapply/час → дублирование правил, деградация производительности                                | HIGH     | M          | Дублированные firewall-правила; перегрузка NDM-обработчика                  | Throttle 5s + Reconcile ⇒ ≤12/час; исправлено в v0.8.0                                                          | mitigated | `internal/ndm/watcher.go`; v0.8.0 changelog                             |
| R021 | DPI / VPN          | nfqws2 десинкал sing-box-исходящий трафик к VPN-серверу → Reality-handshake ломался в момент выхода из туннеля                 | HIGH     | H          | VPN-соединение не устанавливается при активном DPI                          | RETURN-правила в PREROUTING/OUTPUT исключают VPN-сервер из обработки nfqws2; исправлено в v0.8.0               | mitigated | `internal/firewall/modes/tproxy.go`; v0.8.0 changelog                   |
| R022 | Сеть / Auto-update | `resilientResolver` использует фиксированный список `{1.1.1.1, 9.9.9.9, 8.8.8.8}`. На некоторых ISP (особенно cellular в РФ) Cloudflare 1.1.1.1 блокируется/throttle'ится; в редких случаях все три могут быть недоступны. | LOW      | L          | Auto-update hostlist не может зарезолвить URL → SourcesFailed > 0, hostlist устаревает | Список из 3 серверов даёт redundancy. При полном fail — пользователь явно задаёт DPITargets через `--dpi-targets`. Long-term: сделать список настраиваемым через state | open      | `internal/dpi/update.go`; v0.8.1                                         |
| R023 | Зависимость от upstream | `internal/dpi/update.go::DefaultUpdateURLs` ссылается на конкретные пути в Flowseal/zapret-discord-youtube (list-general.txt, list-google.txt). Upstream может переименовать файлы или сменить layout (как было с list-discord.txt → list-general.txt). | HIGH     | M          | Свежие установки получают 404 на дефолтных URL → пустой hostlist → selective-DPI не работает до явной настройки `--dpi-update-urls` | При изменении upstream — патч + новый релиз. Smoke-тест `--dpi-update-now` в CI                                 | open      | `internal/dpi/update.go`; v0.8.2                                         |
| R024 | Состояние / Конфигурация | Если пользователь параллельно редактирует `routing.json` руками, а sign-craze запускает migration (фильтрация TUN-inbound + Save), edit пользователя теряется.                                                     | MED      | L          | Manual customization routing.json (custom outbounds, route_rules) пропадает | Save идёт через atomic temp+rename (durable). Сообщение в WARN-логе при удалении TUN. Long-term: бэкап routing.json в `routing.json.pre-migration` перед перезаписью | open      | `internal/cli/deps.go::configParamsFromState`; v0.8.3                                     |
| R035 | DPI / supply chain | Naiveproxy/mieru бинарь скачивается из `klzgrad/naiveproxy` и `enfein/mieru` GitHub releases. SHA256 не верифицируется sign-craze (только TLS + cosign публикуется upstream выборочно). | MED | L | Подменённый бинарь маскирует трафик некорректно или ведёт исходящий dial | TLS 1.2 минимум, путь через `internal/naiveproxy/download.go` валидирует HTTP 2xx; пользователь явно opt-in `--with-naive` | open | `internal/naiveproxy/download.go`; v1.3.0 |
| R036 | Ядро / совместимость | xray-core v26.3.27 не стартует на mips/mipsle (Keenetic ядро Linux 3.4–4.x): `runtime.futexwakeup ... -89` (ENOSYS — syscall отсутствует). Прочие архитектуры (arm64, arm7) v26.3.27 работают штатно. | HIGH | H (для mips/mipsle пользователей) | xray не запускается; прокси недоступен; трафик идёт напрямую | Пин custom-build v25.12.8 для GOARCH=mips/mipsle в `internal/core/xray/download.go` (`XRayMIPSVersion`); arm64/arm7 получают актуальный upstream. Детектируется при `--update-core` — платформо-зависимый URL. | mitigated | baseline 2026-06-19; `internal/core/xray/download.go` |

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
| R025-SSH-bypass | SSH/admin lockout при policy с привязкой 10+ устройств: трафик `dst=LAN_IP:222` ловился TPROXY → коннект виснул. Mitigated в v1.1.1 (`LocalBypassRules` + `AdminPortsBypassRulesForChain` в `signcraze_policy` pos=1) | HIGH |
| R026-uninstall-bind | `--uninstall` не отвязывал host-policy в startup-config Keenetic → обычный reboot не восстанавливал доступ, требовался factory reset. Mitigated в v1.1.1 (`ndm.UnsetHostPolicy` для всех привязанных хостов до `DeletePolicy`) | CRIT |
| R027-DPI-LAN-coverage | DPI NFQUEUE-цепочка в `mangle POSTROUTING -o $WAN` ловила только sing-box→VPS трафик; LAN не-policy устройства (TV, гости) шли мимо desync. Mitigated в v1.1.0 (`signcraze_dpi_fwd` в `mangle FORWARD` с `-o WAN -m mark ! --mark 0x53`) | MED |
| R028-NDM-rebuild-storm | Пачка из 10+ NDM-событий приводила к 10 параллельным `--reapply` fork/exec, CPU-шторм на slow MIPS. Mitigated в v1.1.1 (netfilter.d hook с `flock -x -n` + pending-маркер + trailing debounce → 2 reapply) | HIGH |
| R029-bcrypt-MIPS-DoS | bcrypt cost=12 на MIPS softfloat: login admin UI 6 c — окно для timing/DoS атак. Mitigated в v1.4.0 (build-tagged `auth_cost_lowmem.go` cost=10 для `GOARCH=mips/mipsle`; login 6 c → 2 c) | LOW |
| R030-PID-reuse | PID-файл мог указывать на чужой процесс после OOM/kill+reuse. Mitigated в v1.4.0 (atomic write + `processAlive` + match `/proc/<pid>/comm` с учётом 15-байт truncation) | MED |
| R031-slow-loris-admin | Admin server (9091) без `ReadHeaderTimeout` уязвим к slow-loris. Mitigated в v1.4.0 (`ReadHeaderTimeout: 15s`) | LOW |
| R032-WS-idle-drop | WebSocket `/api/traffic` соединения за NAT/UPnP падали по idle. Mitigated в v1.4.0 (keepalive ping каждые 30s, RFC 6455 §5.5.2) | LOW |
| R033-xray-geo-silent-fail | xray без `geoip.dat`/`geosite.dat` падал с криптичной ошибкой `failed to load geosite`. Mitigated в v1.4.0 (early-check с подсказкой `--update-geo --core xray`; `GeoAssetsDir=""` контракт = skip для preview/Validator) | MED |
| R034-reproducibility | Build без `-buildid=` и `SOURCE_DATE_EPOCH` → разные хеши при одинаковом коде, нет supply-chain verification. Mitigated в v1.4.0 (reproducible builds + cosign keyless OIDC + SLSA provenance attestation) | MED |

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
- **v1.3.0+ peer-protocols**: при появлении нового supervised peer (naive/mieru/etc) — новая запись в R035 group с указанием upstream-репозитория, asset-pattern и архитектурного ограничения (LE/BE).

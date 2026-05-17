# Changelog

Все значимые изменения проекта фиксируются в этом файле.
Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/).

---

## [1.4.2] — 2026-05-16

### Fixed
- **CI lint+publish**: `NODE_OPTIONS=--no-deprecation` для подавления Node.js 24 warnings в jobs lint, release-publish, attest. Устраняет шум в CI без правки самих actions.

---

## [1.4.1] — 2026-05-16

### Fixed
- **`actions/attest-build-provenance` v2 → v4**: переход на Node.js 24-совместимую версию action. Релизные artefacts продолжают получать SLSA provenance attestation.
- **`golangci-lint-action` pin v9 + golangci-lint v2.12.2**: action v9 не поддерживает golangci-lint v1.x; зафиксирована конкретная v2.12.2 на runner. CI lint job снова зелёный.

---

## [1.4.0] — 2026-05-16

### Added (пост-аудитный hardening)
- **Firewall — iptables-restore batching**: `BatchBuilder` + `RestoreBatch` + `Flush` в `internal/firewall`. `applyPolicy`/`applyFull` собирают пакет правил и применяют через `iptables-restore --noflush` одним вызовом. Сокращение fork/exec 24 → 3 на slow MIPS.
- **WAN cache**: `WANIface` кэшируется, `InvalidateWANCache` при изменении сети. Watchdog tick больше не делает fork на каждое срабатывание.
- **Watchdog REDIRECT/DPI-FORWARD coverage**: проверка восстанавливает не только TPROXY (mangle), но и nat PREROUTING REDIRECT-цепочку и FORWARD DPI-цепочку из v1.1.0.
- **Reproducible builds**: `-buildid=` + `-trimpath` + `SOURCE_DATE_EPOCH` в release.yml.
- **Cosign sign-blob keyless через GitHub OIDC**: `.sig` + `.pem` рядом с каждым артефактом. Раздел [«Verifying the release»](README.md#verifying-the-release) в README.
- **SLSA build provenance**: `actions/attest-build-provenance@v2` → проверка через `gh attestation verify`.
- **`--diag --json`**: machine-parseable output для скриптов и мониторинга. Структурированный JSON по каждому чек-пункту.
- **WebSocket keepalive ping 30s** (RFC 6455 §5.5.2) в `/api/traffic`. Соединения за NAT/UPnP не падают по idle.
- **Cache-Control `immutable`** для embed assets Routing UI / Zashboard; `no-cache` для index.html. Меньше нагрузки на HTTP-сервер.

### Changed
- **bcrypt cost build-tagged**: `auth_cost_lowmem.go` (cost=10) для `GOARCH=mips/mipsle` vs `auth_cost_default.go` (cost=12). Логин в admin UI на slow MIPS: 6 с → 2 с.
- **xray `json.Marshal compact`**: −15% CPU при генерации конфига на MIPS softfloat.
- **sing-box rule-set format**: для URL `sign-craze-dat` ставится `format: "source"` (workflow в апстриме генерит JSON, не binary). Закрывает Known issue v1.0.2.
- **xray geo.dat early-check**: при `--core xray --restart` без `geoip.dat`/`geosite.dat` чёткая ошибка с подсказкой `--update-geo --core xray`. Контракт `GeoAssetsDir=""` = skip для preview/web Validator.
- **PID-файлы через `atomicfs.WriteFileAtomic`**: `processAlive` + match `/proc/<pid>/comm` (PID-reuse guard, учёт 15-байтового truncation).
- **Admin server (9091) `ReadHeaderTimeout` 15s**: защита от slow-loris.
- **hybrid mode**: убран `-m comment` (Keenetic busybox без `xt_comment`).

### Added (тесты, регрессии)
- `TestApplier_Apply_Policy_BypassBeforeTProxy` — регрессия SSH hang от 2026-05-13 (lessons.md).
- `TestService_DefaultShimPath_IsS99` — boot order S99 после S51dropbear.
- `TestRender_Geosite_NoDat_ReturnsError` + `Geoip` + `DatPresent_OK` — xray geo guard.
- `TestFilterUnreferencedRuleSets` (skip) — фикс отложен.
- `FuzzMieruWireProto` — 3 файла corpus.
- **E2E**: `StrictHostKeyChecking=accept-new` (MITM-защита в CI); `trap cleanup INT/TERM` в `scripts/e2e/run.sh` (`sign-craze --stop` при прерывании).

### Geo / DPI
- **SHA256 streaming в geo/srs**: OOM-guard для 30–100 MB `.srs`.
- **IPK + lua streaming**: `atomicfs.WriteFileAtomicFromReader` без полной буферизации.

---

## [1.3.0] — 2026-05-14

### Added
- **naiveproxy integration (klzgrad/naiveproxy)**: supervised peer через process chain. `sign-craze` качает бинарь naive, запускает как daemon на `127.0.0.1:18443`, sing-box подключается socks5-outbound. URL: `naive+https://user:pass@host:port`. См. ADR-0021-naiveproxy-process-chain.
- **Поддерживаемые архитектуры naive**: arm64, arm7, mipsle. mips (big-endian) reject — klzgrad публикует только LE.
- **Supervised-peer scaffolding**: пакет `internal/peer/` (peer interface + mieru core + port allocator). Shared архитектура для process-chain протоколов.
- **`pkg/types`**: `ProtocolNaive` + `NaiveOpts` (5 полей); `ProtocolMieru` + `MieruUsername`/`MieruPassword`/`MieruPort`/`MieruProtocol`/`MieruMultiplexing`/`MieruLocalPort`.
- **`internal/proxyparse`**: схемы `naive+https://`, `naive+quic://`, `mieru://`, `mierus://`.
- **`internal/state`**: `NaiveEnabled` + multi-naive guard + BE-MIPS validation; mieru port range validation.
- **`internal/naiveproxy`**: новый пакет (Download/Extract/Install/Params/Lifecycle) + unit-тесты.
- **`internal/singbox`**: `renderNaive` → socks к `127.0.0.1:NaiveListenPort`; поле `naive_port`.
- **CLI**: `--with-naive` (install + start daemon), `--update-naive` (обновить бинарь). `doStart`/`doStop` хуки. Wizard поддерживает `naive+https://` URL.
- **`internal/core/{xray,mihomo}`**: явный reject naive+mieru с понятной ошибкой (поддерживается только sing-box).
- **`internal/diag`**: peer health check (naive/mieru).
- **`Makefile`**: mieru cross-compile targets под все 4 архитектуры.
- **`go.mod`**: `github.com/ulikunitz/xz` (pure-Go xz decoder для naive tarball).

### Docs
- BEHAVIOR_SPEC §8 (naive), COMPATIBILITY_MATRIX §5.1, TROUBLESHOOTING, CONSTRAINTS обновлены.

### Test status
- 28/28 пакетов PASS, vet clean, gofmt clean, cross-compile arm64/arm7/mipsle/mips OK.

---

## [1.2.0] — 2026-05-13

### Added
- **ANSI-цветной вывод CLI**: собственный мини-врапер в `internal/cli/color.go` без сторонних либ (go.mod остался на 3 пакетах). Семантические хелперы `OK`/`Warn`/`Err`/`Info`/`Hint`/`Header`/`Key` + низкоуровневые `Red`/`Green`/`Yellow`/`Bold`. No-op при выключенном цвете.
- **Полный opt-out** с приоритетами: `--no-color` > `NO_COLOR` > `FORCE_COLOR` > `TERM=dumb` > авто-детект `term.IsTerminal`. Раздельные состояния для stdout/stderr — pipe stdout без цвета, stderr (TTY) со цветом slog логов.
- **Кастомный `colorTextHandler`** в `internal/log/log.go`: DEBUG dim / INFO cyan / WARN yellow / ERROR red+bold. Файловый JSON-лог остался без изменений.
- **Раскрашены команды**: `--status` (running/stopped), `--diag` (PASS/WARN/FAIL), `--core-list` (`*` активного ядра), `--install` (прогресс + ВНИМАНИЕ), `--uninstall` (`printRemoved`), `--start`/`--stop`, `--update`/`--update-geo`/`--update-core`, `--help` (заголовок + ключи + секция глобальных опций).

---

## [1.1.2] — 2026-05-13

### Added
- **Routing UI: светлая тема + переключатель ☀/☾**: CSS variables, persist через localStorage. По умолчанию подхватывается `prefers-color-scheme`.

---

## [1.1.1] — 2026-05-13

### Fixed (защита SSH/admin при policy с 10+ устройствами)
- **Корень**: `applyPolicyMode` не имел bypass для трафика к самому роутеру. Keenetic метит mark всех policy-устройств включая `dst=LAN_IP` — без RETURN пакеты SSH:222 уходили в TPROXY/sing-box, коннект виснул. `doUninstall` не отвязывал host-policy привязки в startup-config → обычный reboot не помогал, требовался factory reset.
- **`LocalBypassRules` + `ensureLANBypass`**: `-d <LAN_IP> -j RETURN` на pos=1 в `signcraze_policy` для TPROXY-real (mangle), REDIRECT (nat), TUN (mangle). LAN-IP автодетект через `netif.DetectLANAddr`.
- **`AdminPortsBypassRulesForChain`**: defense-in-depth для портов 22/222 TCP+UDP в policy chain. `AdminPortsBypassRules` → back-compat обёртка.
- **`ndm.Client`**: `GetHostsWithPolicy` + `UnsetHostPolicy`; `doUninstall` теперь отвязывает все хосты ДО `DeletePolicy` — factory reset больше не нужен.
- **netfilter.d hook**: `flock -x -n` + pending-маркер + trailing debounce. Пачка из 10 NDM-событий = 2 reapply вместо 10 параллельных fork/exec, нет CPU-шторма на slow MIPS.
- **Docs**: BEHAVIOR_SPEC §3a (порядок правил `signcraze_policy`) и §4 (uninstall отвязка хостов).

---

## [1.1.0] — 2026-05-12

### Refactored (DPI chain в FORWARD)
- **Архитектурный fix**: NFQUEUE-цепочка `signcraze_policy_dpi` висела в `mangle POSTROUTING -o $WAN` и ловила только исходящий трафик sing-box к VPS. LAN не-policy устройства (TV, гости, IoT) шли напрямую через ISP без DPI-обхода — YouTube/Discord на них не работали.
- **Новая архитектура**: `DPIForwardChainName "signcraze_dpi_fwd"` в `mangle FORWARD`. Jump из FORWARD `-o $WAN_IFACE -m mark ! --mark 0x53`. LAN policy-устройства перехватываются TPROXY до FORWARD (не попадают в DPI). LAN не-policy идут через FORWARD → DPI → nfqws2 hostlist desync. Self-traffic sing-box (`SO_MARK=0x53`) отсекается mark-фильтром.
- **Cleanup legacy**: `LegacyPolicyDPIChainName` сохранён для cleanup при апгрейде; `applier.Remove` чистит и новый FORWARD jump и legacy POSTROUTING.
- **Tests**: `policy_dpi_test.go` переписаны (FORWARD, `! --mark`, port coverage); `applier_test.go` — 2 новых теста (FORWARD chain + legacy cleanup).
- **E2E на KN-1810**: TV открыл YouTube через nfqws2 desync.

### Added (Routing UI)
- **Пресеты получили два режима**: `+ Добавить` (AS-IS, аддитивно) и `⟳ Заменить` (TO-BE, очистить Rules + force-set Final; RuleSets сохраняются по решению пользователя).
- **Клиентский буфер**: изменения routing (Rules/RuleSets/Final) буферизуются в браузере. Action-bar `Сохранить`/`Отмена` под таблицей при `isDirty`.
- **Новые endpoints**: `POST /api/presets/{name}/preview` (на диск не пишет), `PUT /api/routing` (атомарная замена routing-секции; Inbounds/Outbounds сохраняются нетронутыми), `PUT /api/final`.
- **Apply при unsaved** → confirm + оранжевая подсветка + dirty-dot. `beforeunload` предупреждение при закрытии вкладки с черновиком.
- **+12 backend-тестов**: preview (add/replace/notfound/412/no-disk-write) + commit (write/preserve/validation).

---

## [1.0.2] — 2026-05-12

### Added
- **`firewall.Config.TUNDeviceNameOverride` + `Config.TUNDevice()` метод**: переопределение имени TUN-интерфейса с fallback на default const `TUNDeviceName` ("signbox-tun"). Поле зарезервировано на случай будущей поддержки xray-TUN/mihomo-TUN с собственным именем интерфейса. Использовано в `applier.Remove` (FORWARD ACCEPT cleanup, DeleteTUNRoute).

### Changed
- **`internal/web/clash.go`**: константа `singboxClashAPIAddr` → `coreClashAPIAddr`. Reverse-proxy теперь корректно описан как универсальный (sing-box experimental.clash_api и mihomo external-controller оба слушают 127.0.0.1:9094). Комментарии и `ErrorHandler` сообщение уточнены: xray не имеет Clash-API, UI деградирует gracefully на 502.

### Verified (live тестирование 2026-05-12 на KN-1810 mipsle)
- **Phase 8 (CLI commands)**: `--version`, `--diag` 11 PASS + 1 WARN (geo-files без `.srs`), `--status`, `--backup` (`/opt/var/lib/sign-craze/backups/backup-2026-05-12T*.tar.gz`), `--core-list` (sing-box active + xray installed), `--port-list`, `--exclude-list`, init.d `S99signcraze` после `S51dropbear`, ndm netfilter hook `/opt/etc/ndm/netfilter.d/50-sign-craze`.
- **Phase 9.8 (policy mode)**: RCI `/rci/show/ip/policy` показывает `sign-craze` policy (mark=0xffffaab, table4=4098). state.policy_mark=268434091 (=0xffffaab) совпадает с RCI. `iptables -t mangle -L signcraze_policy` ремаркирует Keenetic-mark 0xffffaab → loopMark 0x53 + TPROXY redirect 127.0.0.1:7895. RPi4 (172.16.0.97, MAC dc:a6:32:35:4d:10, bound к policy через Keenetic Web UI «Приоритеты подключений») `curl ifconfig.me` = `167.17.177.54` (= IP homelabcloud.ru VPS) ≠ Keenetic WAN. Sign-box log: `outbound/vless[vless-grpc-craze]: outbound connection to <ip>:443`.
- **Phase 10 (DPI selective)**: `--dpi on` + `--dpi-targets discord.com,youtube.com,googlevideo.com` + `--restart` → `/opt/etc/sign-craze/dpi-hostlist.txt` содержит 3 домена. nfqws2 cmdline содержит `--hostlist=/opt/etc/sign-craze/dpi-hostlist.txt`. iptables `signcraze_policy_dpi`: NFQUEUE TCP 443=456K packets / TCP 80=15K / UDP 443 QUIC=48 packets. VPN exclude: RETURN на 167.17.177.54 (= homelabcloud.ru) 66623 packets — Reality-маскировка sing-box не десинкается. Возврат: `--dpi off` + `--dpi-targets clear` + `--restart` → nfqws2 stopped.
- **Phase D.1 (VLESS+Reality+spx render)**: URL `vless://...?security=reality&pbk=...&sid=...&spx=%2F` → `ParseCanonical` → `state.outbounds[0].tls.reality.spider_x: "/"`. `sing-box check -c /opt/etc/sign-craze/config.json` exit 0. Render корректно опускает spider_x из `tls.reality` (sing-box игнорирует, xray-only фича). Live трафик: 35131 TCP + 1131 UDP packets через `signcraze_policy` chain за тестовую сессию.
- **Phase reboot (autostart)**: `reboot` → ~2min cold boot → ssh 222 доступен сразу после S51dropbear → `sign-craze --status` показывает sing-box pid 749 (low pid = ранний автостарт через S99signcraze) → `signcraze_policy` принимает 65 TCP + 9 UDP packets за 2 минуты uptime → RPi4 ifconfig.me = VPS IP. S99 ПОСЛЕ S51dropbear подтверждён: SSH 222 поднимается независимо от sign-craze (lesson 2026-05-07 регрессии нет).

### Known issues (обнаружены в live test, fix отложен)
- **`internal/singbox/config.go::buildRuleSets` устанавливает `format: "binary"`**: репо `sign-craze-dat` workflow генерит JSON source под `.srs` расширением → sing-box FATAL `invalid sing-box rule-set file`. Unused rule_sets (объявлены, но не использованы в `rules[]`) всё равно качаются на старте. Workaround: вручную удалить unused блоки из `routing.json`. Полный fix: либо `format: "source"` для sign-craze-dat URL'ов в render, либо `update_interval: "never"` для unused, либо compiled `.srs` через `sing-box rule-set compile` в workflow `sign-craze-dat`.
- **`internal/core/xray/render.go` ссылается на `geosite:category-ads-all`**: xray FATAL `failed to load geosite: CATEGORY-ADS-ALL > open /opt/sbin/geosite.dat: no such file or directory`. `internal/cli/cmd_core.go::handleCoreInstall` не качает `geosite.dat`/`geoip.dat` для xray (хотя `internal/geo/dat.go` реализован в B.5). Workaround: использовать sing-box. Полный fix: integrate dat-download в `--core-install xray`/`--core xray --restart` или guard geosite-rule без `.dat` в xray render.
- **`internal/cli/cmd_lifecycle.go::doStop`**: при core-switch не убивает процессы предыдущего ядра — наблюдался orphan sing-box pid после `--core xray --restart` fail. Fix: stop все зарегистрированные ядра (не только active state.Core) в doStop.

---

## [1.0.0] — 2026-05-12

### Added
- **Унификация routing для xray и mihomo**: web UI на порту 9092 теперь принимает inbound/outbound/rules/rule_sets/presets для любого активного ядра без ошибок. Apply (`POST /api/apply`) регенерирует конфиг активного ядра (`/opt/etc/sign-craze/xray/config.json` или `mihomo/config.yaml`), не только sing-box.
- **`types.CoreRenderParams.RoutingConfig`**: новые поля `RoutingConfig`, `InboundMode`, `DefaultOutboundTag`. Все три ядра консумируют единый `pkg/types.RoutingConfig` и переводят в свою натив-форму через адаптер.
- **Xray translation `RuleSet → geosite:/geoip: matcher`**: префиксы `geosite-`/`geoip-` в `RouteRule.RuleSet` автоматически конвертируются в xray `domain: ["geosite:X"]` / `ip: ["geoip:X"]` matchers. Xray использует встроенный geosite.dat/geoip.dat, отдельный rule_set entry не создаётся.
- **Mihomo `rule-providers`**: `RuleSets` с `.mrs` URL транслируются в mihomo `rule-providers:` секцию + `RULE-SET,tag,action` строки.
- **Per-core preset URLs**: `ruleSetSources` translation table в `routingui_presets.go`. При `apiPresetsApply` URL резолвится под `c.GeoFormat()`: sing-box → `.srs` (SagerNet/sing-*), mihomo → `.mrs` (MetaCubeX/meta-rules-dat), xray → translation в matcher (URL не нужен).
- **Validator с warnings**: `RoutingUIDeps.Validator` callback ловит несовместимости (TUN inbound на xray/mihomo, `.srs` URL на mihomo, refilter rule_set на xray) и surface их в `apiValidate` response как `Warnings[]`. Не блокирует apply.
- **`apiPreview` Content-Type**: автодетект формата активного ядра — `application/yaml` для mihomo, `application/json` для sing-box/xray.
- **e2e тесты multi-core**: `TestE2E_FullFlow_Xray`, `TestE2E_FullFlow_Mihomo`, `TestE2E_PresetApply_PerCore`, `TestE2E_Validate_Warnings_XrayTunInbound` в `internal/web/routingui_multicore_test.go`.
- **`TestValidCoresMatchesRegistry`**: ловит drift между `state.ValidCores` и `core.Names()` (registry).

### Changed
- **`cmd_lifecycle.go` унификация**: убрано ветвление `if c.Name() == "sing-box"`. Все ядра идут через единый `ensureConfigFreshForCore(ctx, c, st)` который читает `routing.json` и зовёт `c.RenderConfig`. TUN-cleanup инкапсулирован в helper `needsTUN(c, st)` (sing-box+TUN inbound only).
- **`regenerateConfig` core-aware**: пишет в `c.ConfigPath()`, не в захардкоженный `/opt/etc/sign-craze/config.json`. Apply через UI теперь корректно обновляет конфиг активного ядра.
- **`state.ValidCores`**: package var вместо хардкода в switch. Тест `cores_sync_test.go` ловит drift с registry.
- **`probeTProxyKernel()`**: вынесен из `cli/deps.go` в `internal/singbox/probe.go` (singbox-specific, не общий слой).

### Fixed
- **Xray/mihomo игнорировали routing.json**: при `--start` с активным xray/mihomo `renderAndWriteConfig` использовал только `st.Outbounds`, не загружая `routing.json`. Все пользовательские rules/inbounds/outbounds из UI пропадали при перезапуске. Теперь `ensureConfigFreshForCore` читает routing.json и прокидывает в `CoreRenderParams.RoutingConfig`.
- **Apply через 9092 был бесполезен для xray/mihomo**: `OnApply → regenerateConfig → singbox.WriteConfig(/opt/etc/sign-craze/config.json)` писал в sing-box путь даже когда активное ядро xray. Apply не обновлял конфиг работающего xray, нужен был ручной `--restart`.
- **Preview на mihomo отдавал JSON**: hardcoded `Content-Type: application/json` в `apiPreview` ломал UI-парсер для YAML конфига mihomo.
- **Xray render фейлил на direct/block outbound**: при добавлении direct outbound через UI xray.Render возвращал ошибку "canonical отсутствует". Теперь `direct`/`block` пропускаются в loop, добавляются автоматически через `freedom`/`blackhole`.

### Migration
- **Routing.json остаётся core-agnostic**: при переключении `--core` существующий `routing.json` сохраняется. Несовместимые конструкции (`.srs` URL на mihomo, refilter rule_set на xray) surfaced через `apiValidate` warnings — пользователь видит их в UI и применяет preset заново.
- **`renderAndWriteConfig` удалена** в `internal/cli/cmd_lifecycle.go`: используйте `ensureConfigFreshForCore` (в `deps.go`) — она работает для всех ядер.
- **Downgrade с xray/mihomo на старый бинарь**: если бинарь не знает xray, `core.Active("xray")` вернёт ошибку → `doStart` падает с clear message. Перед downgrade выполните `sign-craze --core sing-box --restart`.

---

## [0.8.3] — 2026-05-09

### Fixed
- **Миграция routing.json: legacy TUN inbound → автогенерация TPROXY**: на установках v0.6.x → v0.8.x с `state.inbound=tproxy` файл `routing.json` мог содержать TUN-inbound от bootstrap (`auto_route:true, stack:system, signbox-tun`). `Render()` видел `RoutingConfig.Inbounds` непустым → пропускал TPROXY-инъекцию → sing-box стартовал с TUN inbound, а iptables направляли marked-трафик LAN-клиентов в `127.0.0.1:7895` (где никто не слушал) → reset, нет интернета. Теперь `configParamsFromState()` фильтрует TUN-inbound при `state.inbound=tproxy` и пересохраняет routing.json.
- **Verify**: `netstat -tlnp | grep 7895` после `--restart` должен показать `sing-box` LISTEN (не пусто). Если пусто — ручной фикс: `jq 'del(.inbounds)' /opt/etc/sign-craze/routing.json > /tmp/r && mv /tmp/r /opt/etc/sign-craze/routing.json && sign-craze --restart`.

## [0.8.2] — 2026-05-09

### Fixed
- **Hostlist применяется при auto-update без явных `DPITargets`**: ранее `--hostlist=<path>` ставился только при непустом `state.dpi_targets`. Теперь подключается и если файл `/opt/etc/sign-craze/dpi-hostlist.txt` существует на диске (создан auto-update'ом). Без фикса selective-DPI после `--dpi-update-urls` не работал.
- **`DefaultUpdateURLs` исправлены** на актуальные пути: `Flowseal/zapret-discord-youtube/lists/list-general.txt` и `list-google.txt` (не существующие `list-discord.txt`/`list-youtube.txt`). `bol-van/zapret` исключён из дефолтов — в master нет static hostlist, всё генерируется shell-скриптами.

## [0.8.1] — 2026-05-09

### Fixed
- **Resilient DNS resolver для UpdateHostlist**: Keenetic + DNSCrypt-Entware на `127.0.0.1:53` могут фильтровать `raw.githubusercontent.com` (наблюдалось при v0.8.0 deploy). UpdateHostlist теперь использует `net.Resolver` с custom Dial: системный nameserver → fallback `1.1.1.1:53` → `9.9.9.9:53` → `8.8.8.8:53`. Применимо ТОЛЬКО к auto-update; обычный DNS sign-craze идёт через системный resolver.

## [0.8.0] — 2026-05-09

### Added
- **WAN-фильтр в DPI-правилах**: `PolicyDPIRules` принимает `wanIface` — jump POSTROUTING получает `-o $WAN_IFACE`, NFQUEUE ловит только трафик через WAN.
- **`state.dpi_exclude_ips`**: IP-адреса, исключаемые из nfqws2-десинка (RETURN-правила перед NFQUEUE). Защищает Reality VPN handshakes к собственному серверу и downstream-VPN-клиентов; передаётся в `PolicyDPIRules` как `vpnExcludeIPs`.
- **Auto-update hostlist**: `state.dpi_update_urls` + `state.dpi_update_interval_hours` + `state.dpi_last_update` — watchdog goroutine качает список хостов каждые N часов из upstream (bol-van/zapret, Flowseal/zapret-discord-youtube), парсит hosts/Adblock формат, atomic write в `/opt/etc/sign-craze/dpi-hostlist.txt`.
- **Новые CLI-флаги**: `--dpi-exclude-ips`, `--dpi-exclude-ips-list`, `--dpi-update-urls`, `--dpi-update-interval`, `--dpi-update-now`.

### Changed
- **`--reapply` использует `Reconcile` вместо `Apply`**: throttle 5 секунд через mtime-маркер `/opt/var/run/sign-craze-reapply.last`. Ранее NDM netfilter.d hook вызывал Apply пачкой 5–10 раз/сек (263 reapply/час в production).
- **WAN автодетект перенесён в `applyInternal` до `switch mode`**: ранее WAN заполнялся в `ensureWANUIDrop` после DPI-rules — DPI-jump получал пустую строку.

---

## [0.7.1] — 2026-05-08

### Added
- **Реальный TPROXY для проксирования UDP**: ранее (v0.7.0) использовался REDIRECT fallback (TCP-only), потому что было ошибочное предположение, что `xt_TPROXY` отсутствует в kernel Keenetic 4.9. На самом деле модули `xt_TPROXY.ko` и `xt_socket.ko` физически существуют в `/lib/modules/4.9-ndm-5/`, просто не загружены — `modules.dep` отсутствует, `modprobe` не работает, нужен явный `insmod` с путём.

  Добавлен helper `firewall.EnsureKernelModule(name, path)` в `internal/firewall/module.go` — идемпотентная загрузка через `insmod`, проверка `/proc/modules`, fallback gracefully. `applier.applyPolicyMode()` пробует `insmod xt_socket` + `xt_TPROXY` при `UseTProxy=true`; при успехе использует `mangle:PREROUTING -j TPROXY --on-port 7895 --on-ip 127.0.0.1 --tproxy-mark 0x53` (`PolicyTProxyRulesReal`) для TCP+UDP и `EnsureLocalRoute` для доставки. При неудаче `insmod` — fallback на REDIRECT (TCP-only) с warning. Sing-box config рендерится с `Type: "tproxy"` (TCP+UDP в одном inbound, listen `::`) или `Type: "redirect"` (TCP-only fallback) в зависимости от `TProxyKernelOK`.

  Проверено на Keenetic 4.9 mipsle (kernel 4.9-ndm-5): `lsmod` показывает `xt_TPROXY 4911 2 - Live` после `--restart`, `iptables -t mangle -nvL signcraze_policy` — TPROXY правила TCP+UDP с реальным трафиком (92 TCP, 11 UDP packets за минуту), sing-box log: `inbound/tproxy[tproxy-in]: inbound connection from 172.16.0.97:50062 to 149.154.166.110:443` — оригинальный src/dst клиента сохранены через TPROXY, VLESS outbound за 48ms. Self-test через mark `0xffffaab` с роутера `curl https://1.1.1.1` → `HTTP=301 TIME=0.46s` (Cloudflare).

---

## [0.7.0] — 2026-05-08

### Added
- **`state.Inbound: "tun" | "tproxy"`** (default `"tproxy"`): поле в состоянии определяет режим inbound sing-box. Флаг `--inbound` доступен во всех `--install*`-командах. Migration: пустое значение автоматически приводится к `"tproxy"`.

### Fixed
- **Переход с TUN inbound на REDIRECT inbound для прокси LAN-трафика**: TUN+gvisor-стек делал userspace-SNAT клиентского src → TUN gateway (`172.19.0.1`), sing-box видел `inbound connection from 172.19.0.1:PORT` вместо реального IP клиента. Ответы от прокси возвращались router-локально (`dst=172.19.0.1`) и до клиента не доходили — счётчик `iptables FORWARD signbox-tun *` (out) был 0. `xt_TPROXY` на Keenetic 4.9 отсутствует, поэтому выбран fallback в REDIRECT (DNAT в local stack). Sing-box `redirect` inbound на `0.0.0.0:7895`; iptables `nat/PREROUTING -j signcraze_policy` с `REDIRECT --to-port 7895`. Через `SO_ORIGINAL_DST` извлекается оригинальный dst, src клиента preserved. Включено `net.ipv4.conf.all.route_localnet=1` для прохождения REDIRECT'ed пакетов, INPUT ACCEPT на порт sing-box. Проверено: RPi4 → `curl https://1.1.1.1` проходит полный TLS handshake (Certificate, CERT verify, Finished, HTTP 200), conntrack показывает 14 sent / 12 reply пакетов вместо `[UNREPLIED]`. Ограничение: REDIRECT работает только TCP — UDP трафик идёт мимо прокси (`redirect` inbound TCP-only).

---

## [0.6.6] — 2026-05-08

### Fixed
- **TCP MSS clamp на FORWARD к/от signbox-tun**: ServerHello + ServerCertificate (10–16 KB cert chain) не доставлялись LAN-клиентам при HTTPS-хендшейке через прокси. Причина: MTU signbox-tun = 1280, LAN-клиенты согласуют MSS=1460 (MTU 1500) — крупные сегменты от удалённого сервера не вмещались в TUN; IP-фрагментация ломалась, клиент получал «TLS handshake timeout» сразу после ClientHello. ICMP PMTUD на Keenetic LAN ненадёжен. Решение: два правила `mangle/FORWARD` с `--clamp-mss-to-pmtu` — симметрично для SYN-сегментов на `-o signbox-tun` и `-i signbox-tun`. После clamp'а клиенты согласуют MSS=1220, ServerCertificate приходит без фрагментации. Фикс в `internal/firewall/modes/policy.go` (два дополнительных RuleSpec в `PolicyRules()`). Проверено на Keenetic 4.9 mipsle: счётчик `iptables FORWARD signbox-tun *` (out) растёт с 0 до 232 пакетов/мин, RPi4 → `curl https://1.1.1.1` отрабатывает.

---

## [0.6.5] — 2026-05-08

### Fixed
- **UPX-стаб сегфолтит CLI на Keenetic 4.9 mipsle**: бинарь `sign-craze` для `mipsle` упаковывался через `upx --lzma`, что приводило к SIGSEGV при любом интерактивном вызове (`--version`, `--status`, `--help`) на Keenetic 4.9-ndm-5. Daemon-режим (`--service-watchdog`, `--ui-daemon`) без TTY работал — падал только CLI. Гипотетическая причина — `term.IsTerminal(os.Stderr.Fd())` в `internal/log/log.go` крашит UPX-стаб на mipsle. Исправление: в `.github/workflows/release.yml` добавлено поле `upx` в build matrix — UPX применяется только к `arm64` и `arm7`; для `mipsle` и `mips` бинари распространяются несжатыми (~13 MB). Аналогично обновлён `Makefile` target `upx`.

---

## [0.5.16] — 2026-05-07

### Added
- Авто-определение ядра (sing-box / xray / mihomo) по proxy URL без явного флага `--core`.
- Lifecycle dispatch: команды `start` / `stop` / `restart` маршрутизируются к нужному ядру автоматически.

### Changed
- Обновлена документация: рецепт «РФ direct, остальное VPN» добавлен в ROUTING / wiki / README.

---

## [0.5.15] — 2026-05-07

### Fixed
- sing-box: задан явный путь к файлу кэша — `/opt/var/lib/sign-craze/cache.db`. Устраняет ситуацию, когда sing-box создавал `cache.db` в текущем рабочем каталоге.

---

## [0.5.14] — 2026-05-07

### Fixed
- **Keenetic cold boot**: `TUNAttachTimeout` увеличен с 30 с до 90 с. Устраняет отказ старта на роутерах с медленной инициализацией сетевого стека.

---

## [0.5.13] — 2026-05-07

### Fixed
- **Keenetic cold boot**: добавлен retry для `--service-start` при позднем появлении модуля `xt_NFQUEUE`. Сервис больше не падает при холодном старте, если ядро загружает NFQUEUE-модуль с задержкой.

---

## [0.5.12] — 2026-05-07

### Fixed
- **Keenetic init.d shim**: приоритет запуска изменён с `S05signcraze` на `S99signcraze`. Гарантирует старт службы после `S51dropbear` (Entware SSH 222) и устраняет lockout при сбое sign-craze в boot-цепочке.
- DPI: расширен список NFQUEUE-портов — добавлены голосовые порты Discord и CDN YouTube/Discord (POSTROUTING NFQUEUE + lua-расширения nfqws2).
- DPI: исправлен запуск nfqws2 в runtime; YouTube и Discord работают out-of-box после `--dpi on`.

### Removed
- CLI: флаг `--purge` объединён с `--uninstall` (отдельная команда удалена).

---

## [0.5.11] — 2026-05-07

### Fixed
- sing-box: миграция устаревших полей конфигурации под sing-box ≥ 1.12. Устраняет ошибки парсинга при старте на новых версиях ядра.

### Added
- Реализована логика команды `lifecycle` и добавлены unit-тесты для UI-слоя.

---

## [0.5.10] — 2026-05-07

### Fixed
- sing-box: удалено поле `store_mode` из секции `cache_file` — sing-box ≥ 1.10 убрал это поле, конфиг не проходил валидацию.

---

## [0.5.9] — 2026-05-07

### Fixed
- CI: очистка `dist/` перед сборкой и явные пути загрузки артефактов. Устраняет повреждение бинарей из-за stale-файлов в архиве релиза.

---

## [0.5.8] — 2026-05-07

### Fixed
- sing-box: поле `cache_file` перенесено в секцию `experimental.cache_file` согласно требованиям sing-box ≥ 1.8.0. Без фикса sing-box отказывался стартовать на актуальных версиях.

---

## [0.5.7] — 2026-05-07

### Fixed
- Установщик: probe с флагом `-L` (follow redirects) для проверки доступности бинарей; добавлено зеркало `gh-proxy.com` вместо устаревших `raw/dist` URL.

---

## [0.5.6] — 2026-05-07

### Fixed
- Установщик: добавлены таймауты и probe-проверка перед скачиванием; автоматический фолбэк на raw-URL при недоступности GitHub Releases (актуально из РФ).

---

## [0.5.5] — 2026-05-07

### Added
- Добавлена директория `dist/` с бинарями для прямой загрузки через raw.githubusercontent.com (резервный канал при блокировке Releases).

---

## [0.5.4] — 2026-05-07

### Fixed
- Web UI: исправлен admin UI на порту 9092 (Routing Editor SPA не открывался после перезапуска).
- Web UI: исправлен Zashboard на порту 9090 — некорректный proxy-адрес приводил к ошибке подключения.
- CI / lint: обновлены GitHub Actions, подавлены Node.js 20 deprecation warnings (DEP0040/DEP0169), исправлены 12 lint-замечаний и падение sing-box golden-теста.

---

## [0.5.3] — 2026-05-06

### Cleanup (post-v0.5.2)

- Удалён мёртвый код: `basicAuth`, `originGuard`, устаревшие комментарии и строки про порт «9090» из web-слоя.
- Починены stale-комментарии в `internal/cli/cmd_ui.go` и сопутствующих файлах.

### Web UI

- **Zashboard возвращён на порт 9090** (LAN-only). Порты:
  - `9090` — Zashboard (Clash-совместимый dashboard, управление прокси и мониторинг трафика)
  - `9091` — admin REST API sign-craze
  - `9092` — Routing Editor SPA (Preact)
- Доступ к Zashboard ограничен локальной сетью: iptables DROP TCP/UDP 9090 на WAN-интерфейсе через chain `signcraze_local`.
- Аутентификация не требуется (basic auth удалён в v0.5.2).

### CI

- Добавлен job `singbox-check`: golden canonical sing-box конфиги валидируются `sing-box check`.
- Добавлен job `firewall-integration`: прогон `internal/firewall/integration_test.go` в Docker `--privileged`.
- Добавлен job `xray-check`: golden xray конфиги валидируются `xray test`.
- Добавлен job `mihomo-check`: golden mihomo YAML валидируются `mihomo -t`.

### E2E

- Добавлены скрипты оркестрованного E2E-прогона на Keenetic: `scripts/e2e/{00-build,01-install,02-policy,03-dpi-hostlist,04-reboot,99-uninstall,run}.sh`.
- Прогон на KN-1810 / Keenetic 5.0.4: `--install-auto → --start → --status` зелёный, RPi-клиент в `sign-craze` policy, трафик идёт через `signbox-tun` (fwmark `0xffffaab → 0x53 → table 83`).

### Bug fixes (выявлены при E2E на KN-1810)

- `singbox`: устранён дубликат outbound `direct` в сгенерированном `config.json` — `sing-box check` падал с `duplicate outbound/endpoint tag: direct` сразу после `--install-auto`. Шаблон `tun.json.tmpl` перестал безусловно добавлять auto-direct, если он уже описан в `state.outbounds`.
- `singbox`: пропуск DNS `detour: direct` в шаблоне, когда `Final == "direct"`. Без фикса sing-box падал на старте: `start dns/tls[remote]: detour to an empty direct outbound makes no sense`. Затрагивает свежие установки до настройки реального outbound.
- `dpi download`: репозиторий nfqws2-keenetic сменил owner с `bol-van` на `nfqws`. GitHub API возвращал 404 при попытке скачать nfqws2 через `--dpi on`. Обновлён owner (`nfqws/nfqws2-keenetic`) и asset-паттерны: Entware `.ipk` вместо `.tar.gz` (формат изменился начиная с v1.1.5). Добавлена поддержка распаковки `.ipk` (outer tar.gz → `data.tar.gz` → бинарь). E2E на KN-1810 подтверждён: nfqws2 v1.1.5 mipsel-3.4 скачан и установлен без ошибок.

### Known issues

- E2E xray/mihomo на роутере (Phase 12 B.10/C.10) — отдельный hardware-цикл.

---

## [0.5.2] — 2026-05-05

- Удалён basic auth и 9090-сервер Zashboard (впоследствии возвращён в v0.5.3).
- Убраны ссылки на Zashboard/9090 из документации и исходного кода.

## [0.5.1] — 2026-05-05

- Исправлены замечания golangci-lint после релиза v0.5.0.

## [0.5.0] — 2026-05-05

- Добавлена абстракция multi-core: `sing-box`, `xray`, `mihomo` через единый интерфейс.
- Исправлен `--uninstall`: корректное завершение watchdog-демона, разграничение degraded-установки.
- Обновлена и стандартизирована документация (README, wiki).

---

## [0.4.2] — 2026-05-05

- Исправлена проверка `Status()` для `--ui` по пути.
- Разблокированы порты 9090/9092 + демонизация `--ui on|off`.
- Синхронизирован wiki с режимами policy/full, selective DPI, watchdog.
- Исправлен watchdog: переживает ребут роутера через init.d shim.
- **feat**: Selective DPI hostlist + firewall watchdog.

## [0.4.1] — 2026-05-05

- Централизовано управление web-ассетами, консолидирована логика API-роутинга.
- Исправлены замечания golangci-lint.

## [0.4.0] — 2026-05-04

- **feat(web)**: Routing Editor UI на `:9092` с REST API + Preact SPA.

---

## [0.3.10] — 2026-05-04

- Улучшена безопасность: TLS-верификация в установщике, убраны personal tasks из tracking.
- Исправлено: отключение Keenetic FASTNAT/FASTROUTE для прокси-трафика.
- Рефакторинг core data structures и CLI lifecycle.

## [0.3.9] — 2026-05-04

- Исправления firewall: разрешён forward to/from `signbox-tun`, устранён embedded field selector в тестах.

## [0.3.8] — 2026-05-04

- Исправление: снижен MTU TUN до 1280.
- Исправление: переключён TUN стек на gvisor.

## [0.3.7] — 2026-05-04

- Добавлен инвариант DNS hijack (tun-in inbound constraint).
- Исправления NDM, netfilter, iptables, route.

## [0.3.6] — 2026-05-04

- Исправления: robust TUN attach на медленном MIPS, idempotent local route, PATH для Entware.

## [0.3.5] — 2026-05-01

- Рефакторинг: обновлён GitHub downloader, уточнён test roadmap.

## [0.3.4] — 2026-05-01

- Реализован NDM policy management client (Keenetic RCI integration).

## [0.3.3] — 2026-05-01

- Исправлен lifecycle сервиса: processAlive игнорирует zombie-процессы.
- Обновлена CI-конфигурация golangci-lint.

## [0.3.2] — 2026-05-01

- Web UI: WebSocket coverage, флаги сборки.

## [0.3.1] — 2026-05-01

- Workflow: автосинхронизация wiki.

## [0.3.0] — 2026-04-30

- **breaking**: Замена TPROXY → TUN-режим для совместимости со стоковым ядром Keenetic (нет `xt_TPROXY`).
- Preflight-проверка iptables-модулей перед применением правил.
- Убрана зависимость от `xt_comment`.

---

## [0.2.x] — 2026-04-28..2026-04-30

Серия патч-релизов v0.2.0–v0.2.18 (28–30 апреля 2026):

- Phase D/F: sing-box конфиги, Web UI (admin API + auth), WebSocket, backup/restore, diag.
- Реализован `--install-auto`, drop manual outbound wizard.
- NDM: принятие Keenetic `route`-ключа в `/show/ip/route`.
- Исправления в service lifecycle, firewall modes, geo, locks, selfupdate, CLI smoke.

---

## [0.1.x] — 2026-04-27..2026-04-29

- `v0.1.0-alpha.1` (2026-04-27) — первый тег, scaffold: go.mod, Makefile, CI, BEHAVIOR_SPEC.md.
- `v0.1.0` (2026-04-28) — Phase 0–3 завершены: CLI scaffold, sing-box download/install, iptables/ipset (TPROXY-режим), service shim.
- `v0.1.2` (2026-04-29) — Исправления lint (nolint errcheck, golangci-lint v2 migration).

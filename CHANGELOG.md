# Changelog

Все значимые изменения проекта фиксируются в этом файле.
Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/).

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

### Known issues

- `--dpi on` падает с `GitHub API вернул 404` для `bol-van/nfqws2-keenetic`: репозиторий-источник недоступен или переименован. DPI E2E (Phase 10) откладывается до 0.5.4 и поиска актуального upstream nfqws2.
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

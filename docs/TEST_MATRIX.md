# Матрица тестового покрытия sign-craze

> Навигационный документ. Детальные сценарии — в `tasks/test-roadmap.md`.
> Здесь: **что покрыто, что нет, как запускать**.

---

## 1. Обзор типов тестов

| Тип                | Где запускается                                | Build tag       | Инструмент                                                    |
|--------------------|------------------------------------------------|-----------------|---------------------------------------------------------------|
| **Unit**           | локально / CI                                  | — (без тега)    | `go test -race ./...`                                         |
| **Integration**    | [Docker](https://docs.docker.com/) `--privileged` / локально с NET_ADMIN   | `integration`   | `go test -tags=integration ./internal/firewall/...`           |
| **E2E hardware**   | [Keenetic](https://help.keenetic.com/hc/ru) KN-1810 (mipsle, живое железо)        | —               | вручную по `tasks/test-roadmap.md`                            |
| **Manual smoke**   | SSH на роутере                                 | —               | CLI-команды из `tasks/test-roadmap.md`                        |

**Политика TDD** (из `.claude/CLAUDE.md`): тест пишется до реализации; интеграционные тесты для firewall-логики используют Docker `--privileged` или network namespace. Все юнит-тесты обязаны проходить без root.

---

## 2. Матрица покрытия

| Подсистема                  | Unit | Integration | E2E HW | Manual smoke | Ключевые файлы                                                                                              |
|-----------------------------|:----:|:-----------:|:------:|:------------:|-------------------------------------------------------------------------------------------------------------|
| `internal/atomicfs`         | ✅   | ❌          | ❌      | ❌            | `atomicfs/atomicfs_test.go`                                                                                 |
| `internal/backup`           | ✅   | ❌          | ❌      | ✅            | `backup/backup_test.go`, `backup/helper_test.go`                                                            |
| `internal/cli`              | ✅   | ❌          | ❌      | ✅            | `cli/smoke_test.go`, `cli/dispatch_test.go`, `cli/cmd_reapply_test.go`, `cli/color_test.go`, `cli/cores_sync_test.go`, `cli/cmd_ports_test.go`, `cli/cmd_update_test.go`, `cli/cmd_core_test.go` |
| `internal/diag`             | ✅   | ❌          | ⚠️      | ✅            | `diag/diag_test.go`                                                                                         |
| `internal/dpi`              | ✅   | ❌          | ⚠️      | ✅            | `dpi/config_test.go`, `dpi/lifecycle_test.go`, `dpi/download_test.go`, `dpi/install_test.go`, `dpi/update_test.go`, `dpi/presets_test.go` |
| `internal/exectx`           | ✅   | ❌          | ❌      | ❌            | `exectx/exec_test.go`                                                                                       |
| `internal/firewall`         | ✅   | ✅          | ⚠️      | ✅            | `firewall/{applier,iptables,ipset,ipset_persist,route,preflight}_test.go`, `firewall/integration_test.go`   |
| `internal/firewall/modes`   | ✅   | ❌          | ⚠️      | ✅            | `modes/{tproxy,hybrid,excludes,ports,policy_dpi,policy_tproxy}_test.go`                                     |
| `internal/geo`              | ✅   | ❌          | ⚠️      | ✅            | `geo/{srs,ipset,decompile,dat,mrs}_test.go`                                                                 |
| `internal/ghrelease`        | ✅   | ❌          | ❌      | ❌            | `ghrelease/downloader_test.go`                                                                              |
| `internal/locks`            | ✅   | ❌          | ⚠️      | ✅            | `locks/file_test.go`                                                                                        |
| `internal/log`              | ✅   | ❌          | ❌      | ❌            | `log/log_test.go`                                                                                           |
| `internal/ndm`              | ✅   | ❌          | ✅      | ✅            | `ndm/policy_test.go`, `ndm/wan_test.go`                                                                     |
| `internal/proxyparse`       | ✅   | ❌          | ❌      | ❌            | `proxyparse/parse_test.go`                                                                                  |
| `internal/selfupdate`       | ✅   | ❌          | ⚠️      | ✅            | `selfupdate/update_test.go`                                                                                 |
| `internal/service`          | ✅   | ❌          | ⚠️      | ✅            | `service/{lifecycle,netfilter_hook,shim,writeutil}_test.go`                                                 |
| `internal/singbox`          | ✅   | ❌          | ⚠️      | ✅            | `singbox/{config,download,install,version}_test.go`                                                         |
| `internal/state`            | ✅   | ❌          | ❌      | ❌            | `state/state_test.go`, `state/managers_test.go`                                                             |
| `internal/version`          | ✅   | ❌          | ❌      | ❌            | `version/version_test.go`                                                                                   |
| `internal/naiveproxy`       | ✅   | ❌          | ⚠️      | ✅            | `naiveproxy/{config,download,extract,lifecycle}_test.go`                                                    |
| `internal/peer`             | ✅   | ❌          | ⚠️      | ✅            | `peer/{mieru_config,mieru_peer,portalloc}_test.go`                                                          |
| `internal/web`              | ✅   | ❌          | ⚠️      | ✅            | `web/{api,clash,server}_test.go`, `web/routingui_multicore_test.go`, `web/routingui_handlers_test.go`       |
| `internal/core`             | ✅   | ❌          | ❌      | ✅            | `core/{checkconfig,detect,download,registry,validate}_test.go`                                              |
| `internal/core/xray`        | ✅   | ❌          | ❌      | ✅            | `core/xray/{check,check_golden,coreadapter,download,install,render,render_rules,validate,version}_test.go` |
| `internal/core/mihomo`      | ✅   | ❌          | ❌      | ✅            | `core/mihomo/{check,check_golden,download,install,render,render_rules,validate,version}_test.go`            |
| `internal/core/corearchive` | ✅   | ❌          | ❌      | ❌            | `core/corearchive/{binarystream,install}_test.go`                                                           |
| `internal/routing`          | ✅   | ❌          | ❌      | ✅            | `routing/{store,rulesetcheck}_test.go`                                                                      |
| `internal/netif`            | ✅   | ❌          | ❌      | ❌            | `netif/{lan,wan}_test.go`                                                                                   |
| `internal/preset`           | ✅   | ❌          | ❌      | ✅            | `preset/preset_test.go`                                                                                     |
| `internal/elfcheck`         | ✅   | ❌          | ❌      | ❌            | `elfcheck/elfcheck_interp_test.go`                                                                           |
| `pkg/types`                 | ✅   | ❌          | ❌      | ❌            | `pkg/types/{types,routing,outbound_canonical}_test.go`                                                       |

**Легенда:** ✅ покрыто / ❌ не покрыто / ⚠️ частично (сценарий в test-roadmap, не автоматизирован)

---

## 3. Integration tests

Единственный пакет с build-tagged integration-тестами — `internal/firewall`.

**Файл:** `internal/firewall/integration_test.go`
**Build tag:** `//go:build integration`

### Покрытые сценарии

| Тест                                                | Что проверяет                                                                |
|-----------------------------------------------------|-------------------------------------------------------------------------------|
| `TestIntegration_IPTables_EnsureAndDeleteRule`      | EnsureChain, EnsureRule (idempotent ×2), ListRules, DeleteRule (idempotent ×2) |
| `TestIntegration_IPSet_AtomicReplace`               | EnsureSet, AtomicReplace (×2), DestroySet                                      |
| `TestIntegration_Applier_ApplyAndRemove_Full`       | Apply(ModeFull), Remove (idempotent ×2)                                        |
| `TestIntegration_IPRule_EnsureAndDelete`            | EnsureIPRule ([fwmark/SO_MARK](https://www.man7.org/linux/man-pages/man7/socket.7.html) 0x53, table 83), DeleteIPRule (idempotent ×2)             |

### Docker harness

**Dockerfile:** `internal/firewall/testdata/docker/Dockerfile.iptables`
Базовый образ: `golang:1.25`. Устанавливает: `iptables`, `ipset`, `iproute2`, `kmod`.
Запуск: `CMD ["go", "test", "-tags", "integration", "-v", "-timeout", "60s", "./internal/firewall/..."]`

**Capabilities:** `--privileged`. Без `--privileged` тесты пропускаются через `skipIfNoIPTables(t)`.

### Команды запуска

```sh
# Через Makefile (рекомендуется):
make test-integration

# Вручную через Docker:
docker build -t sign-craze-iptables-test -f internal/firewall/testdata/docker/Dockerfile.iptables .
docker run --privileged --rm sign-craze-iptables-test

# Напрямую (если локально доступны iptables/ipset/ip):
go test -tags=integration -v -timeout 60s ./internal/firewall/...
```

---

## 4. Smoke-тесты CLI

**Файл:** `internal/cli/smoke_test.go`

| Тест                                 | Что проверяет                                                            |
|--------------------------------------|---------------------------------------------------------------------------|
| `TestRegistry_AllCommandsRegistered` | Все команды зарегистрированы в `registry`                                 |
| `TestRegistry_NoDuplicateLongFlags`  | Отсутствие дублей Long-флагов                                             |
| `TestDispatch_HelpDoesNotPanic`      | `--help` и `nil` args без паники                                          |
| `TestDispatch_UnknownCommand_Smoke`  | Неизвестная команда → ошибка с сообщением                                 |
| `TestServiceStartIsHidden`           | `--service-start` имеет `Hidden=true`                                     |
| `TestReapplyIsHidden`                | `--reapply` имеет `Hidden=true` (NDM hook)                                |

**Покрытые команды:**
`--ui`, `--install`, `--install-auto`, `--install-offline`, `--reinstall`,
`--start`, `--stop`, `--restart`, `--service-start` (Hidden),
`--status`, `--version`, `--diag`,
`--update`, `--update-geo`, `--update-core`,
`--uninstall`,
`--dpi`, `--dpi-strategy`, `--dpi-update`,
`--mode`, `--port-add`, `--port-del`, `--port-list`,
`--exclude-add`, `--exclude-del`, `--exclude-list`,
`--backup`, `--restore`, `--config-backup`, `--config-restore`,
`--reapply` (Hidden).

### Throttle-тесты `--reapply` (v0.8.0)

**Файл:** `internal/cli/cmd_reapply_test.go`

| Тест                                      | Что проверяет                                                    |
|-------------------------------------------|------------------------------------------------------------------|
| `TestReapplyThrottled_FirstRun`           | Отсутствие маркера → false (дросселирование не срабатывает)      |
| `TestReapplyThrottled_Recent`             | Недавний reapply → true (пропустить повтор)                      |
| `TestReapplyThrottled_Old`                | Старый маркер → false (повтор разрешён)                          |
| `TestTouchReapplyMarker_CreatesFile`      | Создаёт файл-маркер если отсутствует                             |

---

## 4.1. Unit-тесты DPI (v0.8.0)

**Файл:** `internal/dpi/update_test.go`

| Тест                          | Что проверяет                                                      |
|-------------------------------|---------------------------------------------------------------------|
| `TestParseHostLine`           | Парсинг hosts- и Adblock-форматов (15+ кейсов), sanitize           |
| `TestLooksLikeHostname`       | Валидация DNS-допустимых строк                                      |
| `TestShouldUpdate_Disabled`   | Интервал обновления = 0 или -1 → обновление отключено              |
| `TestShouldUpdate_NeverUpdated` | Пустой или битый timestamp → обновление нужно                    |
| `TestShouldUpdate_Threshold`  | Свежий timestamp → пропустить, старый → обновить                   |
| `TestResilientResolver_NotNil`    | DNS-резолвер не nil после инициализации (sanity)                |
| `TestFallbackDNSServers_NotEmpty` | Список fallback DNS-серверов не пустой (sanity)                 |

---

## 4.2. Unit-тесты `firewall/modes` — Policy+DPI (v0.8.0)

**Файл:** `internal/firewall/modes/policy_dpi_test.go`

| Тест                                                      | Что проверяет                                                    |
|-----------------------------------------------------------|------------------------------------------------------------------|
| `TestPolicyDPIRules_HasWANInterfaceFilter`                | Jump-правило содержит флаг `-o $WAN_IFACE`                       |
| `TestPolicyDPIRules_BackwardCompat_EmptyWAN`              | Пустой wanIface → jump без флага `-o` (back-compat)              |
| `TestPolicyDPIRules_VPNExcludeIPsBeforeNFQUEUE`          | RETURN-правила для VPN-exclude генерируются перед [NFQUEUE](https://www.netfilter.org/projects/libnetfilter_queue/)        |
| `TestPolicyDPIRules_VPNExcludeIPsEmpty`                   | Без exclude-IP цепочка не содержит лишних RETURN-правил          |
| `TestPolicyDPIRules_OrderInvariant`                       | Порядок: все RETURN строго до NFQUEUE-правил                     |

---

## 4.3. E2E тесты multi-core routing (v1.0.0)

**Файл:** `internal/web/routingui_multicore_test.go`

| Тест | Что покрывает |
|------|---------------|
| `TestE2E_FullFlow_Xray` | Полный цикл: загрузка routing.json → Apply с активным ядром xray → проверка, что config.json содержит dokodemo-door (tproxy), `rule_set` отсутствует, geosite/geoip матчеры присутствуют |
| `TestE2E_FullFlow_Mihomo` | Полный цикл с mihomo: routing.json → Apply → config.yaml содержит `rule-providers` секцию с `.mrs` URL; TUN inbound не блокирует apply |
| `TestE2E_PresetApply_PerCore` | Применение preset через `/api/routing/preset` при каждом из трёх ядер: URL резолвится через `c.GeoFormat()`, результирующий конфиг корректен для данного ядра |
| `TestE2E_Validate_Warnings_XrayTunInbound` | `POST /api/validate` с routing.json, содержащим TUN inbound, при активном ядре xray: ответ содержит warning, HTTP 200 (не ошибка), Apply после validate не падает |

**Файл:** `internal/cli/cores_sync_test.go`

| Тест | Что покрывает |
|------|---------------|
| `TestValidCores_SyncWithRegistry` | `state.ValidCores` package var содержит ровно те же ядра, что зарегистрированы в core registry — ловит drift при добавлении нового ядра без обновления ValidCores |

---

## 4.4. Unit-тесты naive/peer (v1.3.0)

**Файл:** `internal/naiveproxy/*_test.go`, `internal/peer/*_test.go`

| Тест                                    | Что проверяет                                                               |
|-----------------------------------------|-----------------------------------------------------------------------------|
| `TestParseNaiveURL`                     | Парсинг proxy-URL [naiveproxy](https://github.com/klzgrad/naiveproxy): scheme, user:pass, host:port, edge cases |
| `TestPortAllocator_Reserve`             | port allocator резервирует уникальный порт, не выдаёт дубли                 |
| `TestNaiveLifecycle_StartStop`          | Start запускает subprocess, Stop завершает его без утечки goroutine          |
| `TestNaiveConfigRender`                 | Рендер конфига naiveproxy из шаблона: поля listen, proxy, log               |
| `TestMieruConfigRender`                 | Рендер конфига [mieru](https://github.com/enfein/mieru) из шаблона: portBindings, protocol, encryption |
| `FuzzMieruWireProto`                    | Fuzz-тест wire-протокола mieru: 3 файла corpus в `internal/peer/testdata/fuzz/` |

---

## 4.5. Регрессии и hardening-тесты (v1.4.0)

**Файл:** `internal/firewall/applier_test.go`, `internal/service/lifecycle_test.go`, `internal/core/xray/render_test.go`, `internal/routing/store_test.go`

| Тест                                             | Что проверяет                                                                                    |
|--------------------------------------------------|--------------------------------------------------------------------------------------------------|
| `TestApplier_Apply_Policy_BypassBeforeTProxy`    | Регрессия SSH hang 2026-05-13: bypass-правило (LAN_IP RETURN) стоит до TPROXY-jump в `signcraze_policy` |
| `TestService_DefaultShimPath_IsS99`              | Путь init.d shim = `/opt/etc/init.d/S99signcraze` (расположен после S51dropbear)                |
| `TestRender_Geosite_NoDat_ReturnsError`          | xray без `geosite.dat` возвращает ошибку с подсказкой `--update-geo --core xray`               |
| `TestRender_Geoip_NoDat_ReturnsError`            | xray без `geoip.dat` возвращает ошибку с подсказкой `--update-geo --core xray`                 |
| `TestRender_Geosite_DatPresent_OK`               | Успешный путь: оба dat-файла присутствуют → рендер без ошибок                                  |
| `TestFilterUnreferencedRuleSets`                 | `internal/routing/store_test.go` — фильтрация неиспользуемых rule-set из routing.json (активен, не skipped) |
| `FuzzMieruWireProto`                             | Fuzz-корпус: 3 файла в `internal/peer/testdata/fuzz/` — см. §4.4                                |

---

## 5. E2E на железе (Phase 9.8 — открыт)

**Устройство:** Keenetic KN-1810, mipsle, GOMIPS=softfloat.
**Статус:** `tasks/todo.md:109-115` — Phase 9.8 не закрыта.

**Скрипты:** `scripts/e2e/run.sh` запускает оркестрованный прогон через шаги:
`00-build` → `01-install` → `02-policy` → `03-dpi-hostlist` → `04-reboot` → `99-uninstall`.
Шаги `01`-`99` (кроме `00-build`) подключают общий `scripts/e2e/lib.sh`.

Детальные сценарии: `tasks/test-roadmap.md` §2–§26. Ключевые E2E-чекпоинты:

| № | Сценарий                                                                                  | Источник                          |
|---|--------------------------------------------------------------------------------------------|-----------------------------------|
| 1 | `--install` → policy создана в RCI, description=`sign-craze`                              | `todo.md:147`                     |
| 2 | `--start` → PolicyMark из RCI, iptables применены, [sing-box](https://sing-box.sagernet.org/) PID                           | `todo.md:148`, roadmap §4         |
| 3 | Привязка устройства в Keenetic «Приоритеты» → трафик через прокси (IP меняется)           | `todo.md:149`, roadmap §6         |
| 4 | Другое устройство (без policy) → прямой выход                                              | `todo.md:150`                     |
| 5 | `--mode full --restart` → переход на legacy, ipset заполнен                                | `todo.md:151`, roadmap §9         |
| 6 | `--uninstall` → policy удалена из RCI, `system configuration save` выполнен                | `todo.md:152`, roadmap §14        |
| 7 | `reboot` → автостарт через `S99signcraze`, `--status` running                              | roadmap §12, safety-fixes #12     |
| 8 | NDM rebuild iptables → hook `--reapply` восстанавливает `signcraze_policy`                 | roadmap §4                        |
| 9 | `--diag` все PASS / только WARN при stopped                                                | roadmap §5, §26                   |
|10 | `--ui on` → Routing Editor на 9092, Admin API на 9091 отвечают                             | roadmap §7, §24                   |
|11 | Параллельные команды → lock работает (одна падает с ошибкой)                               | roadmap §21                       |
|12 | `state.json` валиден после `reboot` посередине записи                                      | roadmap §25                       |

---

## 6. CI workflow

**Файл:** `.github/workflows/ci.yml`

| Job                       | Команда                                                              | Триггер           |
|---------------------------|----------------------------------------------------------------------|-------------------|
| `test`                    | `go mod tidy && go vet ./... && go test -race -timeout 5m ./...`    | push main, PR     |
| `lint`                    | `golangci-lint` (errcheck, govet, staticcheck, revive, errorlint)   | push main, PR     |
| `build` (×4)              | cross-compile: `arm64`, `arm7`, `mipsle`, `mips`                    | push main, PR     |
| `firewall-integration`    | Docker `--privileged` → `internal/firewall/integration_test.go` (`needs: [build, test]`) | push main, PR |

Golden-проверки ядер живут в трёх ОТДЕЛЬНЫХ workflow-файлах (не в `ci.yml`) — каждый тонкий caller переиспользуемого `.github/workflows/core-golden.yml` (`workflow_call`, единственный job `golden`, `name: ${{ inputs.core-name }} check golden`). Отображаемое имя GitHub-check составное: `<job-id вызывающего файла> / <core-name> check golden`.

| Файл                     | Job-id           | Отображаемое имя check                  | Тест-путь                          | Триггер  |
|--------------------------|------------------|------------------------------------------|-------------------------------------|----------|
| `singbox-check.yml`      | `singbox-check`  | `singbox-check / sing-box check golden`  | `./internal/singbox/...`            | push, PR |
| `xray-integration.yml`   | `xray-check`     | `xray-check / xray check golden`         | `./internal/core/xray/...`          | push, PR |
| `mihomo-integration.yml` | `mihomo-check`   | `mihomo-check / mihomo check golden`     | `./internal/core/mihomo/...`        | push, PR |

**Build-tag CI mapping:**

| Build tag / CI job | Назначение |
|-------------------|------------|
| `singboxcheck` (CI `singbox-check / sing-box check golden`) | валидация canonical sing-box конфигов через `sing-box check` |
| `xraycheck` (CI `xray-check / xray check golden`) | валидация canonical xray конфигов через `xray test` |
| `mihomocheck` (CI `mihomo-check / mihomo check golden`) | валидация canonical mihomo YAML через `mihomo -t` |

Все workflow (`ci.yml`, `core-golden.yml`, `release.yml`) используют
`go-version-file: go.mod` — единый источник версии Go без хардкода строки
версии.

### Что НЕ запускается в CI (ручная процедура)

- E2E на железе (Keenetic KN-1810): `scripts/e2e/run.sh` — оркестрованный прогон на устройстве
- [UPX](https://upx.github.io/)-сжатие (только в `make release` / `release.yml` по тегу)

### Release workflow (.github/workflows/release.yml)

Запускается по тегу `v*`. Содержит этапы, не входящие в CI:

| Этап                          | Инструмент / действие                                               | Описание                                                                        |
|-------------------------------|---------------------------------------------------------------------|---------------------------------------------------------------------------------|
| `cosign-sign`                 | `sigstore/cosign-installer` + keyless OIDC через GH Actions OIDC  | Подписывает бинари без долгоживущих ключей ([cosign/Sigstore](https://www.sigstore.dev/)); верификация через `cosign verify` |
| `attest-build-provenance`     | `actions/attest-build-provenance@v4`                               | [SLSA](https://slsa.dev/) provenance attestation для каждого артефакта          |
| Reproducible builds           | `SOURCE_DATE_EPOCH`, `-buildid=`, `-trimpath`                       | Идентичный бинарь при повторной сборке из того же коммита                       |

---

## 7. Покрытие по фазам

| Фаза | Статус | Ключевые тесты |
|------|--------|---------------|
| Phase 0 — scaffold | ✅ | CI smoke (build matrix) |
| Phase 1 — Go scaffold | ✅ | `cli/dispatch_test.go`, `locks/file_test.go`, `exectx/exec_test.go`, `log/log_test.go`, `atomicfs/atomicfs_test.go`, `version/version_test.go` |
| Phase 2 — sing-box | ✅ | `singbox/*_test.go`, `service/{lifecycle,shim}_test.go` |
| Phase 3 — firewall | ✅ | `firewall/*_test.go` (6 unit + 1 integration), `firewall/modes/*_test.go` |
| Phase 4 — DPI/nfqws2 | ✅ | `dpi/*_test.go` |
| Phase 5 — geo | ✅ | `geo/*_test.go` |
| Phase 6 — web UI | ✅ | `web/*_test.go` (api/clash/embed/portfind/routingui/server) |
| Phase 7 — release | ✅ | cross-compile build matrix в CI |
| Phase 8 — CLI-команды | ✅ | `cli/smoke_test.go`, `backup/`, `diag/`, `state/`, `ndm/`, `ghrelease/`, `selfupdate/`, `proxyparse/`, `service/netfilter_hook_test.go` |
| Phase 9 — policy mode | 🚧 | `ndm/policy_test.go`, `ndm/wan_test.go` — unit. **E2E hardware: ❌** |
| v0.8.0 — DPI update + policy-DPI rules + reapply throttle | ✅ | `dpi/update_test.go` (5 unit), `firewall/modes/policy_dpi_test.go` (5 unit), `cli/cmd_reapply_test.go` (4 unit) — все pass |
| v1.0.0 — unified multi-core routing | ✅ | `web/routingui_multicore_test.go` (e2e), `cli/cores_sync_test.go` (unit) — см. §4.3 |
| v1.1.0 — DPI FORWARD chain + routing UI presets AS-IS/TO-BE | ✅ | `firewall/modes/policy_dpi_test.go` (переписан под FORWARD + `! --mark`), `web/routingui_handlers_test.go` (+12 backend-тестов: preview+commit) |
| v1.1.1 — SSH/admin bypass + NDM debounce | ✅ | `firewall/modes/excludes_test.go` (LAN-bypass + admin-ports), `firewall/applier_test.go` (bypass order regression) |
| v1.2.0 — ANSI color | ✅ | `cli/color_test.go` (~10 unit-тестов: opt-out priority, no-op при выкл, semantic helpers) |
| v1.3.0 — naive/mieru supervised peers | ✅ | `naiveproxy/*_test.go` + `peer/*_test.go`, mihomo/xray reject-тесты — см. §4.4 |
| v1.4.0 — hardening (iptables batch, watchdog REDIRECT/DPI, repro builds, cosign, SLSA, regressions) | ✅ | см. §4.5; `firewall/watchdog_test.go`, `firewall/fastpath_test.go` |

---

## 8. Gaps (открытые провалы)

- **redirect mode не реализован** (`internal/firewall/modes/redirect.go` возвращает `nil`): тест заглушки отсутствует — `redirect_test.go` не существует.
- **Watchdog policy** (`tasks/phase9-policy.md:155`): нет периодической проверки наличия policy в Keenetic RCI. Если пользователь удалит — узнаём только при `--start`.
- **Автостарт после reboot** (safety-fixes #12, PARTIAL): `BootTimeoutSec` и `boot.log` готовы, но E2E на железе не проведён.
- **Integration-тесты только в `firewall`**: пакеты `dpi`, `service`, `geo` не имеют integration с реальным ядром.
- **Web UI нет E2E**: `web/*_test.go` используют `httptest` — нет реального WebSocket через браузер.
- **ndm.GetPolicy round-trip**: тесты через `httptest`-заглушку; нет интеграционного теста против реального Keenetic RCI.
- **ipset populate при `--start`**: `geo/ipset_test.go` тестирует изолированно; нет полного цикла с реальным kernel.
- **Conntrack flush**: нет автотеста корректной обработки старых сессий после `--start`.

---

## 9. Команды запуска

```sh
# Unit-тесты:
go test ./...
go test -race -timeout 5m ./...

# Integration (Docker + privileged):
make test-integration
docker run --privileged --rm sign-craze-iptables-test

# Integration без Docker (NET_ADMIN):
go test -tags=integration -v ./internal/firewall/...

# Cross-компиляция (через скилл go-xbuild при отсутствии Go на хосте):
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat \
  go build -ldflags="-s -w" -trimpath \
  -o /tmp/sign-craze-mipsle ./cmd/sign-craze

# Smoke CLI:
go test -v ./internal/cli/...
```

E2E на железе: см. пошаговую инструкцию в `tasks/test-roadmap.md`.

# sign-craze — список задач

## Phase 0 — подготовка ✅

- [x] `go.mod` + `go.sum` (github.com/kittylabassistant/sign-craze, go 1.25.9)
- [x] Структура директорий: все `internal/*/doc.go`, `pkg/types/doc.go`
- [x] `cmd/sign-craze/main.go` (точка входа)
- [x] `Makefile` (arm64, arm7, mipsle, mips + upx + lint + test + test-integration)
- [x] `.golangci.yml` (errcheck, govet, staticcheck, revive, bodyclose, errorlint)
- [x] `.editorconfig`, `.gitattributes`
- [x] `.github/workflows/ci.yml`
- [x] `BEHAVIOR_SPEC.md` (CLI-команды, конфиги, инварианты iptables, lifecycle)
- [x] `docs/ARCHITECTURE.md` (диаграмма слоёв, потоки данных, идентификаторы)

## Phase 1 — Go scaffold ✅

- [x] `internal/cli/dispatch.go` + `dispatch_test.go`
- [x] `internal/log/log.go` + `log_test.go`
- [x] `internal/locks/file.go` + `file_test.go`
- [x] `internal/exectx/exec.go` + `exec_test.go`

## Phase 1 — продолжение ✅

- [x] `internal/errors/errors.go` — sentinel-ошибки (ErrNotInstalled, ErrLockHeld, ErrCoreDown, …)
- [x] `internal/atomicfs/atomicfs.go` — WriteFileAtomic, BackupAndReplace, RestoreBackup
- [x] `internal/version/version.go` — встроенная VERSION через go:embed, build info через runtime/debug
- [x] `pkg/types/types.go` — Mode, Arch, Port, PortRange, IPSetName, Outbound, RoutingRules

## Phase 2 — интеграция sing-box ✅

- [x] `internal/singbox/download.go` — GitHub releases, ETag, SHA256, атомарная запись
- [x] `internal/singbox/install.go` — бэкап, untar, chmod, проверка, откат
- [x] `internal/singbox/config.go` + `templates/tproxy.json.tmpl` — text/template, golden-тесты
- [x] `internal/singbox/version.go` — парсинг вывода sing-box version
- [x] `internal/service/shim.go` — генерация /opt/etc/init.d/S05signcraze, idempotency
- [x] `internal/service/lifecycle.go` — Start/Stop/Status/Restart, PID-файлы, Setpgid detach

## Phase 3 — firewall (высокий риск) ✅

- [x] `internal/firewall/iptables.go` — интерфейс IPTables, EnsureRule/DeleteRule/ListRules
- [x] `internal/firewall/ipset.go` — AtomicReplace через create-swap-destroy
- [x] `internal/firewall/applier.go` — Apply/Remove, откат при ошибке
- [x] `internal/firewall/route.go` — EnsureIPRule/DeleteIPRule/EnsureLocalRoute/DeleteLocalRoute
- [x] `internal/firewall/modes/tproxy.go`
- [x] `internal/firewall/modes/redirect.go` (заглушка — отложено)
- [x] `internal/firewall/modes/hybrid.go`
- [x] `testdata/docker/Dockerfile.iptables` + integration_test.go (тег integration)

## Phase 4 — DPI/nfqws2 ✅

- [x] `internal/dpi/download.go` — GitHub releases, ETag, SHA256, атомарная запись
- [x] `internal/dpi/install.go` — untar, chmod, backup+replace
- [x] `internal/dpi/config.go` + `templates/nfqws2.conf.tmpl` — text/template, DetectISPInterface
- [x] `internal/dpi/nfqueue.go` — Enable/Disable NFQUEUE в signcraze_dpi
- [x] `internal/dpi/lifecycle.go` — обёртка над service.NewLifecycle для nfqws2

## Phase 5 — Гео-файлы ✅

- [x] `internal/geo/srs.go` — FetchManifest, Update (SHA256-diff, атомарная запись)
- [x] `internal/geo/ipset.go` — ParseCIDRList, ApplyToIPSet
- [x] `sign-craze-dat/.github/workflows/update.yml` — MetaCubeX → .srs → manifest.json → release
- [x] fix `ruleSetBaseURL` в config.go + testdata: `sign-craze-dats` → `sign-craze-dat`

## Phase 6 — Встроенный Web UI 🔄

- [x] `internal/web/types.go` — интерфейсы StatusReader, ConfigRW, PortsManager, ExcludesManager; StatusInfo
- [x] `internal/web/auth.go` — LoadOrCreateCreds, CheckPassword (bcrypt cost=12)
- [x] `internal/web/ws.go` — минимальный WebSocket upgrade (RFC 6455, без зависимостей)
- [x] `internal/web/server.go` — Server, NewServer, Start/Stop, basicAuth middleware, recoverMiddleware
- [x] `internal/web/clash.go` — Clash-совместимый API: /, /version, /configs, /proxies, /connections, WS /traffic, WS /logs
- [x] `internal/web/api.go` — Admin REST: /api/status, /api/config, /api/ports, /api/excludes
- [x] `internal/web/embed.go` — go:embed assets/zashboard + spaHandler (SPA fallback → index.html)
- [x] `internal/web/assets/zashboard/index.html` — placeholder (реальный Zashboard — git submodule)
- [x] `golang.org/x/crypto` добавлен в go.mod (bcrypt)
- [x] `internal/cli/cmd_ui.go` — `--ui on|off` → web.Server.Start/Stop
- [x] Добавить git submodule Zephyruso/zashboard@gh-pages → `internal/web/assets/zashboard` (shallow=true)

## Phase 7 — Release pipeline ✅

- [x] `.github/workflows/release.yml` — по тегу v* → сборка всех + UPX + upload
- [x] `scripts/install.sh` — определение архитектуры, проверка места, загрузка, установка

## Заметки

- Go не установлен на хосте — тесты и `go mod tidy` запускать в toolbox/контейнере
- Комментарии и документация только на русском языке
- go версия: 1.25.9
- Phase 3 (firewall) требует живого роутера или Docker --privileged для тестирования

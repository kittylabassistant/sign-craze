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

## Phase 5 — Гео-файлы (параллельно с 2/3)

- [ ] `internal/geo/srs.go` — парсинг manifest.json, выборочная загрузка, атомарная замена
- [ ] `internal/geo/ipset.go` — сырой IP-лист → ipset

## Phase 6 — Встроенный Web UI

- [ ] `internal/web/server.go` — http.ServeMux + цепочка middleware
- [ ] `internal/web/api.go` — REST-эндпоинты
- [ ] `internal/web/assets/` — submodule Zashboard + admin SPA

## Phase 7 — Release pipeline

- [ ] `.github/workflows/release.yml` — по тегу v* → сборка всех + UPX + upload
- [ ] `scripts/install.sh` — определение архитектуры, проверка места, загрузка, установка

## Заметки

- Go не установлен на хосте — тесты и `go mod tidy` запускать в toolbox/контейнере
- Комментарии и документация только на русском языке
- go версия: 1.25.9
- Phase 3 (firewall) требует живого роутера или Docker --privileged для тестирования

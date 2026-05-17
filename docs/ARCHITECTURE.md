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
| `internal/naiveproxy` | adapter naiveproxy: download/extract/install/lifecycle (supervised peer) |
| `internal/peer` | supervised-peer scaffolding + mieru core + port allocator |
| `internal/core` | registry + абстрактный интерфейс `Core`; регистрирует ядра: sing-box, xray, mihomo |
| `internal/core/xray` | адаптер ядра xray; translation `RouteRule → xray rules[]`; `RuleSet` с префиксом `geosite-`/`geoip-` → matcher |
| `internal/core/mihomo` | адаптер ядра mihomo; translation `RouteRule → TYPE,VALUE,ACTION`; `RuleSets` с `.mrs` URL → `rule-providers:` |
| `internal/service` | генерация init.d shim; интерфейс `Lifecycle`, связывающий singbox и nfqws2 |
| `internal/geo` | загрузка SRS из sign-craze-dats; конвертация IP-листа → ipset |
| `internal/web` | встроенный HTTP-сервер (Zashboard :9090 + admin REST API :9091 + Routing Editor :9092 + DPI targets API) |
| `internal/locks` | эксклюзивный flock против параллельных запусков |
| `internal/log` | глобальный `slog.Logger` с ротацией по размеру |
| `internal/atomicfs` | атомарная запись: write → fsync → rename |
| `internal/version` | встроенная `VERSION`, build info через `runtime/debug` |
| `internal/errors` | sentinel-ошибки (`ErrNotInstalled`, `ErrLockHeld`, …) |
| `pkg/types` | общие типы (`Mode`, `Arch`, `Port`, `RoutingConfig`, `CoreRenderParams`, …) |

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
cli/doStart
  → mustActiveCore()               (из registry по state.Core)
  → ensureConfigFreshForCore(ctx, c, st)
      → routing.Load(routing.json)          (RoutingConfig или nil если не создан)
      → c.RenderConfig(CoreRenderParams{    (единый путь для всех трёх ядер)
            Mode, Outbounds, RoutingConfig,
            InboundMode, DefaultOutboundTag
          })
      → bytes.Equal → fast-path skip        (на slow MIPS CheckConfig дорого)
      → atomicfs.WriteFileAtomic(c.ConfigPath())
      → c.CheckConfig(ctx, runner, path)    (sing-box check / xray test / mihomo -t)
  → needsTUN(c, st)                (true только для sing-box + Inbound!="tproxy")
      если true  → firewall.ForceDeleteTUNDevice  (cleanup stale signbox-tun)
  → c.NewLifecycle().Start(ctx)
  → если needsTUN: applier.AttachTUN(ctx, "signbox-tun")
  → если DPIEnabled: nfqws2.lifecycle.Start
```

Ключевой инвариант: `ensureConfigFreshForCore` пишет в `c.ConfigPath()` (не в
захардкоженный `/opt/etc/sign-craze/config.json`), поэтому Apply через UI
корректно обновляет конфиг активного ядра — xray, mihomo или sing-box.

## Unified routing (v1.0.0)

Единый `pkg/types.RoutingConfig` потребляется всеми тремя ядрами. `routing.json`
на диске core-agnostic: при переключении `--core` файл не мигрирует, несовместимые
конструкции surfaced через `apiValidate` warnings.

### CoreRenderParams

```go
type CoreRenderParams struct {
    Mode               Mode
    Outbounds          []Outbound
    RoutingConfig      *RoutingConfig // nil → legacy-путь (только state.Outbounds)
    InboundMode        string         // "tun" | "tproxy"
    DefaultOutboundTag string         // fallback final outbound
}
```

### Translation: RoutingConfig → native-формат ядра

| Поле `RouteRule` | sing-box | xray | mihomo |
|---|---|---|---|
| `Domain/DomainSuffix/DomainKeyword` | `domain_suffix:`, `domain:` | `domain: ["full:...","domain:...","keyword:..."]` | `DOMAIN,…` / `DOMAIN-SUFFIX,…` / `DOMAIN-KEYWORD,…` |
| `IPCIDR` | `ip_cidr:` | `ip: ["..."]` | `IP-CIDR,…,no-resolve` |
| `Port` (uint16 slice) | `port:` | `port: "80,443"` | `DST-PORT,…` |
| `PortRange` ("1000:2000") | `port_range:` | `port: "1000-2000"` | — |
| `Network` | `network:` | `network:` | `NETWORK,…` |
| `RuleSet: ["geosite-youtube"]` | rule_set entry + `.srs` URL | `domain: ["geosite:youtube"]` (translation) | `RULE-SET,geosite-youtube,…` + rule-providers entry |
| `RuleSet: ["geoip-ru"]` | rule_set entry + `.srs` URL | `ip: ["geoip:ru"]` (translation) | `RULE-SET,geoip-ru,…` + rule-providers entry |
| `Action: "reject"` | outbound `block` | outbound `block` (blackhole) | `REJECT` |
| Final | `route.final` | последнее rule без matchers | `MATCH,<tag>` |

**xray translation**: `geosite-<name>` → `geosite:<name>`, `geoip-<cc>` → `geoip:<cc>`.
xray не имеет rule_set механизма — используются встроенные geosite.dat/geoip.dat.
RuleSet без префикса `geosite-`/`geoip-` (например refilter) → warning, пропускается.

**mihomo rule-providers**: URL резолвится через `resolveRuleSetURL(tag, core.GeoMRS)` из
`routingui_presets.go`. `.srs` URL на mihomo → warning (нужен `.mrs`). Исправляется
повторным apply preset.

### needsTUN(c, st)

```go
func needsTUN(c core.Core, st *state.State) bool {
    return c.Name() == "sing-box" && st.Inbound != "tproxy"
}
```

Инкапсулирует три прежних `if c.Name() == "sing-box"` в `cmd_lifecycle.go`.
xray/mihomo всегда TProxy (dokodemo-door / tproxy-port), TUN не создают.
sing-box в режиме tproxy (state.Inbound="tproxy") тоже не создаёт TUN.

### Per-core preset URLs (`routingui_presets.go`)

`ruleSetSources` — translation table: каждая запись хранит SRS/MRS URL и Behavior.
При `apiPresetsApply` URL выбирается через `resolveRuleSetURL(tag, c.GeoFormat())`:
- sing-box (`GeoSRS`) → SagerNet/sing-{geosite,geoip} `.srs` URL
- mihomo (`GeoMRS`) → MetaCubeX/meta-rules-dat `.mrs` URL
- xray (`GeoDAT`) → URL пустой, ok=true если `geosite-`/`geoip-` префикс; render-слой переводит в matcher

### Renderer/Validator/ConfigFormat closures (`routingui_deps.go`)

`RoutingUIDeps` передаётся в web-handlers при boot'е. Каждый callback re-resolve'ит
активное ядро на каждом вызове через `mustActiveCore()` — переключение `--core`
без перезапуска UI работает:

```
Renderer       → c.RenderConfig(uiRenderParams(st, cfg))  → bytes ядра
Validator      → validateRoutingConfig: render + tmp + c.CheckConfig + collectCompatWarnings
ConfigFormat   → c.ConfigFormat()  (JSON | YAML) для apiPreview Content-Type
ActiveGeoFormat → c.GeoFormat()    (SRS | DAT | MRS) для preset URL резолва
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

## Packet path: режим policy (TPROXY/REDIRECT) — v0.8.0

WAN-интерфейс определяется автоматически через `ip route show default` в `applyInternal()` до выбора режима.

```plain
LAN client ─► br0 (mangle:PREROUTING):
  ├─ mark==0xffffaab → signcraze_policy → TPROXY 127.0.0.1:7895 mark=0x53
  │     ├─ ip rule: fwmark 0x53 lookup 83
  │     ├─ table 83: local default → sing-box socket (userspace)
  │     └─ sing-box → outbound к VPN-серверу
  │
  └─ без mark → main route → eth3 (WAN)

mangle:POSTROUTING  -o $WAN_IFACE  (signcraze_policy_dpi):
  ├─ -d <DPIExcludeIPs[0]> -j RETURN   ← VPN-эндпоинты: Reality-fingerprint
  ├─ -d <DPIExcludeIPs[1]> -j RETURN     нельзя десинхать (ISP обнаружит VPN)
  │   ...
  └─ -p tcp/udp --dport <DPI-ports> -j NFQUEUE 300
        nfqws2 десинхает ClientHello → ISP видит модифицированный пакет
```

**Исключение VPN-эндпоинтов (`DPIExcludeIPs`)**: Reality маскируется под TLS-fingerprint
реального хоста. nfqws2-десинк ломает этот fingerprint, что позволяет ISP детектировать VPN.
Кроме того, downstream-клиенты с собственным VPN-клиентом к тому же эндпоинту выйдут из строя.
Решение: `state.DPIExcludeIPs` + best-effort резолв первого `outbound.Server`.
Перед каждым блоком NFQUEUE-портов добавляются RETURN-правила для каждого IP из этого списка.

## Auto-update hostlist (v0.8.0)

Подсистема периодического обновления DPI-листа запускается горутиной внутри `--service-watchdog`
(`cmd_service_watchdog.go`):

```plain
watchdog loop (горутина):
  → тикер 1 час
  → ShouldUpdate(state.DPILastUpdate, IntervalHours)
      если да → dpi.UpdateHostlist(ctx, urls, extraTargets, dst)
                   ├─ HTTP-загрузка каждого URL (timeout 30s, max body 2MB)
                   ├─ парсинг hosts/Adblock-форматов
                   ├─ merge + deduplicate
                   └─ atomic write (atomicfs) → dst
  → ошибка одного URL не прерывает обновление (best-effort)
```

Ограничения: HTTP timeout 30s учитывает медленный TLS-хендшейк на MIPS; body cap 2MB
защищает от OOM на роутерах с 128MB RAM.

## Reapply throttle (v0.8.0)

Путь `--reapply` защищён маркером `/opt/var/run/sign-craze-reapply.last` (mtime):

- Throttle-период: **5 секунд**.
- При срабатывании вызывается `Reconcile` (idempotent re-apply) — без pre-flights и
  без auto-rollback, в отличие от полного `Apply`.
- Это исключает каскадный reapply при одновременных внешних событиях (перезапуск
  init.d shim, watchdog-тик).

## DNS fallback chain (v0.8.1)

Применяется **только** к HTTP-клиенту в `dpi.UpdateHostlist` — не затрагивает DNS,
используемый sing-box или другими подсистемами sign-craze.

Корень проблемы: на Keenetic DNSCrypt-Entware занимает `127.0.0.1:53` и фильтрует
`raw.githubusercontent.com` (CDN Fastly попадал в blocklists 2024–2026).
Стандартный `net.DefaultResolver` молча получал NXDOMAIN, и обновление hostlist падало.

`internal/dpi/update.go::resilientResolver` — кастомный `net.Resolver` с
`PreferGo: true` и цепочкой fallback-серверов в `Dial`:

```plain
resilientResolver.LookupHost(host):
  попытка 1 → системный резолвер (127.0.0.1:53, DNSCrypt-Entware)
  попытка 2 → 1.1.1.1:53   (Cloudflare)
  попытка 3 → 9.9.9.9:53   (Quad9)
  попытка 4 → 8.8.8.8:53   (Google)
  каждая попытка timeout=5s; первый успешный ответ возвращается немедленно
```

`http.Client` в `UpdateHostlist` получает кастомный `Transport` с этим резолвером
через `DialContext`. Исходящие HTTP-запросы идут напрямую (минуя политику sing-box).

Файл: `internal/dpi/update.go`

## Hostlist apply rule (v0.8.2)

`internal/cli/deps.go::hostlistShouldApply()` — предикат, определяющий, нужно ли
передавать `--hostlist=<path>` в аргументы запуска nfqws2.

Логика:

```plain
hostlistShouldApply(state, path) bool:
  state.DPITargets непуст  →  true  (явные цели, hostlist актуален)
  файл path существует     →  true  (файл уже есть — пусть nfqws2 использует)
  иначе                    →  false (hostlist не передаётся)
```

До v0.8.2 `auto-update` мог записать `dpi-hostlist.txt`, но если `state.DPITargets`
был пуст (цели не настроены), nfqws2 запускался без `--hostlist` и игнорировал файл.
Теперь наличие файла само по себе является достаточным условием.

Файл: `internal/cli/deps.go`

## Routing config migration (v0.8.3)

`internal/cli/deps.go::configParamsFromState()` выполняет однократную миграцию
`/opt/etc/sign-craze/routing.json` при обнаружении устаревшей конфигурации.

Корень проблемы: при обновлении v0.6.x → v0.8.x routing.json мог содержать TUN-inbound
(`"type":"tun"`), оставшийся от bootstrap. Метод `Render()` проверял inbound-режим
по `state.Inbound` и при `"tproxy"` ожидал TPROXY-inbound, но пропускал инъекцию,
если TUN-запись уже присутствовала. Результат: sing-box стартовал в TUN-режиме,
слушал `127.0.0.1:7895` — НИКТО НЕ СЛУШАЛ → TCP reset для всего fwmark-трафика.

```plain
configParamsFromState(state):
  если state.Inbound == "tproxy":
    загрузить routing.json
    отфильтровать inbounds с type=="tun"
    если фильтрация что-то удалила:
      atomicfs.Write(routing.json, обновлённый конфиг)
      slog.Info("migrated: removed stale tun inbound")
  вернуть configParams
```

Миграция идемпотентна: повторный вызов ничего не меняет, если TUN-inbound уже удалён.

Файл: `internal/cli/deps.go`

## DPI chain в FORWARD (v1.1.0)

NFQUEUE-цепочка `signcraze_dpi_fwd` живёт в `mangle FORWARD`, а не в `mangle POSTROUTING -o $WAN` как до v1.1.0. Это покрывает все LAN-устройства, а не только sing-box → VPS:

- Jump из `FORWARD`: `-o $WAN_IFACE -m mark ! --mark 0x53 -j signcraze_dpi_fwd`
- LAN policy-устройства перехватываются TPROXY до FORWARD → не попадают в DPI (трафик уже идёт через sing-box к VPS, desync ему не нужен).
- LAN не-policy (TV, гости, IoT) идут через FORWARD → DPI → nfqws2 hostlist desync.
- Self-traffic sing-box (`SO_MARK=0x53`) отсекается mark-фильтром.
- Legacy `signcraze_policy_dpi` сохранён в коде только для cleanup при апгрейде с v1.0.x.

Связанные файлы: `internal/firewall/modes/policy.go::DPIForwardChainName`, `applier.go::applyPolicyMode`, тесты `policy_dpi_test.go`.

## SSH/admin bypass + NDM debounce (v1.1.1)

Проблема: при policy с 10+ устройств Keenetic метил mark и пакетам с `dst=<LAN_IP>:222` — SSH к роутеру попадал в TPROXY, sing-box пытался дозвониться до LAN_IP как до remote, коннект виснул.

Реализация:

- **`LocalBypassRules`** + `ensureLANBypass`: правило `-d <LAN_IP> -j RETURN` на pos=1 в `signcraze_policy` (mangle TPROXY-real, nat REDIRECT, mangle TUN). LAN_IP автодетект через `netif.DetectLANAddr`.
- **`AdminPortsBypassRulesForChain`**: defense-in-depth — bypass для портов 22/222 TCP+UDP. `AdminPortsBypassRules` → back-compat обёртка.
- **`ndm.GetHostsWithPolicy`** + **`UnsetHostPolicy`**: `doUninstall` сначала отвязывает все хосты от policy через RCI, потом `DeletePolicy`. Reboot после `--uninstall` восстанавливает доступ без factory reset.
- **netfilter.d hook**: `flock -x -n` + pending-маркер + trailing debounce. Пачка из 10 NDM-событий = 2 reapply (один сразу, один в конце пачки) вместо 10 параллельных fork/exec.

## Supervised peers: naive/mieru (v1.3.0)

Naive (`klzgrad/naiveproxy`) и mieru (`enfein/mieru`) — protocol-specific прокси, которые не нативны ни одному из встроенных ядер. Sign-craze запускает их как daemon, sing-box подключается через socks5-outbound:

```
LAN client → iptables (TPROXY/REDIRECT) → sing-box (socks5 outbound)
                                              ↓
                                         127.0.0.1:NaiveListenPort
                                              ↓
                                         naive daemon → upstream HTTPS
```

- URL-форматы: `naive+https://user:pass@host:port`, `naive+quic://...`, `mieru://`, `mierus://`.
- Поддерживаемые архитектуры naive: arm64, arm7, mipsle (klzgrad публикует только LE; mips BE отклоняется при `--install`).
- Mieru cross-build встроен в Makefile под все 4 архитектуры (`make mieru-mipsle` etc).
- xray и mihomo явно reject naive+mieru с понятной ошибкой (см. `internal/core/{xray,mihomo}/validate.go`).
- Жизненный цикл peer'ов привязан к sign-craze: `doStart` поднимает naive до sing-box, `doStop` опускает после; `internal/diag` включает health-check.

Связанные файлы: `internal/naiveproxy/`, `internal/peer/`, ADR-0020-supervised-peer, ADR-0021-naiveproxy-process-chain.

## Hardening (v1.4.0)

Пост-аудитный hardening по 3 параллельным Explore-аудитам (firewall, cores, build/CI):

- **iptables-restore batching**: `BatchBuilder` + `RestoreBatch` + `Flush` — собирает правила в один blob и применяет через `iptables-restore --noflush`. Сокращение fork/exec 24 → 3 на slow MIPS при `applyPolicy`/`applyFull`.
- **WAN cache**: `WANIface` кэшируется, `InvalidateWANCache` при изменении сети. Watchdog tick больше не fork-ит `ip route` каждые 30s.
- **Watchdog coverage**: REDIRECT (nat PREROUTING) и DPI FORWARD chain помимо TPROXY mangle.
- **Reproducible builds**: `-buildid=` + `-trimpath` + `SOURCE_DATE_EPOCH` в release.yml. Артефакт байт-в-байт одинаковый при идентичном входе.
- **Cosign keyless OIDC**: `.sig` + `.pem` рядом с каждым артефактом, идентичность подтверждается через GitHub OIDC issuer + workflow path.
- **SLSA build provenance**: `actions/attest-build-provenance@v4` (после v1.4.1) — проверка через `gh attestation verify`.
- **`bcrypt cost`**: build-tagged `auth_cost_lowmem.go` cost=10 для `GOARCH=mips/mipsle`; default cost=12. Login admin UI на slow MIPS: 6 c → 2 c.
- **PID-файлы**: `atomicfs.WriteFileAtomic` + `processAlive` с match `/proc/<pid>/comm` (15-байтовый truncation учтён) → PID-reuse guard.
- **`--diag --json`**: machine-parseable вывод для скриптов и мониторинга.
- **WebSocket keepalive**: ping 30s (RFC 6455 §5.5.2) — соединения за NAT/UPnP не падают.
- **Embed assets**: `Cache-Control: immutable` для статики, `no-cache` для index.html.
- **xray geo guard**: early-check `geoip.dat`/`geosite.dat` с подсказкой `--update-geo --core xray`; `GeoAssetsDir=""` = skip для preview/Validator.
- **SHA256 streaming в geo/srs**: OOM-guard для 30–100 MB `.srs`.
- **Регрессии** (из `tasks/lessons.md`): `TestApplier_Apply_Policy_BypassBeforeTProxy`, `TestService_DefaultShimPath_IsS99`, `TestRender_Geosite_NoDat_ReturnsError`, `FuzzMieruWireProto`.

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

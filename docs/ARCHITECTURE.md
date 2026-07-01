# sign-craze: Архитектура

> Версия: 2026-07-01. Архитектура v1.6.3.

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
│  excludes · reapply · service-watchdog              │
│  core (multi) · peers (mieru, naive)                │
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
          ОС: [iptables](https://www.netfilter.org/projects/iptables/index.html) · [ipset](https://ipset.netfilter.org/) · ip · [opkg](https://openwrt.org/docs/guide-user/additional-software/opkg)
```

## Вспомогательные пакеты

| Пакет | Роль |
| ----- | ---- |
| `internal/singbox` | загрузка, установка, генерация конфига, версия [sing-box](https://sing-box.sagernet.org/) |
| `internal/naiveproxy` | adapter [naiveproxy](https://github.com/klzgrad/naiveproxy): download/extract/install/lifecycle (supervised peer) |
| `internal/peer` | supervised-peer scaffolding + [mieru](https://github.com/enfein/mieru) core + port allocator |
| `internal/core` | registry + абстрактный интерфейс `Core`; регистрирует ядра: sing-box, [xray](https://xtls.github.io/), [mihomo](https://wiki.metacubex.one/) |
| `internal/core/xray` | адаптер ядра xray; translation `RouteRule → xray rules[]`; `RuleSet` с префиксом `geosite-`/`geoip-` → matcher |
| `internal/core/mihomo` | адаптер ядра mihomo; translation `RouteRule → TYPE,VALUE,ACTION`; `RuleSets` с `.mrs` URL → `rule-providers:` |
| `internal/service` | генерация init.d shim; интерфейс `Lifecycle`, связывающий singbox и [nfqws2](https://github.com/nfqws/nfqws2-keenetic) |
| `internal/geo` | загрузка SRS из sign-craze-dats; конвертация IP-листа → [ipset](https://ipset.netfilter.org/) |
| `internal/web` | встроенный HTTP-сервер (Zashboard :9090 + admin REST API :9091 + Routing Editor :9092 + DPI targets API) |
| `internal/locks` | эксклюзивный [flock](https://man7.org/linux/man-pages/man2/flock.2.html) против параллельных запусков |
| `internal/log` | глобальный `slog.Logger` с ротацией по размеру |
| `internal/atomicfs` | атомарная запись: write → fsync → rename |
| `internal/version` | встроенная `VERSION`, build info через `runtime/debug` |
| `pkg/types` | общие типы (`Mode`, `Arch`, `Port`, `RoutingConfig`, `CoreRenderParams`, …) |

Sentinel-ошибки живут в своих пакетах (`locks.ErrLocked`, `ndm.ErrNotFound`, `netif.ErrLANNotFound`, `firewall.ErrFWMarkConflict`, `web.ErrNoFreePort`); отдельного пакета `internal/errors` нет.

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

> Архитектурное решение зафиксировано в ADR-0016 и ADR-0025.

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
xray не имеет rule_set механизма — используются встроенные [geosite.dat/geoip.dat](https://sing-box.sagernet.org/configuration/rule-set/).
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
xray/mihomo всегда [TProxy](https://www.kernel.org/doc/html/latest/networking/tproxy.html) (dokodemo-door / tproxy-port), TUN не создают.
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

Серверы биндятся на LAN-bridge IP (`netif.DetectLANAddr`, fail-secure: при невозможности определить LAN-адрес `--ui-daemon` отказывается стартовать, чтобы UI без auth/TLS не оказался на `0.0.0.0`). Дополнительно правила в `filter/INPUT` (owner-comment, idempotent) дропают входящий трафик на порты 9090/9091/9092 от WAN-интерфейса.

## Поток данных: `--ui on` и firewall watchdog

Web UI и watchdog — независимые процессы.

`--ui on` поднимает только HTTP-серверы:

```plain
cli/startUI
  → web.Server.ListenAndServe (блокирует до SIGTERM/ctx.Done)
```

`--service-watchdog` — отдельный standalone-демон (запускается init.d shim `S99signcraze`, переживает `--ui off`):

```plain
cli/handleServiceWatchdog → firewall.NewWatchdog(0, reconcileFirewall).Run(ctx)
  loop (30 с):
    → IPTables.CheckCriticalRules (iptables -C, дешёвая проверка)
    → если правила отсутствуют → Applier.Reconcile (idempotent re-apply)
```

## Режимы маршрутизации

| Режим | sing-box | nfqws2 | [iptables](https://www.netfilter.org/projects/iptables/index.html) |
| ----- | -------- | ------ | -------- |
| `policy` (default) | да | опционально | [fwmark](https://www.man7.org/linux/man-pages/man7/socket.7.html) 0xffffaaXX → TPROXY :7895 (sing-box) |
| `full` | да | опционально | ipset dst-match → TPROXY :7895 (sing-box) |

Legacy-имена `proxy`, `dpi`, `hybrid` принимаются для обратной совместимости и автоматически конвертируются.

Приоритет цепочек в `mangle:PREROUTING`:

1. `signcraze_dpi` ([NFQUEUE](https://www.netfilter.org/projects/libnetfilter_queue/), `--queue-bypass`) — обрабатывается первой (только full+DPI)
2. `signcraze` / `signcraze_policy` (mark-маршрутизация) — обрабатывается второй

DPI-цепочка `signcraze_dpi_fwd` устанавливается в `mangle:FORWARD` (`-o $WAN_IFACE -m mark ! --mark 0x53`, начиная с v1.1.0 — ловит все LAN-устройства, см. раздел «DPI chain в FORWARD» ниже). Legacy `signcraze_policy_dpi` (`mangle:POSTROUTING`) сохранён только для cleanup при апгрейде с v1.0.x.

## DPI chain в FORWARD (v1.1.0)

NFQUEUE-цепочка `signcraze_dpi_fwd` живёт в `mangle FORWARD`, а не в `mangle POSTROUTING -o $WAN` как до v1.1.0. Это покрывает все LAN-устройства, а не только sing-box → VPS:

- Jump из `FORWARD`: `-o $WAN_IFACE -m mark ! --mark 0x53 -j signcraze_dpi_fwd`
- LAN policy-устройства перехватываются TPROXY до FORWARD → не попадают в DPI (трафик уже идёт через sing-box к VPS, desync ему не нужен).
- LAN не-policy (TV, гости, IoT) идут через FORWARD → DPI → nfqws2 hostlist desync.
- Self-traffic sing-box (`SO_MARK=0x53`) отсекается mark-фильтром.
- Legacy `signcraze_policy_dpi` сохранён в коде только для cleanup при апгрейде с v1.0.x.

Связанные файлы: `internal/firewall/modes/policy.go::DPIForwardChainName`, `applier.go::applyPolicyMode`, тесты `policy_dpi_test.go`.

## SSH/admin bypass + NDM debounce (v1.1.1)

Проблема: при policy с 10+ устройств [Keenetic](https://help.keenetic.com/hc/ru) метил mark и пакетам с `dst=<LAN_IP>:222` — SSH к роутеру попадал в TPROXY, sing-box пытался дозвониться до LAN_IP как до remote, коннект виснул.

Реализация:

- **`LocalBypassRules`** + `ensureLANBypass`: правило `-d <LAN_IP> -j RETURN` на pos=1 в `signcraze_policy` (mangle TPROXY-real, nat REDIRECT, mangle TUN). LAN_IP автодетект через `netif.DetectLANAddr`.
- **`AdminPortsBypassRulesForChain`**: defense-in-depth — bypass для портов 22/222 TCP+UDP. `AdminPortsBypassRules` → back-compat обёртка.
- **`ndm.GetHostsWithPolicy`** + **`UnsetHostPolicy`**: `doUninstall` сначала отвязывает все хосты от policy через RCI, потом `DeletePolicy`. Reboot после `--uninstall` восстанавливает доступ без factory reset.
- **netfilter.d hook**: `flock -x -n` + pending-маркер + trailing debounce. Пачка из 10 NDM-событий = 2 reapply (один сразу, один в конце пачки) вместо 10 параллельных fork/exec.

## Supervised peers: naive/mieru (v1.3.0)

[naiveproxy](https://github.com/klzgrad/naiveproxy) (`klzgrad/naiveproxy`) и [mieru](https://github.com/enfein/mieru) (`enfein/mieru`) — protocol-specific прокси, которые не нативны ни одному из встроенных ядер. Sign-craze запускает их как daemon, sing-box подключается через socks5-outbound:

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

Детальный контракт lifecycle и форматы URL: [BEHAVIOR_SPEC.md](BEHAVIOR_SPEC.md) §7-8.

## Hardening (v1.4.0)

Пост-аудитный hardening по 3 параллельным Explore-аудитам (firewall, cores, build/CI):

- **iptables-restore batching**: `BatchBuilder` + `RestoreBatch` + `Flush` — собирает правила в один blob и применяет через `iptables-restore --noflush`. Сокращение fork/exec 24 → 3 на slow MIPS при `applyPolicy`/`applyFull`.
- **WAN cache**: `WANIface` кэшируется, `InvalidateWANCache` при изменении сети. Watchdog tick больше не fork-ит `ip route` каждые 30s.
- **Watchdog coverage**: REDIRECT (nat PREROUTING) и DPI FORWARD chain помимо TPROXY mangle.
- **Reproducible builds**: `-buildid=` + `-trimpath` + `SOURCE_DATE_EPOCH` в release.yml. Артефакт байт-в-байт одинаковый при идентичном входе.
- **[Cosign](https://www.sigstore.dev/) keyless OIDC**: `.sig` + `.pem` рядом с каждым артефактом, идентичность подтверждается через GitHub OIDC issuer + workflow path.
- **[SLSA](https://slsa.dev/) build provenance**: `actions/attest-build-provenance@v4` (после v1.4.1) — проверка через `gh attestation verify`.
- **PID-файлы**: `atomicfs.WriteFileAtomic` + `processAlive` с match `/proc/<pid>/comm` (15-байтовый truncation учтён) → PID-reuse guard.
- **`--diag --json`**: machine-parseable вывод для скриптов и мониторинга.
- **WebSocket keepalive**: ping 30s (RFC 6455 §5.5.2) — соединения за NAT/UPnP не падают.
- **Embed assets**: `Cache-Control: immutable` для статики, `no-cache` для index.html.
- **xray geo guard**: early-check `geoip.dat`/`geosite.dat` с подсказкой `--update-geo --core xray`; `GeoAssetsDir=""` = skip для preview/Validator.
- **SHA256 streaming в geo/srs**: OOM-guard для 30–100 MB `.srs`.
- **Регрессии** (из `tasks/lessons.md`): `TestApplier_Apply_Policy_BypassBeforeTProxy`, `TestService_DefaultShimPath_IsS99`, `TestRender_Geosite_NoDat_ReturnsError`, `FuzzMieruWireProto`.

## Идентификаторы

Полный канонический реестр — [OWNERSHIP.md](OWNERSHIP.md) §5.

| Элемент | Значение |
| ----- | -------- |
| fwmark | `0x53` (= 83 dec) |
| Routing table | `83` |
| Prefix цепочек iptables | `signcraze_*` |
| Prefix ipset-наборов | `signcraze_*` |
| [ip rule](https://www.man7.org/linux/man-pages/man8/ip-rule.8.html) приоритет | `32765` |

---

## Исторические решения

| Версия | Решение | Подробности |
|--------|---------|-------------|
| v0.8.0 | Packet path: TPROXY/REDIRECT через mangle POSTROUTING (policy+DPI) | Заменено на FORWARD в v1.1.0; CHANGELOG v0.8.0, ADR-0007 |
| v0.8.0 | Auto-update hostlist (горутина-тикер в --service-watchdog) | Архитектура актуальна; детали в CHANGELOG v0.8.0 |
| v0.8.0 | Reapply throttle (маркер mtime + Reconcile, 5 с) | Актуально; детали в CHANGELOG v0.8.0 |
| v0.8.1 | DNS fallback chain (resilientResolver: system→1.1.1.1→9.9.9.9→8.8.8.8) | Актуально; детали в CHANGELOG v0.8.1 |
| v0.8.2 | Hostlist apply rule (предикат hostlistShouldApply) | Актуально; детали в CHANGELOG v0.8.2 |
| v0.8.3 | Routing config migration TUN→TPROXY (фильтрация stale tun inbound) | Актуально; детали в CHANGELOG v0.8.3 |

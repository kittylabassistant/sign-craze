# Routing в sign-craze

## Pipeline и порты

```plain
routing.json
  └─ ensureConfigFreshForCore()    internal/cli/deps.go
       └─ buildEffectiveModel()    internal/singbox/render_rules.go:102
            ├─ BaseRules (hardcoded): resolve + sniff + hijack-dns
            └─ UserRules (cfg.Rules[], final = cfg.Final)
                 └─ c.RenderConfig(CoreRenderParams{RoutingConfig: ...})
                      ├─ sing-box: tun.json.tmpl → config.json
                      ├─ xray:     renderXrayRoutingRules → xray/config.json
                      └─ mihomo:   rules/rule-providers → mihomo/config.yaml
```

Файлы на роутере:

| Файл | Путь |
| --- | --- |
| Routing config | `/opt/etc/sign-craze/routing.json` (JSON-поле `"version": 1`, Go-константа `SchemaVersion`, `internal/routing/store.go:25`). **Core-agnostic** — единый файл для всех ядер. |
| Sing-box config | `/opt/etc/sign-craze/config.json` (`internal/singbox/coreadapter.go:25`) |
| Xray config | `/opt/etc/sign-craze/xray/config.json` (`internal/core/xray/coreadapter.go`) |
| Mihomo config | `/opt/etc/sign-craze/mihomo/config.yaml` (`internal/core/mihomo/coreadapter.go`) |

Порты:

| Порт | Назначение |
| --- | --- |
| :9090 | [Zashboard](https://github.com/Zephyruso/zashboard) UI (proxy → :9094) |
| :9091 | Admin REST API |
| :9092 | Routing Editor UI |
| :9094 | sing-box clash\_api (internal) |

---

## Translation matrix: RouteRule → нативный формат ядра

Один `RouteRule` из `routing.json` транслируется в нативные конструкции каждого ядра:

| Поле `RouteRule` | [sing-box](https://sing-box.sagernet.org/) | [xray](https://xtls.github.io/) | [mihomo](https://wiki.metacubex.one/) |
|---|---|---|---|
| `domain: ["x.com"]` | `"domain": ["x.com"]` (1:1) | `"domain": ["x.com"]` | `DOMAIN,x.com,<action>` |
| `domain_suffix: [".x.com"]` | `"domain_suffix": [".x.com"]` | `"domain": ["x.com"]` | `DOMAIN-SUFFIX,x.com,<action>` |
| `domain_keyword: ["x"]` | `"domain_keyword": ["x"]` | `"domain": ["keyword:x"]` | `DOMAIN-KEYWORD,x,<action>` |
| `domain_regex: ["^x\\."]` | `"domain_regex": [...]` | `"domain": ["regexp:^x\\."]` | (не поддерживается, warning) |
| `ip_cidr: ["10.0.0.0/8"]` | `"ip_cidr": [...]` | `"ip": ["10.0.0.0/8"]` | `IP-CIDR,10.0.0.0/8,<action>,no-resolve` |
| `port: [80, 443]` | `"port": [80, 443]` | `"port": "80,443"` | `DST-PORT,80,<action>` (по одному) |
| `port_range: ["1000:2000"]` | `"port_range": [...]` | `"port": "1000-2000"` | `DST-PORT,1000-2000,<action>` |
| `network: "udp"` | `"network": "udp"` | `"network": "udp"` | `NETWORK,udp,<action>` |
| `protocol: ["bittorrent"]` | `"protocol": ["bittorrent"]` | `"protocol": ["bittorrent"]` | (warning, skip) |
| `source_ip_cidr: [...]` | `"source_ip_cidr": [...]` | `"source": [...]` | (не поддерживается, warning) |
| `inbound: ["tproxy-in"]` | `"inbound": [...]` | `"inboundTag": [...]` | (игнорируется) |
| `rule_set: ["geosite-youtube"]` | `"rule_set": ["geosite-youtube"]` | translation → `"domain": ["geosite:youtube"]` | `RULE-SET,geosite-youtube,<action>` + rule-providers |
| `rule_set: ["geoip-ru"]` | `"rule_set": ["geoip-ru"]` | translation → `"ip": ["geoip:ru"]` | `RULE-SET,geoip-ru,<action>` + rule-providers |
| `rule_set: ["refilter-blocked-domains"]` | `"rule_set": [...]` | warning — нет .dat-эквивалента | `RULE-SET,refilter-blocked-domains,<action>` + .mrs rule-provider |
| `action: "reject"` / outbound `block` | `"action": "reject"` | outbound `blackhole` автодобавляется | `,REJECT` |
| `action: "route"`, outbound `direct` | `"outbound": "direct"` | `"outboundTag": "direct"` (freedom автодобавляется) | `,DIRECT` |
| Final rule | `"final": "<tag>"` | последнее правило без матчеров | `MATCH,<tag>` в конце `rules:` |

### Per-core preset URLs

При `POST /api/presets/<name>/apply` URL rule_set резолвится под формат активного ядра:

| Ядро | Формат | Источник |
|---|---|---|
| sing-box | `.srs` | SagerNet/sing-geosite, SagerNet/sing-geoip |
| mihomo | `.mrs` | MetaCubeX/meta-rules-dat |
| xray | translation в matcher | URL не нужен — geosite:/geoip: данные встроены в xray |

Если переключили ядро и в `routing.json` остались URL старого формата — Validate покажет предупреждение. Чтобы обновить URL: повторно применить пресет (кнопка **Пресеты ▾** в UI или `POST /api/presets/<name>/apply`).

---

## UI Editor :9092

**Запуск:** `sign-craze --ui on`. Доступ: `http://<router-ip>:9092` без аутентификации.

**Вкладки:**

- **Inbounds** — tun, [tproxy](https://www.kernel.org/doc/html/latest/networking/tproxy.html), mixed, socks, http
- **Outbounds** — vless / vmess / trojan / shadowsocks / direct / block
- **Routing** — список правил с drag-n-drop reorder; полная схема sing-box rule (domain, ip\_cidr, port, port\_range, protocol, rule\_set, invert)
- **Rule Sets** — кастомные удалённые SRS-источники
- **Preview** — rendered config.json; кнопки Validate / Apply

**Встроенные пресеты** (v1.5.0+) — POST `/api/presets/<name>/apply` или CLI `--preset <name>` при `--install`:

```sh
sign-craze --preset-list                          # список пресетов с описаниями
sign-craze --install --proxy 'vless://...' --preset ru-direct-rest-vpn
```

| Имя | Что делает | Источник SRS |
| --- | --- | --- |
| `sign-craze-default` | bittorrent→direct, youtube→direct, discord→direct, refilter-blocked→{vpn}, final=direct | ruleset-domain-refilter_domains.srs (1andrevich/Re-filter-lists) |
| `block-ads` | geosite-category-ads-all → reject | SagerNet/sing-geosite |
| `ru-direct` | geoip-ru → direct | SagerNet/sing-geoip (`geoip-ru.srs`) |
| `ru-direct-rest-vpn` | geoip-ru → direct; final={vpn} — всё остальное через VPN | SagerNet/sing-geoip (`geoip-ru.srs`) |
| `blocked-vpn` | refilter-blocked-domains → {vpn} | 1andrevich/Re-filter-lists |
| `discord-vpn` | geosite-discord → {vpn} | SagerNet/sing-geosite |
| `torrents-direct` | protocol bittorrent → direct | sniff-based, без SRS |
| `block-bogon-udp` | UDP 135/137–139 → reject | фиксированные порты, без SRS |

Placeholder `{vpn}` резолвится в `state.Outbounds[0].Tag` автоматически.

**REST API** (`:9092`):

```plain
GET    /api/state
GET    /api/inbounds      POST  PUT/{tag}    DELETE/{tag}
GET    /api/outbounds     POST  PUT/{tag}    DELETE/{tag}
GET    /api/rules         POST  PUT/{idx}    DELETE/{idx}
POST   /api/rules/reorder   body={from, to}
GET    /api/rule_sets     POST               DELETE/{tag}
GET    /api/presets       POST /api/presets/{name}/apply
POST   /api/validate      body=RoutingConfig
GET    /api/preview                          → rendered config.json
POST   /api/apply                            → {"needs_restart": true}
```

**Apply pipeline:** `POST /api/apply` → `ensureConfigFreshForCore` → `c.RenderConfig` (check встроен) → atomic write конфига активного ядра. Ответ `{"needs_restart": true}`. Reload через SIGHUP не поддерживается — нужен `sign-craze --restart`.

**Validate response** (начиная с v1.0.0):

```json
{
  "ok": true,
  "errors": [],
  "warnings": ["rule_set geosite-category-ads-all: .srs URL несовместим с mihomo, нужен .mrs — повторно примените пресет"]
}
```

`warnings` не блокируют apply. `errors` блокируют.

**Кастомный SRS-источник** (UI "Rule Sets" → Add, или POST `/api/rule_sets`):

```json
{
  "tag": "my-list",
  "type": "remote",
  "format": "binary",
  "url": "https://example.com/my-rules.srs",
  "download_detour": "direct",
  "update_interval": "12h0m0s"
}
```

После добавления тег `my-list` доступен в поле `rule_set` любого правила.

**Persistence:** изменения сохраняются в `/opt/etc/sign-craze/routing.json`. Файлы с `"version": 0` (отсутствующее поле) мигрируют в `"version": 1` автоматически при чтении — вручную исправлять не нужно.

**Disable:** в `state.json` выставить `"routing_ui_enabled": false`.

---

## Workarounds для частных случаев

### Bypass IP/CIDR через ipset

Для единичных адресов или небольших блоков без SRS:

```bash
sign-craze --exclude-add 95.213.0.0/16
sign-craze --exclude-add 5.45.192.0/18
sign-craze --restart
```

Меняет `state.json:Excludes`, регенерирует `config.json`, обновляет [ipset](https://ipset.netfilter.org/) `signcraze_excludes`. Минус: уровень kernel, sing-box про эти IP не знает; список поддерживается вручную.

### Прямая правка routing.json через REST :9092

```bash
# получить текущий RoutingConfig
curl http://router:9092/api/state > /tmp/routing.json

# внести правки, залить обратно через /api/rules или /api/apply
curl -X POST http://router:9092/api/validate -d @/tmp/routing.json
curl -X POST http://router:9092/api/apply
sign-craze --restart
```

Изменения персистентны (пишутся в routing.json). Это штатный путь для автоматизации без UI.

### Режим policy (по устройству, не по geo)

```bash
sign-craze --mode policy
sign-craze --restart
```

В policy mode трафик конкретных устройств/подсетей определяется в web-UI [Keenetic](https://help.keenetic.com/hc/ru). Минус: гранулярность по device, не по geo/домену.

---

## Verification

```bash
# 1. Предварительная валидация конфига
curl -X POST http://router:9092/api/validate -d @routing.json

# 2. Preview rendered конфига (JSON для sing-box/xray, YAML для mihomo)
curl http://router:9092/api/preview | jq '.route.rules'    # sing-box
curl http://router:9092/api/preview | jq '.routing.rules'  # xray
curl http://router:9092/api/preview                        # mihomo (YAML)

# 3. Apply (sing-box check встроен в WriteConfig)
curl -X POST http://router:9092/api/apply
# ответ: {"needs_restart": true}

# 4. Перезапуск
sign-craze --restart

# 5. Проверка состояния
sign-craze --status
```

После `--restart` убедиться в логах ядра, что `rule_set` успешно скачан (для sing-box/mihomo) или translation применена (для xray):

```bash
tail -f /opt/var/log/sign-craze/sign-craze.log | grep -E 'rule_set|geosite|geoip'
```

---

## Recipes

Детальный пример «RU-трафик через прямое соединение без SRS-пресетов» — см. [Recipe-RU-Direct](Recipe-RU-Direct).

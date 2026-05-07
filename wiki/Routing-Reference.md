# Routing в sign-craze

## Pipeline и порты

```plain
routing.json
  └─ configParamsFromState()       internal/cli/deps.go:136
       └─ buildEffectiveModel()    internal/singbox/render_rules.go:102
            ├─ BaseRules (hardcoded): resolve + sniff + hijack-dns
            │                        internal/singbox/render_rules.go:124-135
            └─ UserRules (cfg.Rules[], final = cfg.Final)
                 └─ tun.json.tmpl ──▶ atomic write ──▶ config.json ──▶ sing-box
```

Файлы на роутере:

| Файл | Путь |
| --- | --- |
| Routing config | `/opt/etc/sign-craze/routing.json` (JSON-поле `"version": 1`, Go-константа `SchemaVersion`, `internal/routing/store.go:25`) |
| Sing-box config | `/opt/etc/sign-craze/config.json` (`internal/singbox/coreadapter.go:25`) |

Порты:

| Порт | Назначение |
| --- | --- |
| :9090 | Zashboard UI (proxy → :9094) |
| :9091 | Admin REST API |
| :9092 | Routing Editor UI |
| :9094 | sing-box clash\_api (internal) |

---

## UI Editor :9092

**Запуск:** `sign-craze --ui on`. Доступ: `http://<router-ip>:9092` без аутентификации.

**Вкладки:**

- **Inbounds** — tun, tproxy, mixed, socks, http
- **Outbounds** — vless / vmess / trojan / shadowsocks / direct / block
- **Routing** — список правил с drag-n-drop reorder; полная схема sing-box rule (domain, ip\_cidr, port, port\_range, protocol, rule\_set, invert)
- **Rule Sets** — кастомные удалённые SRS-источники
- **Preview** — rendered config.json; кнопки Validate / Apply

**Встроенные пресеты** (POST `/api/presets/<name>/apply`):

| Имя | Что делает | Источник SRS |
| --- | --- | --- |
| `sign-craze-default` | bittorrent→direct, youtube→direct, discord→direct, refilter-blocked→{vpn}, final=direct | refilter-domains.srs |
| `block-ads` | geosite-category-ads-all → reject | SagerNet/sing-geosite |
| `ru-direct` | geoip-ru → direct | SagerNet/sing-geoip (`geoip-ru.srs`) |
| `blocked-vpn` | refilter-blocked-domains → {vpn} | 1andrevich/Re-filter-lists |
| `discord-vpn` | geosite-discord → {vpn} | SagerNet/sing-geosite |
| `torrents-direct` | protocol bittorrent → direct | sniff-based, без SRS |
| `block-bogon-udp` | UDP 135/137-139 → reject | фиксированные порты, без SRS |

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

**Apply pipeline:** `POST /api/apply` → `regenerateConfig` → `singbox.WriteConfig` (sing-box check встроен) → atomic write config.json. Ответ `{"needs_restart": true}`. Reload через SIGHUP не поддерживается — нужен `sign-craze --restart`.

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

Меняет `state.json:Excludes`, регенерирует `config.json`, обновляет ipset `signcraze_excludes`. Минус: уровень kernel, sing-box про эти IP не знает; список поддерживается вручную.

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

В policy mode трафик конкретных устройств/подсетей определяется в web-UI Keenetic. Минус: гранулярность по device, не по geo/домену.

---

## Verification

```bash
# 1. Предварительная валидация конфига
curl -X POST http://router:9092/api/validate -d @routing.json

# 2. Preview rendered config.json до apply
curl http://router:9092/api/preview | jq '.route.rules'

# 3. Apply (sing-box check встроен в WriteConfig)
curl -X POST http://router:9092/api/apply
# ответ: {"needs_restart": true}

# 4. Перезапуск
sign-craze --restart

# 5. Проверка состояния
sign-craze --status
```

После `--restart` убедиться в sing-box логах, что `rule_set` успешно скачан:

```bash
tail -f /opt/var/log/sign-craze/sign-craze.log | grep rule_set
```

---

## Recipes

Детальный пример «RU-трафик через прямое соединение без SRS-пресетов» — см. [Recipe-RU-Direct.md](Recipe-RU-Direct.md).

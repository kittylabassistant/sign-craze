# Routing

## Концепция

sign-craze управляет маршрутизацией через единый файл `/opt/etc/sign-craze/routing.json`. Файл редактируется через UI на порту `:9092` либо напрямую в текстовом редакторе. После изменений sign-craze рендерит конфиг **активного прокси-ядра** и перезапускает его.

Начиная с v1.0.0 routing.json является **core-agnostic**: он содержит универсальные правила, которые каждое ядро (sing-box, xray, mihomo) переводит в свой нативный формат при рендеринге. Переключение ядра не требует изменений routing.json.

## Унифицированный routing для всех ядер

Один и тот же файл `routing.json` работает для любого активного ядра. Flow выглядит так:

1. Добавить rule в UI `:9092` (или через REST `POST /api/rules`).
2. Нажать **Apply** (или `POST /api/apply`).
3. sign-craze читает `routing.json`, определяет активное ядро и вызывает его адаптер.
4. Адаптер переводит универсальные правила в нативный формат ядра и записывает конфиг.
5. Запустить `sign-craze --restart`.

Пример: правило `rule_set: ["geosite-youtube"], outbound: "direct"` превращается в:
- **[sing-box](https://sing-box.sagernet.org/)** → `"route": {"rules": [{"rule_set": ["geosite-youtube"], "outbound": "direct"}]}`
- **[xray](https://xtls.github.io/)** → `"routing": {"rules": [{"domain": ["geosite:youtube"], "outboundTag": "direct"}]}`
- **[mihomo](https://wiki.metacubex.one/)** → строка `RULE-SET,geosite-youtube,DIRECT` в секции `rules:` + запись в `rule-providers:`

Правило добавляется один раз — формат определяется ядром автоматически при Apply.

## Порты sign-craze

| Порт | Что слушает | Зачем |
| ------ | ------------- | ------- |
| `:9090` | [Zashboard](https://github.com/Zephyruso/zashboard) SPA + reverse-proxy в sing-box clash_api | Runtime monitoring: proxies, connections, traffic |
| `:9091` | Admin REST | Управление state: ports, excludes, DPI |
| `:9092` | **Routing Editor** (Preact SPA + REST) | **CRUD routing.json, пресеты, validate, apply** |
| `:9094` | sing-box clash_api | Internal (localhost only) |

**Routing-правила редактируются на `:9092`.** Порт `:9090` — только мониторинг, изменять правила там нельзя.

## Pipeline

```plain
/opt/etc/sign-craze/routing.json
           │ (UI :9092 → POST /api/apply)
           ▼
buildEffectiveModel (BaseRules + UserRules + Final)
           │
           ▼
    ┌──────┴──────┬──────────────────┐
    ▼             ▼                  ▼
sing-box       xray              mihomo
config.json    xray/config.json  mihomo/config.yaml
    │             │                  │
    └──────────────▼─────────────────┘
           активное ядро (требуется sign-craze --restart)
```

После нажатия Apply в UI sign-craze применяет изменения в памяти, записывает конфиг **активного ядра** и сигнализирует о необходимости перезапустить его командой `sign-craze --restart`. Без `--restart` новые правила в сетевой трафик не попадут.

## Быстрый рецепт: РФ-сайты напрямую, остальное через VPN

В UI `:9092` применить пресет `ru-direct` (добавит geoip-ru → direct), затем вручную добавить rule_set `geosite-ru` и rule с `outbound=direct`, в секции final выбрать VPN-outbound, нажать Apply. После — выполнить `sign-craze --restart` в SSH. Полный пошаговый рецепт с примерами JSON — в [Recipe-RU-Direct.md](Recipe-RU-Direct.md).

## Встроенные пресеты (v1.5.0+)

Доступны через web UI `:9092` («Пресеты ▾»), REST API (`POST /api/presets/<name>/apply`) и CLI при установке:

```sh
sign-craze --preset-list                          # показать все 8 пресетов с описаниями
sign-craze --install --proxy 'vless://...' --preset ru-direct-rest-vpn
```

Флаг `--preset` применяется в режиме replace (полностью перезаписывает Rules/Final). Без `--preset` routing не трогается.

| Пресет | Что делает |
| ------ | ------------ |
| `sign-craze-default` | bittorrent→direct, youtube→direct, discord→direct, refilter-blocked-domains→{vpn}, final=direct |
| `block-ads` | Блокирует рекламные домены через geosite-category-ads-all → block |
| `ru-direct` | РФ-адреса (geoip-ru) → direct, минует VPN |
| `ru-direct-rest-vpn` | geoip-ru → direct, остальной трафик → VPN (final={vpn}). Инвертированный «всё через VPN, кроме РФ» |
| `blocked-vpn` | Заблокированные домены (refilter-blocked-domains) → прокси-outbound |
| `discord-vpn` | Discord (geosite-discord) → прокси-outbound |
| `torrents-direct` | BitTorrent-трафик → direct, не тратит VPN-квоту |
| `block-bogon-udp` | UDP-порты 135, 137–139 (NetBIOS/SMB) → reject, защита от шума LAN |

## Структура routing.json

Файл содержит три секции:

- **base_rules** — системные правила, управляются знак-craze (не редактировать вручную).
- **user_rules** — пользовательские правила, редактируются через `:9092` или напрямую.
- **final** — default outbound: куда уходит трафик, не совпавший ни с одним правилом.

`buildEffectiveModel` объединяет секции в указанном порядке при рендеринге `config.json`.

## Валидация

Перед применением UI `:9092` запускает встроенный validate: проверяет теги outbound, синтаксис rule_set, дублирующиеся правила. **Ошибки** (невалидный конфиг) блокируют apply. **Предупреждения** (несовместимые конструкции для активного ядра) отображаются в UI, но не блокируют Apply.

Типичные предупреждения при смене ядра:
- `rule_set URL .srs несовместим с mihomo — нужен .mrs` → повторно применить пресет
- `TUN inbound несовместим с xray/mihomo` → TUN inbound игнорируется, используется TProxy
- `refilter rule_set без эквивалента на xray` → правило пропускается для xray

Ручная валидация через API:

```sh
# Validate с показом предупреждений
curl -sX POST http://router:9092/api/validate | jq '{ok, errors, warnings}'
```

Ручная валидация конфига ядра напрямую:

```sh
# sing-box
sing-box check -c /opt/etc/sign-craze/config.json
# xray
xray test -c /opt/etc/sign-craze/xray/config.json
# mihomo
mihomo -t -d /opt/etc/sign-craze/mihomo/
```

## Применение изменений

```sh
# Из SSH на роутере:
sign-craze --restart
```

Команда атомарно останавливает sing-box, применяет новый `config.json` и запускает его снова. Watchdog (`--service-watchdog`) пересинхронизирует firewall-правила автоматически после рестарта.

## Где детальная документация

- [Routing-Reference.md](Routing-Reference.md) — полная инструкция по routing.json и API
- [Recipes.md](Recipes.md) — пошаговые рецепты для типовых сценариев

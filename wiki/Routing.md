# Routing

## Концепция

sign-craze управляет sing-box-маршрутизацией через файл `/opt/etc/sign-craze/routing.json`. Файл редактируется через UI на порту `:9092` либо напрямую в текстовом редакторе. После изменений sign-craze рендерит итоговый `/opt/etc/sign-craze/config.json` через шаблон `tun.json.tmpl` и перезапускает sing-box.

## Порты sign-craze

| Порт | Что слушает | Зачем |
| ------ | ------------- | ------- |
| `:9090` | Zashboard SPA + reverse-proxy в sing-box clash_api | Runtime monitoring: proxies, connections, traffic |
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
tun.json.tmpl → /opt/etc/sign-craze/config.json
           │
           ▼
sing-box (требуется sign-craze --restart)
```

После нажатия Apply в UI sign-craze применяет изменения в памяти, записывает `config.json` и сигнализирует о необходимости перезапустить sing-box командой `sign-craze --restart`. Без `--restart` новые правила в сетевой трафик не попадут.

## Быстрый рецепт: РФ-сайты напрямую, остальное через VPN

В UI `:9092` применить пресет `ru-direct` (добавит geoip-ru → direct), затем вручную добавить rule_set `geosite-ru` и rule с `outbound=direct`, в секции final выбрать VPN-outbound, нажать Apply. После — выполнить `sign-craze --restart` в SSH. Полный пошаговый рецепт с примерами JSON — в [Recipe-RU-Direct.md](Recipe-RU-Direct.md).

## Встроенные пресеты

| Пресет | Что делает |
| ------ | ------------ |
| `sign-craze-default` | Базовые правила: LAN direct, loopback direct, DNS через прокси |
| `block-ads` | Блокирует рекламные домены через geosite-category-ads-all → block |
| `ru-direct` | РФ-адреса (geoip-ru + geosite-ru) → direct, минует VPN |
| `blocked-vpn` | Заблокированные сайты из geosite-blocked → прокси-outbound |
| `discord-vpn` | Discord (geosite-discord) → прокси-outbound |
| `torrents-direct` | BitTorrent-трафик → direct, не тратит VPN-квоту |
| `block-bogon-udp` | Bogon-диапазоны в UDP → drop, защита от спуфинга |

## Структура routing.json

Файл содержит три секции:

- **base_rules** — системные правила, управляются знак-craze (не редактировать вручную).
- **user_rules** — пользовательские правила, редактируются через `:9092` или напрямую.
- **final** — default outbound: куда уходит трафик, не совпавший ни с одним правилом.

`buildEffectiveModel` объединяет секции в указанном порядке при рендеринге `config.json`.

## Валидация

Перед применением UI `:9092` запускает встроенный validate: проверяет теги outbound, синтаксис rule_set, дублирующиеся правила. Ошибки выводятся до записи файла — apply не происходит при невалидном состоянии.

Ручная валидация через CLI:

```sh
sing-box check -c /opt/etc/sign-craze/config.json
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

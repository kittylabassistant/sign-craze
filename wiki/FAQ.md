# FAQ

> Если вашего вопроса здесь нет — откройте [issue](https://github.com/kittylabassistant/sign-craze/issues) или предложите PR в каталог `wiki/` основного репозитория.

## Общие

### В чём отличие режимов `policy` и `full`?

- **`policy` (по умолчанию)** — sign-craze создаёт IP-policy `sign-craze` в Keenetic через RCI, читает присвоенный Keenetic fwmark и ставит iptables-правила с фильтром по этому mark. **Выбор устройств** — через штатный web-UI Keenetic «Приоритеты подключений». Никакого собственного ipset не требуется. Безопасный default.
- **`full` (legacy)** — sign-craze управляет всем: создаёт ipset `signcraze_ipv4`/`signcraze_ipv6`/`signcraze_excludes`, ставит свой fwmark `0x53` и маршрутизирует **весь** транзитный трафик через TUN. Полезен на не-Keenetic-роутерах или если нужен fine-grained контроль через ipset.

Переключение: `sign-craze --mode policy --restart` или `sign-craze --mode full --restart`.

### Почему через несколько часов sign-craze перестаёт проксировать?

**Корневая причина**: ndm Keenetic периодически пересобирает FORWARD chain в iptables и стирает sign-craze ACCEPT правила для интерфейса `signbox-tun`. Без них клиентский трафик попадает под `FORWARD policy DROP`.

**Решение** (с Phase 11): standalone firewall watchdog работает в отдельном процессе `sign-craze --service-watchdog`, запускается из init.d shim на boot. Каждые 30 сек проверяет критичные правила и реапплаит при пропаже. Также есть event-driven `--reapply` через NDM netfilter.d hook (`/opt/etc/ndm/netfilter.d/50-sign-craze`) для мгновенной реакции.

Проверить что watchdog жив:

```sh
ps | grep service-watchdog
cat /opt/var/run/sign-craze-watchdog.pid
grep watchdog /opt/var/log/sign-craze/sign-craze.log | tail -10
```

Если процесса нет — переустановить shim через `sign-craze --reinstall` или вручную перегенерировать через `sign-craze --install`.

## DPI bypass

### Можно ли пускать через nfqws2 только Discord/YouTube, а не весь трафик?

Да, через `--dpi-targets`. nfqws2 поддерживает флаг `--hostlist=<file>` — desync применяется только к соединениям где SNI/hostname matches файл. Все остальные пакеты идут passthrough.

```sh
sign-craze --dpi on
sign-craze --dpi-targets discord.com,discordapp.com,youtube.com,googlevideo.com
sign-craze --restart
```

Готовые пресеты доступны через Web UI или API:

```sh
curl -u admin:<пароль> -X POST http://<router>:9091/api/dpi/presets/discord-youtube/apply
```

### Сколько ресурсов потребляет nfqws2 на MIPS-роутере?

| Сценарий | KN-1410 (580MHz softfloat) | KN-Giga (880MHz) |
|----------|---------------------------|-------------------|
| Без DPI (только sing-box TUN) | ~50-70 Mbps | ~150-200 Mbps |
| DPI all traffic, 15 Mbps | 12-18% CPU | 5-8% CPU |
| DPI all traffic, 40 Mbps | 30-40% CPU | 12-18% CPU |
| DPI all traffic cap | ~25-35 Mbps | ~80-100 Mbps |
| DPI selective (`--dpi-targets`) | NFQUEUE по-прежнему ловит весь TCP, экономия 15-25% CPU за счёт пропуска desync для не-match SNI |

RAM: nfqws2 ~3 MB RSS, sing-box 20-30 MB, sign-craze + watchdog ~10 MB. Итого ~35-45 MB, в бюджете 128 MB.

### Почему `--dpi-targets` не помогает для Discord voice/video?

Discord voice использует UDP/QUIC без TLS SNI в открытом виде — фильтр по hostname не срабатывает. Для voice/video нужны другие техники (например, маршрутизация через прокси-outbound через geosite-discord rule в sing-box).

## Web UI

### Какие порты открывает sign-craze?

- `9090` — Zashboard SPA (Clash-совместимый дашборд)
- `9091` — admin REST API (статус, конфиг, порты, исключения, DPI targets)
- `9092` — Routing Editor SPA (правила маршрутизации, geosite/geoip пресеты)

Все три требуют Basic Auth: логин `admin`, пароль из `/opt/etc/sign-craze/admin.creds` (генерируется при первом `--ui on`).

Запуск: `sign-craze --ui on`. Standalone процесс, держится до SIGTERM. Watchdog работает независимо в `--service-watchdog`.

## Установка / обновление

### Где живёт state.json?

`/opt/etc/sign-craze/state.json`, права `0600` (содержит outbound credentials). Атомарная запись через temp + rename. flock защищает от TOCTOU между web API и CLI.

### Как обновить sign-craze без потери конфига?

```sh
sign-craze --update
sign-craze --restart
```

`--update` скачивает последний релиз с GitHub, проверяет SHA256, атомарно заменяет `/opt/sbin/sign-craze`. Конфиг и state не трогает.

---

## См. также

- [Home](Home) — обзор проекта
- [Installation](Installation) — пошаговая установка
- [Releases](https://github.com/kittylabassistant/sign-craze/releases) — последние версии
- [Issues](https://github.com/kittylabassistant/sign-craze/issues) — задать вопрос

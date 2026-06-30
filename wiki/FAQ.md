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

### Работает ли DPI/nfqws2 из коробки?

Нет. После `--install` / `--install-auto` запущен только `sing-box` (режим `policy`), `nfqws2` **не скачан**, NFQUEUE-правила в iptables не добавлены, `state.json` хранит `dpi_enabled=false`. DPI-обход — opt-in.

Минимум для активации:

```sh
sign-craze --dpi on        # качает nfqws2 в /opt/sbin/, генерит /opt/etc/sign-craze/nfqws2.conf, ставит DPIEnabled=true
sign-craze --restart       # applier добавляет NFQUEUE-правила, lifecycle поднимает nfqws2
```

> **nfqws2 загружается из репозитория `github.com/nfqws/nfqws2-keenetic`** в формате Entware `.ipk` (актуальная версия: v1.2.3). Распаковка `.ipk` (outer tar.gz → `data.tar.gz` → бинарь) выполняется автоматически. Если скачивание падает с 404 — убедитесь, что бинарь sign-craze актуальный (`sign-craze --update`).

После `--dpi on` состояние сохраняется в `state.json` и переживает ребуты — init.d shim `/opt/etc/init.d/S99signcraze` поднимет `nfqws2` автоматически.

Для экономии CPU на MIPS-роутерах рекомендуется selective режим (см. следующий вопрос):

```sh
sign-craze --dpi-targets discord.com,youtube.com,googlevideo.com
sign-craze --restart
```

Иначе `nfqws2` обрабатывает весь TCP/UDP — packet-copy overhead до ~30% CPU при 40 Mbps (см. таблицу ниже).

### Можно ли пускать через nfqws2 только Discord/YouTube, а не весь трафик?

Да, через `--dpi-targets`. nfqws2 поддерживает флаг `--hostlist=<file>` — desync применяется только к соединениям где SNI/hostname matches файл. Все остальные пакеты идут passthrough.

```sh
sign-craze --dpi on
sign-craze --dpi-targets discord.com,discordapp.com,youtube.com,googlevideo.com
sign-craze --restart
```

Готовые пресеты доступны через Web UI или API:

```sh
curl -X POST http://<router>:9091/api/dpi/presets/discord-youtube/apply
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

## Мульти-ядерность

### Что такое мульти-ядерность и зачем?

sign-craze поддерживает три взаимозаменяемых прокси-ядра. Выбор зависит от протокола:

| Ядро | Когда использовать |
|---|---|
| **sing-box** (умолчание) | Большинство конфигураций: VLESS/VMess/Trojan/Shadowsocks, TUN-режим на стоковом ядре Keenetic |
| **xray** | PQ-VLESS (постквантовое шифрование), Vision UDP443, xhttp packet-up режимы — сценарии жёсткого DPI |
| **mihomo** | Hysteria2, TUIC, WireGuard — протоколы, нативно реализованные в mihomo/Meta |

Переключение ядра:

```sh
sign-craze --core xray --restart     # переключиться на xray
sign-craze --core sing-box --restart # вернуться обратно
sign-craze --core-list               # показать доступные ядра
```

При переключении `routing.json` сохраняется без изменений — он core-agnostic. Каждое ядро читает один и тот же файл и переводит правила в свой нативный формат.

### Routing UI работает с xray/mihomo одинаково как с sing-box?

Да, начиная с v1.0.0. Web UI `:9092` полностью унифицирован: один и тот же редактор inbounds/outbounds/rules/presets работает для всех трёх ядер. Apply (`POST /api/apply`) генерирует конфиг активного ядра:
- sing-box → `/opt/etc/sign-craze/config.json` (JSON)
- xray → `/opt/etc/sign-craze/xray/config.json` (JSON)
- mihomo → `/opt/etc/sign-craze/mihomo/config.yaml` (YAML)

Preview соответственно отдаёт `application/json` для sing-box/xray и `application/yaml` для mihomo.

Если routing.json содержит конструкции, несовместимые с активным ядром, кнопка **Validate** покажет предупреждения (не ошибки) — Apply при этом не блокируется.

### Переключил ядро, Preview показывает warning — что делать?

Самая частая причина: при переключении с sing-box на mihomo в `routing.json` остались URL с `.srs`-форматом rule_set, которые mihomo не поддерживает (ему нужны `.mrs`).

Решение — повторно применить активный пресет для обновления URL под текущее ядро:

1. Открыть `http://<router>:9092` → вкладка **Routing** → кнопка **Пресеты ▾**.
2. Выбрать нужный пресет (например `sign-craze-default` или `ru-direct`) → нажать **Применить**.
3. Нажать **Apply**.
4. Выполнить `sign-craze --restart`.

Пресет автоматически подставит правильные URL для активного ядра (`.srs` для sing-box, `.mrs` для mihomo, translation в matcher для xray).

Либо через REST API:

```sh
curl -sX POST http://<router>:9092/api/presets/sign-craze-default/apply
curl -sX POST http://<router>:9092/api/apply
sign-craze --restart
```

### Почему пресет block-ads не работает на xray?

Пресет `block-ads` использует rule_set `geosite-category-ads-all` из SagerNet/sing-geosite (`.srs` формат). Xray не использует механизм rule_set — вместо этого sign-craze автоматически транслирует `geosite-category-ads-all` в xray matcher `domain: ["geosite:category-ads-all"]`, что требует наличия `geosite.dat` на роутере.

Если xray у вас работает через встроенные geosite/geoip данные — пресет будет работать. Если нет — Validate покажет warning `refilter rule_set без .dat-эквивалента`.

Для надёжной блокировки рекламы на xray добавьте явное правило через **Routing → + Добавить** с полем `domain_keyword` или `domain_suffix`, либо переключитесь на sing-box (`--core sing-box`) где `.srs` работает нативно.

## Routing

### Как сделать чтобы РФ-сайты не шли через VPN?

Открыть UI `:9092`, применить пресет `ru-direct` (добавляет geoip-ru → direct), затем добавить правило с rule_set `geosite-ru` и outbound=direct, в final поставить VPN-outbound, нажать Apply. После: `sign-craze --restart`. Подробно — [полный рецепт](Recipe-RU-Direct.md).

## Web UI

### Как открыть веб-интерфейс?

Откройте в браузере `http://<ROUTER_IP>:9090/` из локальной сети. Это Zashboard — Clash-совместимый dashboard, который показывает реальное дерево прокси, живые счётчики трафика, активные соединения и логи в реальном времени. Данные берутся напрямую из sing-box через Clash API реверс-прокси (sign-craze проксирует `/proxies`, `/connections`, `/traffic`, `/logs` и пр. на внутренний порт sing-box `9094`). Стриминговые эндпоинты (`/traffic`, `/logs`, `/connections`) передаются без буферизации. Предварительно запустите: `sign-craze --ui on`.

Выбор активного прокси сохраняется через `experimental.cache_file.path` (`/opt/var/lib/sign-craze/cache.db`); поле `store_mode` удалено в sing-box ≥ 1.10.

### Какие порты открывает sign-craze?

- `9090` — Zashboard (Clash-совместимый dashboard, управление прокси, мониторинг трафика)
- `9091` — admin REST API (статус, конфиг, порты, исключения, DPI targets)
- `9092` — Routing Editor SPA (правила маршрутизации, geosite/geoip пресеты)

Все порты работают без аутентификации и доступны только из LAN.

Запуск: `sign-craze --ui on`. Standalone процесс, держится до SIGTERM. Watchdog работает независимо в `--service-watchdog`.

### Почему 9090 не открывается из интернета?

Это by design. Sign-craze добавляет правила в `filter/INPUT` (owner-comment + префикс) — DROP TCP/UDP 9090 с WAN-интерфейса. Локальная сеть имеет доступ. Если из LAN тоже не открывается — проверьте `iptables -nvL INPUT`, убедитесь что ваш интерфейс не определён как WAN ошибочно (см. `sign-craze --diag`).

## Установка / обновление

### Можно ли передать прокси-URL сразу при установке, без интерактивного режима?

Да, через флаг `--proxy <URL>`:

```sh
sign-craze --install --proxy 'vless://...'

# При переустановке (sing-box уже работает):
sign-craze --reinstall --proxy 'vless://...'
sign-craze --restart
```

Флаг принимает те же форматы, что и интерактивный wizard: `vless://`, `vmess://`, `ss://`, `trojan://`, `http://`, `socks5://`. Начиная с v1.3.0 также поддерживаются supervised-peer схемы: `naive+https://` (naiveproxy) и `mieru://` / `mierus://` (mieru) — только sing-box. При `--reinstall` завершение говорит `--restart`, а не `--start` — sing-box не останавливается при переустановке.

### Где живёт state.json?

`/opt/etc/sign-craze/state.json`, права `0600` (содержит outbound credentials). Атомарная запись через temp + rename. flock защищает от TOCTOU между web API и CLI.

### Как обновить sign-craze без потери конфига?

```sh
sign-craze --update
sign-craze --restart
```

`--update` скачивает последний релиз с GitHub, проверяет SHA256, атомарно заменяет `/opt/sbin/sign-craze`. Конфиг и state не трогает.

### Как sign-craze выбирает прокси-ядро?

При `--install --proxy <URL>` sign-craze анализирует схему URL и выбирает
подходящее ядро автоматически:
- `vmess://`, `vless://`, `trojan://`, `ss://`, `ssr://` — sing-box (default).
- PQ-VLESS / Vision UDP443 — xray (более полная поддержка).
- Clash YAML / специфичные outbound'ы — mihomo.
- `naive+https://`, `mieru://`, `mierus://` — sing-box (supervised peers, v1.3.0+; xray/mihomo не поддерживаются). Дополнительно: `--with-naive` для активации naiveproxy-демона.

Явный выбор: `--install --proxy <URL> --core <sing-box|xray|mihomo>`.
Список ядер: `sign-craze --core-list`. Смена ядра в установленной системе:
`--uninstall` → `--install --core <name>`.

---

## См. также

- [Home](Home) — обзор проекта
- [Installation](Installation) — пошаговая установка
- [Releases](https://github.com/kittylabassistant/sign-craze/releases) — последние версии
- [Issues](https://github.com/kittylabassistant/sign-craze/issues) — задать вопрос

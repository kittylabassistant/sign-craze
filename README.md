# sign-craze

[![GitHub Release](https://img.shields.io/github/v/release/kittylabassistant/sign-craze)](https://github.com/kittylabassistant/sign-craze/releases) [![GitHub stars](https://img.shields.io/github/stars/kittylabassistant/sign-craze)](https://github.com/kittylabassistant/sign-craze/stargazers) [![GitHub License](https://img.shields.io/github/license/kittylabassistant/sign-craze)](LICENSE) [![GitHub Wiki](https://img.shields.io/badge/wiki-docs-blue)](https://github.com/kittylabassistant/sign-craze/wiki)

Go-утилита для управления межсетевым экраном на роутерах Keenetic.

Поддерживает три прокси-ядра с единым управлением через Web UI: **sing-box**, **xray** и **mihomo**. Ядро определяется автоматически по URL outbound при `--install`, переключается командой `--core`. Дополнительно - supervised peers **naiveproxy** и **mieru** (process chain через sing-box socks5-outbound). Опционально [nfqws2](https://github.com/nfqws/nfqws2-keenetic) для DPI-обхода.

> [!WARNING]
> Данный материал подготовлен в научно-технических целях. Sign-Craze предназначен для управления межсетевым экраном роутера Keenetic, защищающим домашнюю сеть. Разработчик не несёт ответственности за иное использование утилиты. Перед применением убедитесь, что ваши действия соответствуют законодательству вашей страны.

---

> [!IMPORTANT]
> **Юридические условия использования:**
>
> - **Лицензия.** Проект распространяется на условиях [BSD 3-Clause](LICENSE). Юридически обязывающим является только англоязычный текст в файле `LICENSE`; русский перевод предоставлен исключительно для удобства ознакомления.
> - **Отказ от гарантий.** Программное обеспечение предоставляется «КАК ЕСТЬ» (AS IS), без каких-либо явных или подразумеваемых гарантий.
> - **Ограничение ответственности.** Автор и участники не несут ответственности за любой прямой, косвенный, случайный или последующий ущерб, включая нарушение работы сети, потерю данных или утрату доступа к оборудованию.
> - **Зона ответственности пользователя.** Применять утилиту допустимо только на оборудовании и в сетях, которыми вы владеете либо имеете явное письменное разрешение владельца.
> - **Соответствие законодательству.** Пользователь самостоятельно отвечает за соответствие своих действий законодательству страны пребывания.
> - **Согласие.** Загружая, устанавливая или используя Sign-Craze, вы подтверждаете, что ознакомились с указанными условиями и принимаете их в полном объёме.

---

![sign-craze](img/banner_release.jpg)

## Возможности

- **Мульти-ядерность (v1.0.0)**: поддержка sing-box, xray и mihomo с автодетектом ядра по URL outbound при `--install`. Переключение: `--core <name>`, список: `--core-list`, загрузка: `--core-install <name>`.
- **Supervised peers (v1.3.0)**: naiveproxy и mieru подключаются как daemon на 127.0.0.1, sing-box заворачивает в socks5-outbound (process chain). URL: `naive+https://...`, `mierus://...`. CLI: `--with-naive`, `--update-naive`.
- **Унифицированный Routing Editor (v1.0.0)**: Web UI на `:9092` принимает inbound / outbound / rules / rule_sets и presets для любого активного ядра. Apply регенерирует конфиг нужного ядра без перезапуска процесса UI.
- **Routing UI: пресеты AS-IS/TO-BE + клиентский буфер (v1.1.0)**: «+ Добавить» (аддитивно) и «⟳ Заменить» (очистка Rules + force-set Final). Изменения буферизуются в браузере; Action-bar Сохранить/Отмена. Apply при unsaved требует подтверждения.
- **Светлая тема Routing UI (v1.1.2)**: переключатель ☀/☾, persist через localStorage, дефолт по `prefers-color-scheme`.
- **Per-core presets**: при применении пресета URL rule_sets подбираются автоматически под ядро: `.srs` для sing-box (SagerNet), `.mrs` для mihomo (MetaCubeX), geosite:/geoip: matcher для xray (встроенный `.dat`).
- **Протоколы по ядрам** (актуально для sing-box 1.13+, xray 25.x, mihomo 1.18+; полная таблица ниже):
  - Все три ядра: VLESS Reality, VMess, Trojan, Shadowsocks legacy + 2022, XHTTP basic, транспорты WS/gRPC/QUIC/h2, uTLS-фингерпринт
  - sing-box + mihomo: Hysteria 2 (obfs-salamander, brutal CC), TUIC v5, WireGuard, AnyTLS
  - xray only: Vision UDP443 (`flow=*-udp443`), PQ-VLESS (`mlkem768x25519plus`), XHTTP `stream-up`/`stream-one`/`packet-up`
  - sing-box only через peer-chain: naiveproxy, mieru
- Управление sing-box / xray / mihomo / naive / mieru: установка, запуск, остановка, обновление, откат
- Два режима маршрутизации: `policy` - выборочная маркировка (default), `full` - весь LAN-трафик через прокси. DPI работает в обоих режимах через nfqws2 + NFQUEUE.
- **DPI в mangle FORWARD (v1.1.0)**: `signcraze_dpi_fwd` ловит трафик всех LAN-устройств (не только policy), desync YouTube/Discord работает для TV/гостей/IoT.
- **Защита SSH/admin при policy (v1.1.1)**: LAN-bypass `-d <LAN_IP> -j RETURN` на pos=1 + admin-ports (22/222) bypass; `--uninstall` отвязывает host-policy в startup-config (factory reset больше не нужен).
- **ANSI-цветной CLI и логи (v1.2.0)**: `--status`/`--diag`/`--core-list`/`--install`/`--help` раскрашены; opt-out: `--no-color`, `NO_COLOR`, `TERM=dumb`. Кастомный `colorTextHandler` для slog stderr (файловый JSON не меняется).
- **Hardening (v1.4.0)**: `iptables-restore --noflush` batching (24 fork до 3), WAN cache, watchdog покрывает REDIRECT + DPI FORWARD, reproducible builds (`-buildid=` + `SOURCE_DATE_EPOCH`), cosign keyless OIDC + SLSA provenance, `--diag --json`, WebSocket keepalive 30s, bcrypt cost=10 для MIPS (login 6s до 2s).
- Атомарное применение правил iptables/ipset с гарантированным откатом
- Гео-фильтрация через SRS rule-set (выборочная загрузка по SHA256, streaming)
- Встроенный Web UI (только из LAN): Zashboard `:9090`, admin API `:9091`, Routing Editor `:9092` (vanilla Preact + htm SPA)
- Selective DPI: desync только для выбранных доменов/SNI через `--dpi-targets`
- WAN-фильтр в DPI-правилах: NFQUEUE захватывает только ISP-трафик (`-o $WAN_IFACE`), не трогая LAN-bridge и TUN
- VPN-exclude (`--dpi-exclude-ips`): RETURN перед NFQUEUE для заданных IP - сохраняет TLS-маскировку Reality-handshake к собственному серверу и downstream-VPN-клиентов на том же эндпоинте
- Auto-update hostlist (`--dpi-update-interval 24`): автообновление списка DPI-доменов раз в 24 ч из upstream-источников (zapret, Flowseal discord/youtube); `--dpi-update-now` - принудительный запуск
- Firewall watchdog: автовосстановление iptables-правил каждые 30 с при работающем `--ui on`
- Управление портами и исключениями без перезапуска
- Резервное копирование и восстановление конфигурации
- Диагностический режим (`--diag`, опционально `--diag --json`)

## Поддерживаемые архитектуры

| Платформа | GOARCH | Примечание |
| ----------- | -------- | ----------- |
| Keenetic (MIPS LE) | `mipsle` | GOMIPS=softfloat |
| Keenetic (MIPS BE) | `mips` | GOMIPS=softfloat |
| Keenetic / RPi (ARM 32) | `arm` | GOARM=7 |
| Keenetic Ultra / RPi 4 | `arm64` | |

## Требования

- Роутер Keenetic с установленным [Entware](https://help.keenetic.com/hc/ru/articles/360021214160)
- Доступ к интернету с роутера (для загрузки sing-box при установке)
- Свободное место на `/opt`: минимум 30 МБ

## Установка

> BusyBox `wget` на Keenetic без SSL - нужен `curl` (или `wget-ssl` из Entware): `opkg install curl`.

```sh
# Определить архитектуру автоматически и установить последний релиз
curl -fsSL https://github.com/kittylabassistant/sign-craze/releases/latest/download/install.sh | sh

# Альтернатива: скрипт напрямую с raw.githubusercontent.com (другой CDN,
# может пройти, если releases-домен заблокирован у провайдера или DNS).
curl -fsSL https://raw.githubusercontent.com/kittylabassistant/sign-craze/refs/heads/main/scripts/install.sh | sh

# Запустить установку (интерактивно: запросит URL прокси / outbound)
sign-craze --install

# Или без вопросов: автоопределение из конфигурации роутера
sign-craze --install-auto

# Установка с прокси, DPI и routing-пресетом одной командой:
sign-craze --install --proxy vless://server:443?...#tag --with-dpi --preset sign-craze-default

# Запустить
sign-craze --start
```

> [!NOTE]
> `sing-box` загружается с GitHub Releases во время `--install` и **не входит** в sign-craze.
> `nfqws2` загружается **только при первом `sign-craze --dpi on`** или при `sign-craze --install --with-dpi`. DPI-обход отключён по умолчанию (opt-in). Флаг `--with-dpi` устанавливает nfqws2 + blob-файлы и сразу включает DPI с preset `discord-youtube` (out-of-box работа YouTube + Discord). См. [wiki/FAQ → «Работает ли DPI/nfqws2 из коробки»](https://github.com/kittylabassistant/sign-craze/wiki/FAQ#работает-ли-dpinfqws2-из-коробки).

### Offline-установка (роутер без доступа к GitHub)

Если на роутере нет интернета: скачайте bundle на машине с интернетом, перенесите по `scp` и запустите локально.

На машине с интернетом (укажите arch: `arm64`, `arm7`, `mipsle` или `mips`):

```sh
ARCH=mipsle
wget https://github.com/kittylabassistant/sign-craze/releases/latest/download/signcraze-${ARCH}.tar.gz
scp signcraze-${ARCH}.tar.gz root@192.168.1.1:/tmp/
```

На роутере:

```sh
cd /tmp && tar xzf signcraze-mipsle.tar.gz
cd signcraze-mipsle-bundle && ./install-offline.sh
```

> Только сам бинарь sign-craze ставится offline. `sign-craze --install` всё равно тянет sing-box с GitHub. Для полностью изолированной установки скачайте sing-box tarball отдельно и используйте `sign-craze --install-offline /tmp/sing-box-*.tar.gz`.

## Quick Routing

sign-craze управляет маршрутизацией через файл `/opt/etc/sign-craze/routing.json`. Файл core-agnostic: одни и те же правила работают на sing-box, xray и mihomo - каждое ядро транслирует их в свой натив-формат. Редактирование - через встроенный Web UI на порту 9092 либо через REST API.

### Порты управления

| Порт | Что | Когда использовать |
| ----------- | ----------- | ------------------- |
| `:9090` | Zashboard (мониторинг) | Смотреть live трафик, переключать proxy-selectors |
| `:9091` | Admin REST | Скрипты управления state, excludes, DPI |
| `:9092` | **Routing Editor** | **Редактировать правила маршрутизации** |

### Поднять Routing UI

```bash
sign-craze --ui on
# Открыть http://<router-ip>:9092
```

### Готовые пресеты

В UI на вкладке **Routing → "Пресеты ▾"**: `block-ads`, `ru-direct`, `blocked-vpn`, `discord-vpn`, `torrents-direct`, `block-bogon-udp`, `sign-craze-default`.

URL rule_sets в пресете подбираются автоматически под активное ядро: `.srs` (sing-box), `.mrs` (mihomo), matcher через geosite:/geoip: (xray).

После любых правок нажмите **Apply**, затем выполните `sign-craze --restart`.

> CLI-альтернатива: `sign-craze --install --preset <name>` (см. `--preset-list`).

### Документация

- [wiki/Routing.md](wiki/Routing.md) - обзор routing pipeline
- [wiki/Routing-Reference.md](wiki/Routing-Reference.md) - полная инструкция по routing.json и API
- [wiki/Recipe-RU-Direct.md](wiki/Recipe-RU-Direct.md) - рецепт "РФ direct, остальное VPN"
- [wiki/Recipes.md](wiki/Recipes.md) - индекс всех рецептов

## Мульти-ядерность (v1.0.0)

sign-craze поддерживает три прокси-ядра. Активное ядро задаётся в `state.json` и определяет, какой бинарь запускается и как генерируется конфиг.

### Переключение ядра

```bash
# Посмотреть список и текущее активное
sign-craze --core-list

# Переключить на xray
sign-craze --core xray

# Установить (скачать) ядро, если ещё не установлено
sign-craze --core-install mihomo

# Применить переключение
sign-craze --restart
```

### Поддерживаемые протоколы

Актуально для sing-box 1.13+, xray-core 25.x, mihomo 1.18+. Полная матрица с разбивкой по транспортам, uTLS, ECH, sniffing: [`docs/COMPATIBILITY_MATRIX.md`](docs/COMPATIBILITY_MATRIX.md) (раздел «Матрица протокол × ядро»).

| Протокол / режим | sing-box | xray | mihomo |
|------------------|:--------:|:----:|:------:|
| VLESS Reality (PublicKey + ShortID)   | ✓ | ✓ | ✓ |
| VLESS Vision (`xtls-rprx-vision`, TCP/TLS) | ⚠ | ✓ | ✓ |
| VLESS Vision UDP443 (`*-udp443`)      | ✗ | ✓ | ✗ |
| VLESS XHTTP (basic, без `mode`)       | ✓ | ✓ | ✓ |
| VLESS XHTTP `stream-up` / `stream-one` / `packet-up` | ✗ | ✓ | ✓ |
| PQ-VLESS (`mlkem768x25519plus`)       | ⚠ | ✓ | ✗ |
| VMess (AEAD)                          | ✓ | ✓ | ✓ |
| Trojan                                | ✓ | ✓ | ✓ |
| Shadowsocks legacy (AEAD)             | ✓ | ✓ | ✓ |
| Shadowsocks 2022 (`2022-blake3-*`)    | ✓ | ✓ | ✓ |
| Hysteria 2 (obfs-salamander, brutal CC) | ✓ | ✗ | ✓ |
| TUIC v5 (unified)                     | ✓ | ✗ | ✓ |
| WireGuard outbound                    | ✓ | ✗ | ✓ |
| AnyTLS                                | ✓ | ✗ | ✓ |
| Transport WebSocket / gRPC / QUIC / h2 | ✓ | ✓ | ✓ |
| uTLS fingerprint (chrome/firefox/safari/ios/random) | ✓ | ✓ | ✓ |
| ECH (Encrypted ClientHello)           | ⚠ | ⚠ | ⚠ |
| Socks5 / HTTP CONNECT                 | ✓ | ✓ | ✓ |
| naiveproxy (supervised peer)          | ✓ ¹ | ✗ | ✗ |
| mieru (supervised peer)               | ✓ ¹ | ✗ | ✗ |

> ✓ - полная поддержка · ⚠ - частичная / экспериментальная (см. примечания в COMPATIBILITY_MATRIX) · ✗ - не поддерживается, `Validate` отвергнет конфиг.
>
> ¹ naive/mieru - supervised peer (process chain). sign-craze запускает daemon на `127.0.0.1`, sing-box подключается через socks5-outbound. xray/mihomo явно отклоняют такие конфиги с подсказкой `--core sing-box`. mips (big-endian) для naive не поддерживается: klzgrad публикует только linux-mipsel (LE).
>
> **Ограничения mihomo** (источник: `internal/core/mihomo/validate.go`): Vision UDP443 и PQ-VLESS отклоняются с подсказкой `--core xray`. **Ограничения xray** (источник: `internal/core/xray/validate.go`): TUIC v5, WireGuard, Hysteria 2 отклоняются с подсказкой `--core sing-box` / `--core mihomo`.

### Унифицированный Routing Editor

Файл `/opt/etc/sign-craze/routing.json` core-agnostic: один набор правил работает на всех ядрах. Web UI `:9092` принимает изменения и Apply нажимает конфиг активного ядра:

- sing-box → `/opt/etc/sign-craze/config.json`
- xray → `/opt/etc/sign-craze/xray/config.json`
- mihomo → `/opt/etc/sign-craze/mihomo/config.yaml`

Несовместимые конструкции (например `.srs` URL при активном mihomo) отображаются как предупреждения в `apiValidate`, но не блокируют Apply и видны в UI.

## DPI: auto-update hostlist и VPN-исключения (v0.8.0)

### Auto-update hostlist

Включить автообновление списка DPI-доменов раз в 24 ч из upstream:

```bash
# Задать источники (по умолчанию уже включены zapret + Flowseal discord/youtube)
sign-craze --dpi-update-urls \
  https://raw.githubusercontent.com/bol-van/zapret/master/ipset/zapret-hosts-user.txt.example,\
https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-youtube.txt,\
https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-discord.txt

# Включить авто-обновление раз в сутки
sign-craze --dpi-update-interval 24

# Обновить немедленно
sign-craze --dpi-update-now
```

### VPN-exclude (Reality/VLESS)

Защита TLS-маскировки Reality-handshake: NFQUEUE не трогает трафик к указанным IP.
Применяется, если sing-box подключается к собственному Reality-серверу или downstream-клиенты на роутере используют тот же VPN-эндпоинт.

```bash
# Добавить IP VPN-сервера в исключения
sign-craze --dpi-exclude-ips 203.0.113.1,2001:db8::1

# Проверить список
sign-craze --dpi-exclude-ips-list

# Применить изменения
sign-craze --restart
```

## Web UI

**Web UI** (только из LAN):

- **9090** - Zashboard (управление прокси, мониторинг трафика, Clash-совместимый API). Откройте `http://<ROUTER_LAN_IP>:9090/` в браузере.
- **9091** - admin REST API sign-craze (статус, конфиг, порты, исключения, DPI targets).
- **9092** - Routing Editor SPA (визуальный редактор правил маршрутизации).

Порты 9090/9091/9092 слушают на `0.0.0.0`; правила в `filter/INPUT` (owner-comment + префикс) блокируют Zashboard :9090 с WAN-интерфейса. Из локальной сети доступ открыт без аутентификации.

Запуск: `sign-craze --ui on`.

## Команды

```bash
sign-craze --install            Установить sing-box + правила iptables
sign-craze --install-auto       Установить без интерактивных подсказок
sign-craze --install-offline <путь>  Установить из локального бинаря
sign-craze --install --with-dpi      Установить + включить nfqws2 с preset discord-youtube
sign-craze --install --preset <name>      Установить + применить routing-preset (см. --preset-list)
sign-craze --preset-list                  Показать встроенные routing-пресеты (8 шт.)

sign-craze --start              Применить правила + запустить sing-box
sign-craze --stop               Остановить + убрать правила iptables
sign-craze --restart / -r       Перезапуск (stop + start)
sign-craze --status  / -s       Показать состояние сервисов

sign-craze --update  / -u       Обновить sign-craze
sign-craze --update-geo / -g    Обновить гео-файлы (SRS rule-set)
sign-craze --update-core        Обновить бинарь активного ядра (sing-box/xray/mihomo)
sign-craze --update-naive       Обновить бинарь naiveproxy (v1.3.0+)
sign-craze --dpi-update         Переустановить актуальную версию nfqws2
sign-craze --reinstall          Переустановить sign-craze поверх существующей (сохраняет state.json)

sign-craze --core-list          Список зарегистрированных ядер (sing-box/xray/mihomo) и активное
sign-craze --core <name>        Переключить активное ядро; routing.json сохраняется, конфиг пересобирается (требует --restart)
sign-craze --core-install <name> Скачать и установить указанное ядро (sing-box/xray/mihomo)
sign-craze --install --with-naive  Установка + развёртывание naiveproxy daemon (v1.3.0+)

sign-craze --config-backup      Создать архив state.json в /opt/var/lib/sign-craze
sign-craze --config-restore <путь> Восстановить конфиг из архива

sign-craze --mode policy|full        Переключить режим маршрутизации
                                     (Legacy-имена `proxy`/`dpi`/`hybrid` мигрируются в `policy` с предупреждением.)
sign-craze --dpi on|off              Включить / выключить DPI-обход (по умолчанию off; первый `on` качает nfqws2)
sign-craze --dpi-strategy <пресет>   Установить стратегию DPI
sign-craze --dpi-targets <домены>    Selective DPI: desync только для указанных SNI (через запятую; clear - сбросить)
sign-craze --dpi-targets-list        Показать текущий список DPI-целей
sign-craze --dpi-exclude-ips <IP>    IP/IPv6-адреса, исключённые из NFQUEUE (Reality VPN-эндпоинты); clear - сбросить
sign-craze --dpi-exclude-ips-list    Показать список IP-исключений DPI
sign-craze --dpi-update-urls <URL>   Источники для auto-update hostlist (через запятую; clear - отключить)
sign-craze --dpi-update-interval <ч> Период авто-обновления hostlist в часах (0 - выкл, рекомендуется 24)
sign-craze --dpi-update-now         Принудительно обновить hostlist из dpi-update-urls прямо сейчас

sign-craze --port-add <порт>    Добавить порт в проксируемый набор
sign-craze --port-del <порт>    Удалить порт
sign-craze --port-list          Показать список портов

sign-craze --exclude-add <ip>   Добавить IP/CIDR в исключения
sign-craze --exclude-del <ip>   Удалить из исключений
sign-craze --exclude-list       Показать исключения

sign-craze --ui on|off          Включить / выключить Web UI (порты 9090/9091/9092)
                                Watchdog firewall активен, пока процесс `--ui on` работает
sign-craze --backup  / -b       Создать резервную копию конфигурации
sign-craze --restore <путь>     Восстановить из резервной копии
sign-craze --diag    / -D       Диагностика (PASS/WARN/FAIL по каждому пункту)
sign-craze --diag --json        Machine-parseable JSON для скриптов/мониторинга (v1.4.0+)
sign-craze --no-color           Отключить ANSI-цвет (также NO_COLOR=1 / TERM=dumb)
sign-craze --uninstall          Полное удаление: sing-box, конфиги, логи, бинарь sign-craze
sign-craze --version / -v       Показать версии sign-craze и sing-box
```

Полное описание каждой команды, инварианты iptables и форматы конфигов: [`BEHAVIOR_SPEC.md`](BEHAVIOR_SPEC.md).

## Сборка из исходников

Go не требуется на хосте: сборка выполняется в контейнере.

```sh
# Все архитектуры + UPX
podman run --rm \
  -v $(pwd):/workspace:z -w /workspace \
  golang:1.25 make build

# Конкретная архитектура
podman run --rm \
  -v $(pwd):/workspace:z -w /workspace \
  -e GOOS=linux -e GOARCH=mipsle -e GOMIPS=softfloat -e CGO_ENABLED=0 \
  golang:1.25 go build -ldflags="-s -w" -o dist/sign-craze-mipsle ./cmd/sign-craze

# Тесты
podman run --rm -v $(pwd):/workspace:z -w /workspace golang:1.25 go test ./...

# Линтер
podman run --rm -v $(pwd):/workspace:z -w /workspace \
  golangci/golangci-lint:v2.12.2 golangci-lint run ./...
```

Целевой размер бинаря после `upx --lzma`: ≤ 4 МБ.

## Архитектура

```plain
cmd/sign-craze/main.go
        │
internal/cli  (диспетчер команд, ANSI-цвет, --no-color)
   ├── internal/core      (registry: sing-box / xray / mihomo)
   │      ├── internal/singbox   (адаптер: конфиг, RenderConfig, CheckConfig)
   │      ├── internal/core/xray  (адаптер: render_rules, translation geosite:/geoip:)
   │      └── internal/core/mihomo (адаптер: rule-providers, YAML-рендер)
   ├── internal/naiveproxy (supervised peer: download/extract/install/lifecycle)
   ├── internal/peer       (supervised-peer scaffolding + mieru core + portalloc)
   ├── internal/dpi       (nfqws2: загрузка, NFQUEUE в mangle FORWARD)
   ├── internal/firewall  (iptables/ipset: tproxy / redirect / hybrid, restore --noflush batch)
   ├── internal/service   (init.d shim S99, lifecycle, PID-файлы через atomicfs)
   ├── internal/geo       (SRS rule-set, ipset-конвертация, SHA256 streaming)
   ├── internal/state     (state.json: ValidCores, Mode, Inbound, Outbounds, NaiveEnabled)
   ├── pkg/types          (RoutingConfig, CoreRenderParams, core-agnostic)
   └── internal/web       (HTTP: admin API :9091 + Routing Editor :9092 + Zashboard :9090)
```

Подробная диаграмма потоков данных: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Структура файлов на роутере

```plain
/opt/sbin/sing-box                        # бинарь активного ядра (или xray/mihomo при --core)
/opt/sbin/sign-craze                      # этот бинарь
/opt/sbin/naive                           # naiveproxy daemon (если --with-naive / v1.3.0+)
/opt/sbin/mieru                           # mieru daemon (если выбран peer mieru)
/opt/etc/sign-craze/config.json           # конфигурация sing-box (или xray/mihomo путь)
/opt/etc/sign-craze/routing.json          # core-agnostic правила маршрутизации
/opt/etc/sign-craze/nfqws2.conf           # конфигурация nfqws2 (если DPI включён)
/opt/etc/sign-craze/dpi-hostlist.txt      # список SNI-целей для Selective DPI (если задан)
/opt/etc/init.d/S99signcraze              # init.d shim (автозапуск, после S51dropbear)
/opt/etc/ndm/netfilter.d/50-sign-craze    # NDM hook (reapply с flock+debounce)
/opt/var/lib/sign-craze/                  # состояние (гео-файлы, бэкапы, cache.db)
/opt/var/log/sign-craze/                  # логи с ротацией (JSON file + цветной stderr)
/opt/var/run/sign-craze-singbox.pid       # PID sing-box (atomic write)
/opt/var/run/sign-craze-nfqws2.pid        # PID nfqws2
/opt/var/run/sign-craze-naive.pid         # PID naiveproxy peer (если включён)
/opt/var/run/sign-craze-reapply.last      # mtime-throttle маркер NDM hook
/opt/var/lock/sign-craze.lock             # эксклюзивная блокировка
```

## Verifying the release

Релизные артефакты подписаны через [Sigstore](https://sigstore.dev/) (keyless OIDC) и
аттестованы через SLSA build provenance. Проверка не требует отдельного ключа.

```bash
# 1. Скачать артефакты нужной архитектуры
gh release download v1.2.3 -p 'sign-craze-mipsle' -p 'sign-craze-mipsle.sig' \
  -p 'sign-craze-mipsle.pem' -p 'sha256sums.txt'

# 2. Проверить SHA-256 целостность
sha256sum -c sha256sums.txt --ignore-missing

# 3. Проверить cosign-подпись (подлинность через Sigstore OIDC)
cosign verify-blob \
  --certificate sign-craze-mipsle.pem \
  --signature sign-craze-mipsle.sig \
  --certificate-identity-regexp 'https://github\.com/kittylabassistant/sign-craze/\.github/workflows/release\.yml@.*' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  sign-craze-mipsle

# 4. Проверить SLSA provenance
gh attestation verify sign-craze-mipsle --repo kittylabassistant/sign-craze
```

Замените `mipsle` на нужную архитектуру: `arm64`, `arm7`, `mips`.

## Контакты

- E-Mail: [kittylabassistant@protonmail.com](mailto:kittylabassistant@protonmail.com)

## Поддержка проекта

- USDT ERC-20: 0x8aEDDa62aE3fd33696585632b6FB213f76599659
- USDT TRC-20: TCM77F2AYZPWTPzMefq6dGr9zzTx7oxGrk

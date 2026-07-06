# sign-craze

[![GitHub Release](https://img.shields.io/github/v/release/kittylabassistant/sign-craze)](https://github.com/kittylabassistant/sign-craze/releases) [![GitHub stars](https://img.shields.io/github/stars/kittylabassistant/sign-craze)](https://github.com/kittylabassistant/sign-craze/stargazers) [![GitHub License](https://img.shields.io/github/license/kittylabassistant/sign-craze)](LICENSE) [![GitHub Wiki](https://img.shields.io/badge/wiki-docs-blue)](https://github.com/kittylabassistant/sign-craze/wiki)

Go-утилита для управления межсетевым экраном на роутерах [Keenetic](https://help.keenetic.com/hc/ru).

Поддерживает три прокси-ядра с единым управлением через Web UI: [**sing-box**](https://sing-box.sagernet.org/), [**xray**](https://xtls.github.io/) и [**mihomo**](https://wiki.metacubex.one/). Ядро определяется автоматически по URL outbound при `--install`, переключается командой `--core`. Дополнительно - supervised peers [**naiveproxy**](https://github.com/klzgrad/naiveproxy) и [**mieru**](https://github.com/enfein/mieru) (process chain через sing-box socks5-outbound). Опционально [nfqws2](https://github.com/nfqws/nfqws2-keenetic) для DPI-обхода.

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

## Содержание

- [TL;DR / Быстрый старт](#tldr--быстрый-старт)
- [Возможности](#возможности)
- [Поддерживаемые архитектуры](#поддерживаемые-архитектуры)
- [Требования](#требования)
- [Установка](#установка)
- [Quick Routing](#quick-routing)
- [Мульти-ядерность](#мульти-ядерность-v100)
- [DPI: auto-update hostlist и VPN-исключения](#dpi-auto-update-hostlist-и-vpn-исключения-v080)
- [Web UI](#web-ui)
- [Команды](#команды)
- [Сборка из исходников](#сборка-из-исходников)
- [Архитектура](#архитектура)
- [Verifying the release](#verifying-the-release)
- [Контакты](#контакты)

## TL;DR / Быстрый старт

Sign-craze управляет firewall-правилами на роутере [Keenetic](https://help.keenetic.com/hc/ru) и запускает прокси-ядро ([sing-box](https://sing-box.sagernet.org/), [xray](https://xtls.github.io/) или [mihomo](https://wiki.metacubex.one/)) для обхода DPI. Установка через `install.sh` ([Entware](https://entware.net/) обязателен):

```sh
# Определить архитектуру и установить последний релиз
curl -fsSL https://github.com/kittylabassistant/sign-craze/releases/latest/download/install.sh | sh

# Настроить (запросит URL прокси, ядро, режим)
sign-craze --install
```

Подробная инструкция: [wiki/Installation](https://github.com/kittylabassistant/sign-craze/wiki) · [Способ 2: offline bundle](#способ-2-offline-bundle-альтернатива).

## Возможности

**Управление прокси-ядром**
- Три ядра: sing-box / xray / mihomo — автодетект по URL outbound при `--install`, переключение `--core <name>`. Supervised peers naiveproxy и mieru через process chain sing-box (CLI: `--with-naive`).
- Полная матрица поддерживаемых протоколов по ядрам: [`docs/COMPATIBILITY_MATRIX.md`](docs/COMPATIBILITY_MATRIX.md). Коротко: [VLESS](https://xtls.github.io/config/) [Reality](https://github.com/XTLS/REALITY) / [VMess](https://xtls.github.io/config/) / Trojan / Shadowsocks — все три ядра; [Hysteria 2](https://v2.hysteria.network/) / [TUIC v5](https://github.com/EAimTY/tuic) / [WireGuard](https://www.wireguard.com/) — sing-box и mihomo; xray-only: Vision UDP443, PQ-VLESS, XHTTP stream-up.

**Маршрутизация**
- Два режима: `policy` (выборочная маркировка) и `full` (весь LAN-трафик через прокси).
- Core-agnostic `routing.json`: единые правила транслируются в натив-форматы sing-box/xray/mihomo. Routing Editor `:9092` (Web UI) с пресетами (`block-ads`, `ru-direct`, `blocked-vpn` и др.), клиентским буфером и режимами AS-IS/TO-BE.
- Гео-фильтрация через [SRS rule-set](https://sing-box.sagernet.org/configuration/rule-set/) (SHA256, streaming); атомарное применение [iptables](https://www.netfilter.org/projects/iptables/index.html)/[ipset](https://ipset.netfilter.org/) с гарантированным откатом.

**DPI-обход (nfqws2)**
- `signcraze_dpi_fwd` в mangle FORWARD: desync для всех LAN-устройств (TV, гости, IoT), а не только policy.
- Selective DPI по SNI (`--dpi-targets`), VPN-exclude (`--dpi-exclude-ips`), auto-update hostlist раз в N часов из upstream.

**Web UI и диагностика**
- Только из LAN: [Zashboard](https://github.com/Zephyruso/zashboard) `:9090`, admin REST `:9091`, Routing Editor `:9092`. Без HTTP-аутентификации by design; WAN-доступ блокируется NDM INPUT DROP.
- ANSI-цветной CLI (`--status`, `--diag`, `--core-list`); `--diag --json` для скриптов.

**Безопасность релизов**
- Reproducible builds, [cosign](https://www.sigstore.dev/) keyless OIDC + [SLSA](https://slsa.dev/) provenance, SHA-256 checksums. Firewall watchdog: автовосстановление правил каждые 30 с.

## Поддерживаемые архитектуры

Поддерживаются `mipsle` и `mips` ([GOMIPS](https://pkg.go.dev/cmd/go#hdr-Environment_variables)=softfloat), `arm` (GOARM=7) и `arm64`. Полная матрица совместимости по железу, прошивкам и Entware-суффиксам: [`docs/COMPATIBILITY_MATRIX.md`](docs/COMPATIBILITY_MATRIX.md).

## Требования

- Роутер Keenetic с установленным [Entware](https://help.keenetic.com/hc/ru/articles/360021214160)
- Доступ к интернету с роутера (для загрузки sing-box при установке)
- Свободное место на `/opt`: минимум 30 МБ
- Пакеты `iptables` и `ipset` — нужны для firewall-правил, поставьте вручную перед установкой: `opkg install iptables ipset`

## Установка

### Способ 1: curl | sh (рекомендуется)

> [BusyBox](https://busybox.net/) `wget` на Keenetic без SSL - нужен `curl` (или `wget-ssl` из Entware): `opkg install curl`.

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

### Способ 2: offline bundle (альтернатива)

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

Полная матрица с разбивкой по транспортам, uTLS, ECH и sniffing: [`docs/COMPATIBILITY_MATRIX.md`](docs/COMPATIBILITY_MATRIX.md). Коротко (актуально на 19.06.2026: sing-box 1.13.13, xray-core 26.3.27, mihomo 1.19.27):

- **Все три ядра:** VLESS Reality, VMess, Trojan, Shadowsocks (legacy + 2022), XHTTP basic, транспорты WS/gRPC/QUIC/h2, uTLS-фингерпринт.
- **sing-box + mihomo:** Hysteria 2, TUIC v5, WireGuard, AnyTLS.
- **xray only:** Vision UDP443, PQ-VLESS (`mlkem768x25519plus`), XHTTP `stream-up`/`stream-one`.
- **sing-box only (peer-chain):** naiveproxy, mieru.
- На mips/mipsle используется pinned custom-build xray v25.12.8 (регрессия в 26.3.27).

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

Защита TLS-маскировки Reality-handshake: [NFQUEUE](https://www.netfilter.org/projects/libnetfilter_queue/) не трогает трафик к указанным IP.
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

Серверы биндятся на LAN-bridge IP (`netif.DetectLANAddr`); если LAN-адрес определить не удаётся, `--ui-daemon` отказывается стартовать (fail-secure: UI без auth/TLS не должен оказаться на `0.0.0.0`). Дополнительно правила в `filter/INPUT` (owner-comment + префикс) блокируют порты 9090/9091/9092 с WAN-интерфейса. Из локальной сети доступ открыт без аутентификации.

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

Полное описание каждой команды, инварианты iptables и форматы конфигов: [`BEHAVIOR_SPEC.md`](docs/BEHAVIOR_SPEC.md).

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
gh release download v1.6.3 -p 'sign-craze-mipsle' -p 'sign-craze-mipsle.sig' \
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

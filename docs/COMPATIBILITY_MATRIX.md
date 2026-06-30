# Матрица совместимости sign-craze

Этот документ является единым справочником по поддерживаемым архитектурам, прошивкам, устройствам, kernel-модулям и внешним зависимостям. Он заменяет разрозненные упоминания в README. Актуален для sign-craze v0.3.x+ (TUN-режим, без TPROXY как dependency на ядро).

Последнее обновление: v1.6.1 (2026-06-19).

---

## 1. Архитектуры CPU

| Суффикс релиза      | GOARCH   | GOARM | GOMIPS      | Порядок байт  | UPX (`--lzma`) | Примеры устройств                         |
|---------------------|----------|-------|-------------|---------------|----------------|------------------------------------------ |
| `sign-craze-mipsle` | `mipsle` | —     | `softfloat` | Little-endian | Да             | Keenetic KN-1810, KN-1012, KN-2510        |
| `sign-craze-mips`   | `mips`   | —     | `softfloat` | Big-endian    | Нет¹           | Некоторые старые Keenetic/TP-Link MIPS BE |
| `sign-craze-arm7`   | `arm`    | `7`   | —           | Little-endian | Да             | Keenetic Giga / Speed X (ARMv7)           |
| `sign-craze-arm64`  | `arm64`  | —     | —           | Little-endian | Да             | Keenetic Ultra KN-1811, Raspberry Pi 4    |

> ¹ UPX 4.x не поддерживает MIPS big-endian. В release.yml шаг compress выполняется с `|| true` — артефакт загружается без сжатия. Источник: `.github/workflows/release.yml:63-64`.

Все цели собираются с `CGO_ENABLED=0`, `-trimpath`, `-s -w`. Версия Go ≥ 1.25 (`go.mod`). Бинарь single-static — без внешних `.so` зависимостей.

**Требование softfloat**: KN-1810 (и большинство Keenetic на MIPS) не имеет FPU. `GOMIPS=hardfloat` → `SIGILL` при первом запуске.

---

## 2. Firmware Keenetic

| KeeneticOS     | Режим `policy` (RCI 5.x)   | Режим `full` (legacy) | Известные ограничения                                    |
|----------------|--------------------------  |---------------------- |----------------------------------------------------------|
| **5.0.x**      | Поддерживается ✓           | Поддерживается ✓      | Проверено на KN-1810 (validated reference)               |
| **5.1–5.x**    | Ожидается ✓ (RCI стабилен) | Ожидается ✓           | Не тестировалось; сообщите об отклонениях                |
| **4.x**        | Не поддерживается ✗        | Не проверялось        | RCI `/rci/show/ip/policy` отсутствует в 4.x              |
| **3.x и ниже** | Не поддерживается ✗        | Не проверялось        | Нет IP Policy API; iptables-модули могут отличаться      |

**Режим `policy`** требует Keenetic NDM RCI версии 5.x (`/rci/show/ip/policy`, `/rci/show/rc/ip/policy`, POST `/rci/`). Если RCI на `127.0.0.1:79` недоступен — sign-craze завершится с ошибкой и предложит переключиться в `--mode full`. Источник: `docs/BEHAVIOR_SPEC.md` §4.

**NDM rebuild iptables**: Keenetic пересобирает iptables при изменении конфигурации (привязка устройства к политике, save startup-config, WAN reconnect). Цепочки sign-craze (`signcraze_policy`, `signcraze`) при этом теряются и восстанавливаются через hook `/opt/etc/ndm/netfilter.d/50-sign-craze`. Источник: `docs/BEHAVIOR_SPEC.md` §3c.

---

## 3. Протестированные устройства

| Модель                       | Arch     | KeeneticOS | RAM    | Flash  | Статус                               |
|------------------------------|----------|------------|--------|--------|---------------------------------     |
| **Keenetic KN-1810** (Ultra) | `mipsle` | 5.0.4      | 256 MB | 128 MB | **validated** — reference device     |
| Keenetic KN-1012             | `mipsle` | —          | 128 MB | 16 MB  | community                            |
| Keenetic KN-2510             | `mipsle` | —          | 128 MB | 32 MB  | community                            |
| Keenetic Giga (ARMv7)        | `arm7`   | —          | 256 MB | —      | untested                             |
| Keenetic Ultra KN-1811       | `arm64`  | —          | 512 MB | —      | untested                             |
| Raspberry Pi 4               | `arm64`  | —          | ≥1 GB  | —      | untested (Entware + iptables-legacy) |

**Статусы:**

- `validated` — проверено на физическом железе разработчиком (полный тест-план `tasks/test-roadmap.md`).
- `community` — сообщалось пользователями; официально не тестировалось.
- `untested` — бинарь собирается, совместимость не проверялась.

> RAM < 128 MB: sing-box обычно потребляет < 30 MB RSS; `--update-geo` использует стриминговую запись.

---

## 4. Kernel-модули и системные компоненты

| Компонент / модуль          | Требуется в режиме    | Как проверить                                      | Установка (Entware)                           |
|-----------------------------|-----------------------|----------------------------------------------------|---------------------------------------------- |
| `/dev/net/tun` (CONFIG_TUN) | `policy` и `full`     | `ls -l /dev/net/tun`                               | Встроен в стоковое ядро Keenetic (OpenVPN/WG) |
| `iptables-legacy`           | `policy` и `full`     | `iptables --version` (ожидается `legacy`)          | Стандартный на Keenetic                       |
| `xt_MARK`                   | `policy` и `full`     | `iptables -t mangle -A TEST -j MARK --set-mark 1`  | Встроен                                       |
| `xt_set` / `libxt_set.so`   | только `full`         | `opkg list-installed \| grep iptables-mod-ipset`   | `opkg install iptables-mod-ipset`             |
| `ipset` (userspace)         | только `full`         | `ipset --version`                                  | `opkg install ipset`                          |
| `kmod-nfnetlink-queue`      | только `--dpi on`     | `lsmod \| grep nfnetlink_queue`                    | `opkg install kmod-nfnetlink-queue`           |
| `iptables-mod-nfqueue`      | только `--dpi on`     | `iptables -j NFQUEUE --help`                       | `opkg install iptables-mod-nfqueue`           |
| `ip rule` / `ip route`      | `policy` и `full`     | `ip rule show \| grep 0x53`                        | Встроен (iproute2 в Entware/busybox)          |

**Важно**: `xt_TPROXY` больше не требуется как kernel-зависимость начиная с v0.3.0 — sing-craze использует TUN-mode для inbound (sing-box).

**xt_comment** не требуется — sign-craze v0.3+ не использует `-m comment`.

**Preflight** xt_set выполняется автоматически перед применением правил в `full` mode через dry-run (`signcraze_probe` chain). Источник: `internal/firewall/preflight.go`.

---

## 5. Внешние бинари

| Бинарь      | Мин. версия | GitHub-репозиторий                          | Путь установки          | Загружается при              |
|-------------|-------------|---------------------------------------------|-------------------------|------------------------------|
| `sing-box`     | **1.13.0**  | `github.com/SagerNet/sing-box`              | `/opt/sbin/sing-box`    | `--install`, `--update-core`              |
| `nfqws2`       | latest      | `github.com/nfqws/nfqws2-keenetic`          | `/opt/sbin/nfqws2`      | `--dpi on`                                |
| `naiveproxy`   | latest      | `github.com/klzgrad/naiveproxy`             | `/opt/sbin/naive`       | `--with-naive`, `--install --with-naive`  |
| `mieru`        | latest      | `github.com/enfein/mieru`                   | `/opt/sbin/mieru`       | supervised peer при подключении mieru     |

**sing-box 1.13** — минимальная версия: начиная с 1.13 DNS-сервер использует тип `local` (системный resolver); тип `udp` с `detour: direct` запрещён. Генератор конфига в `internal/singbox/config.go` опирается на это поведение.

> Все canonical-комбинации конфигов валидируются в CI через golden-tests:
>
> - sing-box: `internal/singbox/testdata/canonical/*.json` → `sing-box check` (CI job `singbox-check`)
> - xray: `internal/core/xray/testdata/canonical/*.json` → `xray test` (CI job `xray-check`)
> - mihomo: `internal/core/mihomo/testdata/canonical/*.yaml` → `mihomo -t` (CI job `mihomo-check`)

**Паттерны asset-файлов** при автозагрузке:

| Arch     | sing-box asset                         | nfqws2 asset              |
|----------|----------------------------------------|---------------------------|
| `mipsle` | `linux-mipsle-softfloat.tar.gz`        | `mipsel-3.4.ipk`          |
| `mips`   | `linux-mips-softfloat.tar.gz`          | `mips-3.4.ipk`            |
| `arm7`   | `linux-armv7.tar.gz`                   | `armv7-3.2.ipk`           |
| `arm64`  | `linux-arm64.tar.gz`                   | `aarch64-3.10.ipk`        |

Источник: `internal/singbox/download.go`, `internal/dpi/download.go`. Загрузка использует ETag-кэширование. SHA256 верификация sing-box отключена (safety-fixes Issue #4).

---

## 5.1. naiveproxy (klzgrad/naiveproxy)

| Arch     | Поддержка | Asset                          | Причина / Replacement                              |
|----------|-----------|--------------------------------|----------------------------------------------------|
| arm64    | Да        | linux-arm64.tar.xz             |                                                    |
| armv7    | Да        | linux-arm.tar.xz               |                                                    |
| mipsle   | Да        | linux-mipsel.tar.xz            |                                                    |
| mips BE  | **Нет**   | (отсутствует)                  | klzgrad публикует только linux-mipsel (little-endian) |
| amd64    | Да        | linux-x64.tar.xz               | self-test в Docker                                 |

Версия pin: latest GitHub release (v149.0.7827.114-1 на 19.06.2026).
Формат tarball: tar.xz (требует `github.com/ulikunitz/xz` decoder).

Путь установки: `/opt/sbin/naive`. Загружается при `--with-naive` / `--install --with-naive`. Источник: `internal/naiveproxy/`.

---

## 5.2. mieru (enfein/mieru)

| Arch     | Поддержка | Asset                          | Причина / Replacement                              |
|----------|-----------|--------------------------------|----------------------------------------------------|
| arm64    | Да        | tar.gz                         |                                                    |
| armv7    | Да        | tar.gz                         |                                                    |
| mipsle   | Да        | tar.gz                         |                                                    |
| mips BE  | **Нет**   | (отсутствует)                  | enfein публикует только little-endian MIPS         |

Версия pin: latest GitHub release. Формат tarball: tar.gz. Источник кросс-сборки: `Makefile` mieru targets (v1.3.0 changelog).

Путь установки: `/opt/sbin/mieru`. Supervised peer: sign-craze запускает daemon, sing-box подключается через socks5-outbound. Источник: `internal/peer/`.

---

## 6. Несовместимости

- **systemd**: sign-craze не генерирует systemd unit-файлы. Используется только Entware init.d (`/opt/etc/init.d/S99signcraze`). На системах с systemd без Entware установка не поддерживается.
- **nftables**: sign-craze использует `iptables-legacy`. Если активен `iptables-nft` (nf_tables backend), поведение MARK/CONNMARK может отличаться. Проверьте `iptables --version` — ожидается `legacy`.
- **MIPS hardfloat**: бинарь из релиза собран с `GOMIPS=softfloat`. Запуск hardfloat-бинаря на роутере без FPU → `SIGILL`.
- **xray-core на mips/mipsle**: из ветки 26.* при старте не запускается ТОЛЬКО сборка **26.3.27** (текущий latest) — падает с `runtime.futexwakeup ... returned -89` (ENOSYS) на старых ядрах Keenetic (3.4–4.x), регрессия конкретного релиза. Остальные 26.* (более ранние) работают. На mips/mipsle sign-craze ставит pinned custom-build xray v25.12.8 (репо `kittylabassistant/sign-craze-xray`, источник `internal/core/xray/download.go`).
- **OpenWrt без Entware**: sign-craze предполагает файловую структуру Entware (`/opt/sbin`, `/opt/etc/init.d`, `/opt/etc/ndm`). На чистом OpenWrt без Entware пути некорректны, NDM hook отсутствует.
- **KeeneticOS 4.x и ниже**: отсутствует `/rci/show/ip/policy`. Режим `policy` недоступен. Режим `full` теоретически применим, не тестировался.
- **IPv6 без kernel-модулей**: ip6tables-цепочки создаются по аналогии. Если провайдер не предоставляет IPv6 — цепочки пустые (не ломают работу).
- **Проксирование трафика самого роутера в режиме `policy`**: Keenetic ставит mark только на пакеты LAN-устройств, привязанных к политике. Трафик с роутера (SSH, `curl` из shell) mark не получает.
- **Conntrack и уже открытые соединения**: после `--start` соединения, открытые до применения правил, продолжают идти напрямую. Применение ко всем соединениям: `conntrack -F` или реконнект клиента.

---

## Матрица протокол × ядро

Sign-craze поддерживает три взаимозаменяемых прокси-ядра: `sing-box` (по умолчанию), `xray` и `mihomo`. Переключение через `--core <name>`. Протоколы и их параметры покрываются ядрами неравномерно: выбор конкретного ядра зависит от используемых функций — режима XHTTP, наличия Vision flow, PQ-шифрования, формата geo-данных и нативного Clash API.

### Таблица 1: базовые протоколы

| Протокол | sing-box | xray | mihomo | Заметки |
| -------- | -------- | ---- | ------ | ------- |
| VLESS | ✅ | ✅ | ✅ | См. таблицу 2 для частных режимов |
| VMess | ✅ | ✅ | ✅ | AEAD by default |
| Shadowsocks (legacy) | ✅ | ✅ | ✅ | aes-256-gcm, chacha20-poly1305 |
| Shadowsocks 2022 | ✅ | ✅ | ✅ | 2022-blake3-aes-{128,256}-gcm, 2022-blake3-chacha20-poly1305 |
| Trojan | ✅ | ✅ | ✅ | TLS обязателен |
| Hysteria 2 | ✅ | ❌ | ✅ | xray не имеет нативного outbound |
| TUIC v5 | ✅ | ❌ | ✅ | unified-mode mihomo, xray не имеет |
| WireGuard | ✅ | ⚠️ | ✅ | xray частично через outbound, рекомендуется sing-box/mihomo |
| Socks5 | ✅ | ✅ | ✅ | |
| HTTP CONNECT | ✅ | ✅ | ✅ | |

### Таблица 2: VLESS субпротоколы

| Параметр | sing-box | xray | mihomo | Назначение |
| -------- | -------- | ---- | ------ | ------- |
| Reality (PublicKey + ShortID) | ✅ | ✅ | ✅ | TLS-маскировка под чужой домен |
| Vision (`flow=xtls-rprx-vision`) | ⚠️ | ✅ | ✅ | TLS-passthrough, splice |
| Vision UDP443 (`flow=xtls-rprx-vision-udp443`) | ❌ | ✅ | ❌ | Splice over UDP/443 — только xray |
| XHTTP (basic, без mode) | ✅ | ✅ | ✅ | HTTP-маскированный транспорт |
| XHTTP `mode=stream-up` | ❌ | ✅ | ✅ | Upload-only split |
| XHTTP `mode=stream-one` | ❌ | ✅ | ✅ | Single stream |
| XHTTP `mode=packet-up` | ❌ | ✅ | ✅ | Packet-based upload — обход активного DPI |
| Encryption `mlkem768x25519plus` (PQ-VLESS) | ❌ | ✅ | ❌ | sing-box 1.13.x: `Validate()` отвергает (`internal/singbox/validate.go`); раньше ≤ 1.12 не валидировалось. Только xray |
| uTLS Fingerprint | ✅ | ✅ | ✅ | chrome/firefox/safari/ios/random |
| ECH (Encrypted ClientHello) | ⚠️ | ⚠️ | ⚠️ | Все три ядра в процессе внедрения 2026 |
| Transport WebSocket | ✅ | ✅ | ✅ | |
| Transport gRPC | ✅ | ✅ | ✅ | |
| Transport QUIC | ✅ | ✅ | ✅ | header_type для маскировки |
| Transport HTTP/2 (h2) | ✅ | ✅ | ✅ | |

### Таблица 3: ядро-специфичные функции

| Функция | sing-box | xray | mihomo | Заметки |
| ------- | -------- | ---- | ------ | ------- |
| TUN inbound | ✅ | ❌ | ✅ | xray использует tproxy через dokodemo-door |
| TPROXY inbound | ✅ | ✅ | ✅ | sign-craze стандарт через fwmark 0x53 |
| Native Clash API | ⚠️ | ❌ | ✅ | sing-box даёт compat-stub; xray не имеет; mihomo native |
| Proxy-providers (URL-импорт списка) | ❌ | ❌ | ✅ | Clash native |
| Rule-providers | ❌ | ❌ | ✅ | Clash native |
| SRS rule-set (geo бинарный формат) | ✅ | ❌ | ❌ | sing-box native |
| `geoip.dat`/`geosite.dat` | ❌ | ✅ | ⚠️ | xray native; mihomo поддерживает legacy v2ray dat |
| `.mrs` rule-set | ❌ | ❌ | ✅ | mihomo native (с 1.18) |
| Hysteria 2 obfs-salamander tuning | ✅ | ❌ | ✅ | mihomo шире; sing-box базовый |
| Sniffing (TLS SNI / HTTP host / QUIC) | ✅ | ✅ | ✅ | |

### Условные обозначения

```plain
✅ — полная поддержка
⚠️ — частичная или экспериментальная (см. заметки)
❌ — не поддерживается, sign-craze.Validate отвергнет конфиг
```

### Выбор ядра под задачу

- Хочу VLESS Vision UDP443 → `--core xray`
- Хочу PQ-VLESS (mlkem768x25519plus) → `--core xray` (стабильно)
- Хочу XHTTP packet-up для обхода активного DPI → `--core xray` или `--core mihomo`
- Хочу native Clash API → `--core mihomo`
- Хочу Hysteria 2 с тонкими obfs/congestion-control → `--core mihomo` или `--core sing-box`
- Хочу TUIC v5 + WireGuard вместе → `--core sing-box` или `--core mihomo`
- Хочу всё работало "из коробки" со стандартным VLESS+Reality → любой, default `sing-box`

### Источники

- Официальные репозитории: `XTLS/Xray-core`, `MetaCubeX/mihomo`, `SagerNet/sing-box`

---

## 7. CLI-команды (по версиям)

Таблица отражает наличие флага в конкретной версии. `+` — доступно, `—` — недоступно.

### 7.1 DPI-исключения и auto-update (добавлены в v0.8.0)

| Команда                         | v0.7.x | v0.8.0+ | Описание                                                                 |
|---------------------------------|--------|---------|--------------------------------------------------------------------------|
| `--dpi-exclude-ips <IPs>`       | —      | +       | Список IP-адресов, исключённых из DPI-десинхронизации (например VPN-endpoints). Принимает запятую-разделённый список или CIDR. |
| `--dpi-exclude-ips-list`        | —      | +       | Печать текущего списка IP-исключений из DPI.                             |
| `--dpi-update-urls <URLs>`      | —      | +       | Задать URL-источники для автоматического обновления hostlist (через запятую). |
| `--dpi-update-interval <часы>`  | —      | +       | Период авто-обновления hostlist в часах. `0` — авто-обновление отключено. |
| `--dpi-update-now`              | —      | +       | Принудительный немедленный update hostlist из заданных URL-источников.   |

**Обратная совместимость**: `state.json` без новых полей (`dpi_exclude_ips`, `dpi_update_urls`, `dpi_update_interval`) мигрирует автоматически при первом старте v0.8.0 — отсутствующие поля инициализируются нулевыми значениями, данные существующей конфигурации не теряются.

### 7.3 Naive/Mieru (v1.3.0+)

| Команда            | Где применяется             | Описание                                                                                   |
|--------------------|-----------------------------|--------------------------------------------------------------------------------------------|
| `--with-naive`     | флаг для `--install*`       | Установить naiveproxy daemon и sing-box socks5-outbound на него.                           |
| `--update-naive`   | самостоятельная команда     | Обновить бинарь naiveproxy (`/opt/sbin/naive`) до latest GitHub release.                  |

**Совместимость ядер:** `xray` и `mihomo` отклоняют naive/mieru с понятной ошибкой. Supervised peers поддерживаются только при активном ядре `sing-box`.

### 7.4 Hardening flags (v1.4.0+)

| Команда / флаг     | Описание                                                                                   |
|--------------------|--------------------------------------------------------------------------------------------|
| `--diag --json`    | Машинно-читаемый JSON-вывод диагностики (sign-craze doctor).                              |
| `--no-color`       | Отключить ANSI-цвет в выводе. Альтернативы: переменная `NO_COLOR`, `TERM=dumb`.           |

### 7.2 Артефакты релиза v0.8.0

| Архитектура | Суффикс артефакта       | Сжатие (UPX)  |
|-------------|-------------------------|---------------|
| `arm64`     | `sign-craze-arm64`      | `--lzma`      |
| `arm7`      | `sign-craze-arm7`       | `--lzma`      |
| `mipsle`    | `sign-craze-mipsle`     | `--lzma`      |
| `mips`      | `sign-craze-mips`       | Нет¹          |

> ¹ UPX 4.x не поддерживает MIPS big-endian — см. §1.

---

## 8. Поведение по версиям

| Версия    | Изменение                                                                                           |
|-----------|-----------------------------------------------------------------------------------------------------|
| **v0.8.0** | WAN-фильтр DPI (`--dpi-exclude-ips`), VPN-exclude, авто-обновление hostlist (`--dpi-update-urls`, `--dpi-update-interval`). |
| **v0.8.1** | Fallback DNS для `UpdateHostlist` — при недоступности системного resolver используется запасной DNS. |
| **v0.8.2** | Применение hostlist без `DPITargets` — `--dpi on` не требует заранее заданных хостов.               |
| **v0.8.3** | Миграция `routing.json`: TUN → TPROXY. При старте на конфиге с TUN-inbound автоматически переводит routing.json в TPROXY-схему. |
| **v1.0.0** | Unified multi-core routing: Web UI `:9092` и Apply работают с любым активным ядром. routing.json core-agnostic. Presets per-core с автотрансляцией URL. Несовместимости (TUN/xray, .srs/xray, .dat/sing-box) surfaced как warnings в `/api/validate`. `state.ValidCores` — канонический список ядер. |
| **v1.1.0** | Routing UI: пресеты AS-IS («+Добавить») и TO-BE («Заменить») + клиентский буфер изменений (Save/Cancel + dirty-confirm). DPI NFQUEUE-цепочка переехала из mangle POSTROUTING в mangle FORWARD (`signcraze_dpi_fwd`) — desync покрывает все LAN-устройства. |
| **v1.1.1** | Защита SSH/admin при режиме policy: LAN-bypass `-d <LAN_IP> -j RETURN` на pos=1 в `signcraze_policy`; admin-ports 22/222 bypass; `--uninstall` отвязывает host-policy через RCI `UnsetHostPolicy`. netfilter.d hook с flock+debounce: пачка 10 NDM-событий генерирует 2 reapply вместо 10. |
| **v1.1.2** | Routing UI: светлая тема + переключатель (persist в localStorage, дефолт по `prefers-color-scheme`). |
| **v1.2.0** | ANSI-цвет CLI и логов (`internal/cli/color.go` без внешних зависимостей; `colorTextHandler` для slog stderr). Opt-out: `--no-color` > `NO_COLOR` > `FORCE_COLOR` > `TERM=dumb`. |
| **v1.3.0** | naiveproxy (klzgrad) + mieru (enfein) supervised peers через process chain: sign-craze запускает daemon, sing-box подключается через socks5. `--with-naive`, `--update-naive`. arm64/arm7/mipsle поддерживаются; mips BE отклоняется. xray/mihomo отклоняют naive/mieru с понятной ошибкой. |
| **v1.4.0** | Hardening: `iptables-restore --noflush` batch (24 fork → 3), WAN cache, watchdog покрывает REDIRECT/DPI-FORWARD, reproducible builds, cosign keyless OIDC + SLSA, `--diag --json`, WebSocket keepalive 30s, bcrypt cost=10 для MIPS, SHA256 streaming geo, IPK streaming. Регрессионные тесты: SSH-bypass, S99-shim, geosite no-dat, FuzzMieruWireProto. |
| **v1.4.1** | CI fix: `actions/attest-build-provenance` v2 → v4, `golangci-lint-action v9` + golangci-lint v2.12.2. |
| **v1.4.2** | CI: `NODE_OPTIONS=--no-deprecation` (подавление предупреждений Node 24 в lint+publish jobs). |

---

## 9. RuleSet формат × ядро (v1.0.0+)

### Таблица: поддержка форматов rule-set

| Формат | sing-box | xray | mihomo | Путь трансляции в routing.json → ядро |
|--------|----------|------|--------|---------------------------------------|
| `.srs` (SRS бинарный) | ✅ native | ❌ warning | ❌ warning | sing-box: прямая передача в `rule_set`; xray/mihomo: warning при validate |
| `.mrs` (mihomo rule-set) | ❌ warning | ❌ warning | ✅ native | mihomo: `.srs` URL из routing.json конвертируется в `.mrs` через `ruleSetSources`-таблицу |
| `geoip.dat` / `geosite.dat` | ❌ | ✅ native | ⚠️ legacy v2ray | xray: ищет dat-файлы в рабочей директории или `XRAY_ASSET_LOCATION` |
| `geoip:<tag>` matcher (inline) | ✅ | ✅ | ✅ | все ядра: inline матчер в правиле без внешнего файла |
| `geosite:<tag>` matcher (inline) | ✅ | ✅ | ✅ | все ядра: inline матчер в правиле без внешнего файла |

### Трансляция xray (routing.json → xray config.json)

При генерации конфига для xray поле `rule_set` **не записывается** — xray не поддерживает SRS rule-set. Вместо этого:

1. URL из `rule_set_sources` → маппинг через `ruleSetSources` translation table.
2. `geoip:` и `geosite:` prefix-матчеры записываются в правило напрямую (`ip` / `domain` поля routing rule).
3. `.dat`-файлы xray ищет сам; sign-craze не управляет их загрузкой — только geo-файлы для sing-box/mihomo обновляются через `--update-geo`.

**Пример трансляции:**

| routing.json rule_set URL | Результат в xray config.json |
|--------------------------|------------------------------|
| `https://…/geoip-ru.srs` | `"ip": ["geoip:ru"]` |
| `https://…/geosite-category-ads.srs` | `"domain": ["geosite:category-ads"]` |
| `https://…/custom.srs` | warning (нет dat-эквивалента) + правило пропускается |

### TUN / TProxy inbound × ядро

| Inbound в routing.json | sing-box | xray | mihomo |
|------------------------|----------|------|--------|
| `type: tproxy` (порт 7895) | ✅ — стандарт | ✅ — dokodemo-door tproxy | ✅ — tproxy listener |
| `type: tun` | ✅ — `needsTUN()=true` → создаётся TUN-интерфейс | ⚠️ warning при validate; Apply продолжается с tproxy | ✅ — TUN поддерживается нативно |

**xray + TUN**: если `routing.json` содержит TUN inbound, при генерации конфига для xray sign-craze логирует warning и использует `dokodemo-door` (tproxy) вместо TUN. Firewall-правила не меняются — fwmark `0x53` направляет трафик на `127.0.0.1:7895` в любом случае.

### Protocol matcher × ядро

| Matcher-тип в routing.json | sing-box | xray | mihomo |
|---------------------------|----------|------|--------|
| `domain` / `domain_suffix` | ✅ | ✅ | ✅ |
| `domain_regex` | ✅ | ✅ | ⚠️ |
| `ip_cidr` | ✅ | ✅ | ✅ |
| `geoip:<tag>` | ✅ | ✅ | ✅ |
| `geosite:<tag>` | ✅ | ✅ | ✅ |
| `rule_set` (SRS) | ✅ | ❌ → warning, трансляция через geosite/geoip | ❌ → warning, конвертируется в `.mrs` |
| `protocol` (http/tls/bittorrent) | ✅ | ✅ | ⚠️ частично |
| `port` / `port_range` | ✅ | ✅ | ✅ |
| `network` (tcp/udp) | ✅ | ✅ | ✅ |

---

## 10. Entware IPK arch suffix mapping

При публикации opkg-пакета архитектура указывается в имени файла
по соглашению Entware (отличается от sign-craze internal suffix).

| sign-craze suffix | Entware Architecture | IPK filename example | Типичные роутеры |
|---|---|---|---|
| `arm64` | `aarch64-3.10` | `sign-craze_1.6.0_aarch64-3.10.ipk` | Keenetic Hero 4G+, новые ARM64 |
| `arm7` | `armv7-3.2` | `sign-craze_1.6.0_armv7-3.2.ipk` | Keenetic KN-1011, ARM Cortex-A7 |
| `mipsle` | `mipsel-3.4` | `sign-craze_1.6.0_mipsel-3.4.ipk` | Keenetic Giga/Ultra/Hopper (MT7621) |
| `mips` | `mips-3.4` | `sign-craze_1.6.0_mips-3.4.ipk` | Старые big-endian MIPS-роутеры |

Определить архитектуру роутера: `opkg print-architecture | grep -v all`.

## 11. Автоматические зависимости через opkg Depends

Если sign-craze установлен через opkg (feed или offline .ipk),
opkg автоматически разрешит и установит следующие пакеты:

| Пакет | Назначение |
|---|---|
| `ipset` | ipset signcraze_ipv4/ipv6 в режиме full |
| `iptables` | основа firewall |
| `iptables-mod-conntrack-extra` | CONNMARK match в mangle chains |
| `iptables-mod-tproxy` | xt_TPROXY kernel module для -j TPROXY |
| `kmod-nfnetlink-queue` | -j NFQUEUE для DPI через nfqws2 |
| `curl` | скачивание sing-box и других ядер при --install |

Не входят в Depends (sign-craze управляет сам): sing-box, xray, mihomo,
nfqws2, naiveproxy, mieru. На Keenetic `xt_set` встроен в стоковое
ядро, поэтому `iptables-mod-ipset` не указан в Depends.

# Матрица совместимости sign-craze

Этот документ является единым справочником по поддерживаемым архитектурам, прошивкам, устройствам, kernel-модулям и внешним зависимостям. Он заменяет разрозненные упоминания в README. Актуален для sign-craze v0.3.x+ (TUN-режим, без TPROXY как dependency на ядро).

Последнее обновление: v0.8.3.

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
| `sing-box`  | **1.13.0**  | `github.com/SagerNet/sing-box`              | `/opt/sbin/sing-box`    | `--install`, `--update-core` |
| `nfqws2`    | latest      | `github.com/nfqws/nfqws2-keenetic`          | `/opt/sbin/nfqws2`      | `--dpi on`                   |

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

## 6. Несовместимости

- **systemd**: sign-craze не генерирует systemd unit-файлы. Используется только Entware init.d (`/opt/etc/init.d/S99signcraze`). На системах с systemd без Entware установка не поддерживается.
- **nftables**: sign-craze использует `iptables-legacy`. Если активен `iptables-nft` (nf_tables backend), поведение MARK/CONNMARK может отличаться. Проверьте `iptables --version` — ожидается `legacy`.
- **MIPS hardfloat**: бинарь из релиза собран с `GOMIPS=softfloat`. Запуск hardfloat-бинаря на роутере без FPU → `SIGILL`.
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
| Encryption `mlkem768x25519plus` (PQ-VLESS) | ⚠️ | ✅ | ❌ | sing-box экспериментально; в sing-box ≤ 1.12 не валидируется, в 1.13+ недоступно через Validate() |
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

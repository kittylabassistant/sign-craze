# BEHAVIOR_SPEC.md

> Версия: 2026-05-20. Спецификация v1.4.2.

Функциональная спецификация sign-craze, написанная в режиме clean-room.
Исходники XKeen не читались. Только публичные источники.

> **Текущая архитектура (TUN-mode):** sign-craze использует TPROXY-аналог через
> TUN-интерфейс sing-box. Routing к `signbox-tun` идёт через fwmark `0x53` →
> table `83` → default route. Описание ниже отражает текущую TUN-mode архитектуру.

Использованные источники:

- <https://sing-box.sagernet.org/configuration/>
- `man iptables`, `man iptables-extensions`, `man ipset`, `man iproute2`
- <https://help.keenetic.com> (Entware, OPKG, контракты init.d)
- <https://www.kernel.org/doc/html/latest/networking/netfilter.html>
- README nfqws2-keenetic (MIT, публичный)
- Документация zapret2 (MIT, публичная)

---

## 1. CLI-команды

### `--install` / `-i`

**Входные данные**: нет (интерактивные подсказки для URL прокси / outbound, если не настроены).

**Эффекты в системе**:

- Скачивает бинарь sing-box с `github.com/SagerNet/sing-box/releases` под текущую архитектуру.
- Устанавливает бинарь в `/opt/sbin/sing-box`, права `0755`.
- Создаёт директорию конфигов `/opt/etc/sign-craze/`, права `0755`.
- Генерирует `/opt/etc/sign-craze/config.json` (tproxy inbound на порту `7895`, fwmark `0x53`).
- Создаёт init.d shim `/opt/etc/init.d/S99signcraze`, права `0755`.
- Создаёт NDM netfilter.d hook `/opt/etc/ndm/netfilter.d/50-sign-craze`,
  права `0755` (см. §3c — persistence через NDM rebuild).
- Создаёт директорию состояния `/opt/var/lib/sign-craze/`, права `0755`.
- Создаёт директорию логов `/opt/var/log/sign-craze/`, права `0755`.
- Сервис **не запускает** (пользователь вызывает `--start` отдельно).

**Идемпотентность**: безопасно повторять; текущий бинарь резервируется перед заменой.

**Ошибки**:

- Нет сети → загрузка завершается с понятной ошибкой.
- `/opt` не смонтирован / недостаточно места (<30 МБ) → явная ошибка до начала загрузки.
- `sing-box check -c <config>` падает → откат бинаря из резервной копии, возврат ошибки.

---

### `--install-auto`

Аналог `--install`, но читает конфигурацию роутера для автоопределения outbound / ISP-интерфейса. Без интерактивных подсказок.

---

### `--install-offline`

Аналог `--install`, но читает бинарь из локального пути, переданного аргументом. Сетевая загрузка пропускается.

- `[--with-naive]` — опт-ин на скачивание и установку бинаря klzgrad/naiveproxy при `--install` без явного `--proxy naive+...`. См. §8 "Supervised peers — naiveproxy" и ADR-0021.

---

### `--start`

**Входные данные**: нет.

**Эффекты в системе**:

- Захватывает `/opt/var/lock/sign-craze.lock` (flock LOCK_EX|LOCK_NB).
- Проверяет установку (`/opt/sbin/sing-box` существует, конфиг существует).
- Применяет правила iptables (см. §3).
- Запускает `/opt/sbin/sing-box run -c /opt/etc/sign-craze/config.json` в фоне (detached, Setpgid).
- Записывает PID в `/opt/var/run/sign-craze-singbox.pid`.
- Опрашивает `/proc/<pid>/status` до 5 с для подтверждения, что процесс жив.
- Если режим DPI включён: запускает nfqws2 в фоне, записывает `/opt/var/run/sign-craze-nfqws2.pid`.
- Освобождает блокировку.

**Наблюдаемо**: `ps` показывает процесс `sing-box run -c ...`. Правила iptables видны в `iptables -t mangle -L -n`.

---

### `--stop`

**Входные данные**: нет.

**Эффекты в системе**:

- Захватывает блокировку.
- Читает PID-файлы; отправляет SIGTERM процессу sing-box (и nfqws2, если запущен).
- Ждёт до 10 с; отправляет SIGKILL, если процесс ещё жив.
- Удаляет правила iptables (все правила с меткой `signcraze:*`).
- Удаляет PID-файлы.
- Освобождает блокировку.

**Наблюдаемо**: процессы исчезли из `ps`. Цепочки iptables очищены / удалены.

---

### `--restart` / `-r`

Эквивалент `--stop` + `--start`. Блокировка удерживается на всё время операции.

---

### `--status` / `-s`

**Вывод** (stdout):

```plain
<active_core>:  запущен  (pid 1234)
nfqws2:         остановлен
режим:          policy
версия:         sign-craze v1.4.2 / <core> v<core-version>
```

Метка ядра соответствует активному core (`sing-box`, `xray`, `mihomo`) из `--core <name>`.

Состояние системы не изменяется.

---

### `--update` / `-u`

Обновляет бинарь sign-craze. Скачивает последний релиз с GitHub, проверяет SHA256, атомарно заменяет `/opt/sbin/sign-craze`. Запущенный сервис **не перезапускает**.

---

### `--update-geo` / `-g`

Скачивает обновлённые SRS rule-set файлы из манифеста релиза `sign-craze-dat`. Загружает только файлы, чей SHA256 отличается от локального. Сохраняет в `/opt/var/lib/sign-craze/geo/`. Атомарная замена каждого файла.

---

### `--update-core`

Обновляет только бинарь sing-box (не сам sign-craze). Аналог шагов 1–5 из `--install`.

---

### `--uninstall`

Останавливает сервис (если запущен), удаляет правила iptables, удаляет init.d shim, удаляет `/opt/sbin/sing-box`, директории конфигов и состояния, бинарь sign-craze и логи (`/opt/var/log/sign-craze/`).

---

### `--version` / `-v`

Выводит:

```plain
sign-craze v<VERSION> (собран <дата>, <коммит>)
sing-box   v<VERSION>  (установлен в /opt/sbin/sing-box)
```

---

### `--diag` / `-D`

Запускает диагностику. Для каждого пункта выводит PASS/WARN/FAIL:

- Бинарь sign-craze существует и исполняемый
- Бинарь sing-box существует, версия читается
- Конфиг валиден (`sing-box check -c`)
- iptables/ip6tables доступны
- ipset доступен
- `ip route` — маршрут по умолчанию существует
- Сервис запущен (PID-файлы + проверка /proc)
- Geo-файлы присутствуют и не устарели (>7 дней → WARN)
- Блокировка не удерживается другим процессом

---

### `--port-add`

Добавляет порт (или диапазон) в проксируемый набор. Сохраняет в конфиг. Сервис **не перезагружает**.

### `--port-del`

Удаляет порт из проксируемого набора.

### `--port-list`

Выводит список настроенных портов.

### `--exclude-add` / `--exclude-del` / `--exclude-list`

Аналог port-команд для списка исключений (трафик в обход sing-box).

---

### `--backup` / `-b`

Создаёт tarball директории `/opt/etc/sign-craze/` в `/opt/var/lib/sign-craze/backups/` с именем `backup-<timestamp>.tar.gz`.

### `--restore`

Восстанавливает конфиг из tarball (путь — аргумент). Сначала останавливает сервис, восстанавливает, **не перезапускает**.

---

### `--ui on|off`

`on`: запускает встроенные HTTP-серверы:

- порт `9090` — Metacube(XD) (Clash-совместимый dashboard, управление прокси и мониторинг трафика)
- порт `9091` — admin REST API sign-craze
- порт `9092` — Routing Editor SPA

Серверы слушают на `0.0.0.0`. Правила в `filter/INPUT` (с owner-комментарием `signcraze:wan-block`, идемпотентно добавляются через `EnsureRule`) дропают входящий трафик на порт 9090 с WAN-интерфейса (определяется через `DetectISPInterface`). Доступ из LAN открыт без аутентификации.

`off`: останавливает все HTTP-серверы.

---

### `--dpi on|off`

Включает или отключает DPI-обход через nfqws2. При включении: скачивает бинарь nfqws2 если отсутствует, генерирует nfqws2.conf, добавляет цепочки NFQUEUE. При отключении: удаляет цепочки NFQUEUE, останавливает nfqws2.

### `--dpi-strategy <preset|file://путь>`

Устанавливает стратегию DPI. Пресеты берутся из релиза nfqws2-keenetic. Сохраняет в конфиг.

### `--dpi-update`

Скачивает последний tarball nfqws2-keenetic для текущей архитектуры. Сервис **не перезапускает**.

### `--dpi-targets <домены через запятую | clear>`

Selective DPI: задаёт список доменов/SNI-паттернов для applying desync. Пишет
`/opt/etc/sign-craze/dpi-hostlist.txt` и добавляет `--hostlist=<path>` в
`NFQWS_ARGS`. Если targets пуст или передано `clear` — флаг убирается, nfqws2
пытается desync для всего трафика (поведение по умолчанию).

При активном `DPIEnabled` команда сразу регенерирует `nfqws2.conf` и hostlist.
nfqws2 подхватит изменения только после `--restart`.

```sh
sign-craze --dpi-targets discord.com,youtube.com,googlevideo.com
sign-craze --dpi-targets clear
```

#### Инвариант: hostlist apply без явных DPITargets (v0.8.2)

`nfqws2` запускается с флагом `--hostlist=<path>` если выполнено хотя бы одно
условие:
- `state.dpi_targets` непуст, **или**
- файл `/opt/etc/sign-craze/dpi-hostlist.txt` существует на диске.

Без второго условия auto-update создавал бы файл hostlist, но nfqws2 его не
читал (флаг не передавался), пока в `dpi_targets` не добавить хотя бы одну
запись вручную.

### `--dpi-targets-list`

Печатает текущий список DPI targets (один на строку) либо сообщение, что desync
применяется ко всему трафику.

### `--dpi-exclude-ips <IP-список через запятую | clear>`

Список IP-адресов, для которых nfqws2-десинхронизация **не применяется**:
RETURN-правила вставляются в начало `signcraze_dpi_fwd` перед NFQUEUE-портами.
Назначение — сохранение TLS-маскировки VPN-handshakes к Reality-серверу
(downstream-VPN-клиенты роутера, открывающие соединение к нашему VPS).

Каждый IP валидируется через `netip.ParseAddr` (IPv4/IPv6). `clear` очищает
список.

```bash
sign-craze --dpi-exclude-ips 167.17.177.54,2606:4700::1
sign-craze --dpi-exclude-ips clear
```

При активном `DPIEnabled` изменения применяются после `--restart`.

### `--dpi-exclude-ips-list`

Печатает текущий список IP-исключений DPI (один на строку).

### `--dpi-update-urls <URL-список через запятую | clear>`

URL-источники для auto-update hostlist (B.3). Каждый URL должен начинаться с
`http://` или `https://`. Скачанные строки парсятся как hostlist (одна запись
на строку, поддерживается формат hosts/Adblock), объединяются с
`state.dpi_targets`, дедуплицируются и атомарно пишутся в
`/opt/etc/sign-craze/dpi-hostlist.txt`.

Рекомендованные источники (в `dpi.DefaultUpdateURLs`):

```
https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-general.txt
https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-google.txt
```

`clear` отключает auto-update.

### `--dpi-update-interval <часы>`

Период auto-update hostlist в часах. `0` — выключено. Умолчание: `24`.
Watchdog-демон (`--service-watchdog`) проверяет `state.dpi_last_update`
раз в час и запускает обновление, если прошло больше `interval`.

### `--dpi-update-now`

Принудительный одноразовый запуск UpdateHostlist. Использует список из
`state.dpi_update_urls`. После успеха обновляет `state.dpi_last_update`.

#### Resilient DNS для UpdateHostlist (v0.8.1)

Скачивание hostlist использует `net.Resolver{PreferGo: true}` с кастомным
`Dial`-цепочкой: системный resolver → fallback `1.1.1.1:53` → `9.9.9.9:53` →
`8.8.8.8:53`. Применяется **только** к HTTP-запросам auto-update URLs.
Обычные DNS-резолвы sign-craze (sing-box, outbound.server) идут через системный
resolver без override — пользовательский DNSCrypt/DoH сохраняется.

### `--core-list`

Выводит список зарегистрированных ядер (sing-box / xray / mihomo) с указанием активного.

---

### `--core <name>`

Устанавливает активное ядро в `state.json`. Для применения требует `--restart`.

---

### `--core-install <name>`

Скачивает и устанавливает указанное ядро в `/opt/sbin/`. Сервис **не перезапускает**.

**Примечание для xray**: после `--core-install xray` обязательно выполнить `--update-geo --core xray` для загрузки `geosite.dat`/`geoip.dat` — иначе `--start --core xray` упадёт с явной ошибкой "запустите --update-geo --core xray" (см. `internal/core/xray/render_rules.go`, инцидент 2026-05-12).

---

### Унифицированный routing для всех ядер (v1.0.0+)

`routing.json` (`/opt/etc/sign-craze/routing.json`) — единый core-agnostic
источник правил маршрутизации, inbound, outbound, rule_set и presets для всех
трёх ядер. Web UI на порту 9092 принимает изменения для активного ядра без
ошибок; при `Apply` каждое ядро через свой адаптер транслирует `RoutingConfig`
в native-форму:

| Ядро | Format конфига | Routing native-форма | RuleSet translation |
|---|---|---|---|
| sing-box | JSON | `route.rules[]` + `route.rule_set[]` | прямой `.srs` URL |
| xray | JSON | `routing.rules[]` (field type) | `geosite-X`/`geoip-X` → `domain:["geosite:X"]`/`ip:["geoip:X"]` matcher |
| mihomo | YAML | `rules:` (TYPE,VALUE,ACTION) + `rule-providers:` | `.mrs` URL в rule-providers |

Несовместимые конструкции (TUN inbound на xray/mihomo, `.srs` URL на mihomo,
RuleSet без `geosite-`/`geoip-` префикса на xray) **не блокируют** apply, но
surfaced через `POST /api/validate` как `Warnings[]` для отображения в UI.

Apply (`POST /api/apply`) регенерирует конфиг **активного ядра**, не
sing-box-only: пишет в `c.ConfigPath()` (зависит от `core.Active(state.Core)`).

Built-in presets (`/api/presets/<name>/apply`) используют translation table
`ruleSetSources` для резолва per-core URL — один и тот же preset работает на
всех трёх ядрах.

---

### `--mode policy|full`

Переключает режим маршрутизации. Для применения требует перезапуска (`--restart`).

- `policy` *(default)* — интеграция с Keenetic IP Policy. Sign-craze создаёт
  через RCI policy с `description=sign-craze`, читает присвоенный Keenetic mark
  и ставит TPROXY с фильтром `-m mark --mark <keenetic-mark>`. Выбор устройств
  делается в web-UI Keenetic «Приоритеты подключений».
- `full` — legacy-схема: ipset `signcraze_ipv4/v6` по dst-IP + fwmark `0x53` +
  цепочки `signcraze*`. Эквивалент бывшего `hybrid` (с опциональным DPI через
  `--dpi on`).

Legacy-имена (`proxy`, `dpi`, `hybrid`) принимаются для обратной совместимости
и автоматически конвертируются в `policy` с WARN в логе. Для возврата старого
поведения после миграции: `sign-craze --mode full --restart`.

---

## 2. Объекты конфигурации

### `/opt/etc/sign-craze/config.json` (sing-box)

Источник: <https://sing-box.sagernet.org/configuration/>

Минимальная структура TUN inbound (из `internal/singbox/templates/tun.json.tmpl`):

<!-- MTU=1280 — минимум IPv6, защита от PMTUD black holes на VPN. См. `internal/singbox/config.go::DefaultTUNMTU`. -->

```json
{
  "log": { "level": "info", "output": "/opt/var/log/sign-craze/sing-box.log", "timestamp": true },
  "inbounds": [
    {
      "type": "tun",
      "tag": "tun-in",
      "interface_name": "signbox-tun",
      "address": ["172.19.0.1/30"],
      "mtu": 1280,
      "auto_route": false,
      "stack": "gvisor"
    }
  ],
  "outbounds": [ /* задаётся пользователем */ ],
  "route": { /* правила с SRS rule-set, auto_detect_interface: true */ }
}
```

Ключевые параметры (настраиваются через sign-craze):

- `interface_name`: всегда `"signbox-tun"` — фиксированное имя TUN-устройства
- `auto_route`: `false` — маршрутизация управляется sign-craze через ip rule/ip route
- Уровень логирования: зеркалирует `SIGNCRAZE_LOG_LEVEL`
- `mark` не нужен для TUN inbound (применяется на уровне iptables fwmark)

### `/opt/etc/sign-craze/nfqws2.conf`

Источник стратегий: github.com/nfqws/nfqws2-keenetic v1.1.5+ (MIT).

Файл — **диагностический артефакт** для оператора. Бинарь nfqws2 НЕ читает
.conf-файл сам (флаг `--config` отсутствует у nfqws2). sign-craze формирует
командную строку nfqws2 в Go из тех же ConfigParams, что записываются в
конфиг (`internal/dpi/config.go`).

```sh
ISP_INTERFACE=eth0            # определяется через `ip route show default`
NFQWS_QUEUE_NUM=300           # совпадает с upstream nfqws2-keenetic
POLICY_NAME=signcraze
BLOB_DIR=/opt/etc/sign-craze/blobs
NFQWS_HOSTLIST=""             # /opt/etc/sign-craze/dpi-hostlist.txt при selective DPI

NFQWS_BASE_ARGS="--lua-init --blob-dir=/opt/etc/sign-craze/blobs"
NFQWS_ARGS="--filter-tcp=443,80,1984,5222 --filter-l7=http,tls,mtproto ..."   # TLS/HTTP+QUIC: YouTube
NFQWS_ARGS_QUIC="--filter-udp=443 --filter-l7=quic --payload=quic_initial ..." # QUIC
NFQWS_ARGS_UDP="--filter-udp=590-600,1400,3478-3481,... --filter-l7=stun,discord,..."  # Discord voice/STUN
NFQWS_EXTRA_ARGS=""           # --hostlist=<file> добавляется при непустом NFQWS_HOSTLIST
```

#### Командная строка nfqws2

sign-craze собирает cmdline по схеме upstream init.d (`etc/init.d/common`):

```sh
nfqws2 --user=nobody --qnum=300 <NFQWS_BASE_ARGS>
       <NFQWS_ARGS_UDP>  --new
       <NFQWS_ARGS_QUIC> --new
       <NFQWS_ARGS> <NFQWS_EXTRA_ARGS>
```

Маркер `--new` отделяет независимые стратегии. Без него nfqws2 склеит
TCP/UDP/QUIC флаги в один блок и упадёт в init.

#### Blob-файлы

При `--dpi on` (или `--install --with-dpi`) sign-craze распаковывает из .ipk:

- `/opt/etc/sign-craze/blobs/quic_initial.bin`
- `/opt/etc/sign-craze/blobs/tls_clienthello.bin`

Стратегии `--lua-desync=fake:blob=quic_initial` и `blob=tls_clienthello`
требуют этих файлов. Без них nfqws2 падает в init c "blob not found".

---

## 3. Системные инварианты (после `--install` + `--start`)

### 3a. Режим `policy` (default)

В этом режиме маркировку трафика по src-устройству выполняет Keenetic
самостоятельно (через UI «Приоритеты подключений»). Sign-craze добавляет
только TPROXY-перенаправление помеченных пакетов в sing-box и собственный
ip rule для loop-prevention выходного трафика sing-box.

#### Keenetic-генерируемое (sign-craze не трогает)

```plain
ip rule:    N:   from all fwmark 0xffffaaXX lookup <T>
            N+1: from all fwmark 0xffffaaXX blackhole
ip route:   table <T>: default via <gw> dev <WAN>, ...
```

`<T>` и `XX` присваиваются Keenetic'ом инкрементально. Пример: XKeen получает
`mark=0xffffaaa`/`table=4096`, sign-craze — `mark=0xffffaab`/`table=4098`.

#### iptables (mangle), добавляет sign-craze

```plain
Chain PREROUTING
  -j signcraze              # переход в основную цепочку маркировки

Chain FORWARD
  -o $WAN_IFACE -m mark ! --mark 0x53 -j signcraze_dpi_fwd   # только если DPIEnabled=true

Chain signcraze_policy      # инварианты порядка правил: bypass-правила вставляются
                            # через -I 1 (в начало), до TPROXY/MARK

  # --- LOCAL bypass (pos=1) ---
  # Keenetic метит mark 0xffffaaXX весь трафик от policy-устройств, включая пакеты
  # с dst=LAN_IP роутера. Без этого RETURN SSH/dropbear:222 и веб-интерфейс роутера
  # перехватываются TPROXY и становятся недоступны для policy-устройств.
  # LAN_IP определяется через netif.DetectLANAddr при Apply.
  -d <LAN_IP>    -j RETURN  # трафик к самому роутеру — вернуть без перехвата

  # --- Admin-ports bypass (pos=1, defense-in-depth) ---
  # Страхует доступ к SSH/web-admin даже если DetectLANAddr вернул неверный IP
  # (multi-LAN роутер, нестандартный бридж). Порты: 22 (OpenSSH), 222 (dropbear Entware).
  -p tcp --dport 22  -j RETURN
  -p udp --dport 22  -j RETURN
  -p tcp --dport 222 -j RETURN
  -p udp --dport 222 -j RETURN

  # --- TPROXY/MARK правила (далее) ---

Chain signcraze
  ! -s 127.0.0.0/8 ! -s 169.254.0.0/16 ! -i lo \
    -m mark --mark 0xffffaaXX -p tcp -j MARK --set-mark 0x53
    -m comment --comment "signcraze:mark-policy-tcp"
  ! -s 127.0.0.0/8 ! -s 169.254.0.0/16 ! -i lo \
    -m mark --mark 0xffffaaXX -p udp -j MARK --set-mark 0x53
    -m comment --comment "signcraze:mark-policy-udp"

Chain signcraze_dpi_fwd  # только если DPIEnabled=true; jump из FORWARD
  # RETURN для каждого IP из state.DPIExcludeIPs (Reality-маскировка VPN)
  -d <vpn_endpoint_ip> -j RETURN
  ...
  -p tcp --dport 80   -j NFQUEUE --queue-num 300 --queue-bypass
  -p tcp --dport 443  -j NFQUEUE --queue-num 300 --queue-bypass
  -p tcp --dport 2053:2096 -j NFQUEUE --queue-num 300 --queue-bypass
  -p tcp --dport 8443 -j NFQUEUE --queue-num 300 --queue-bypass
  -p udp --dport 443  -j NFQUEUE --queue-num 300 --queue-bypass
  -p udp --dport 19200:19400 -j NFQUEUE --queue-num 300 --queue-bypass
  -p udp --dport 50000:50100 -j NFQUEUE --queue-num 300 --queue-bypass
```

**`-o $WAN_IFACE`** на jump-правиле: NFQUEUE срабатывает только для трафика,
реально уходящего через WAN-интерфейс ISP. Без фильтра NFQUEUE захватывал бы
loopback, br0 (LAN bridge) и трафик-к-TUN — это лишняя CPU-нагрузка на slow
MIPS. Имя WAN определяется автодетектом через `ip route show default` до
запуска DPI-rules.

**`-m mark ! --mark 0x53`** на jump-правиле: loop-prevention — исключает
self-traffic sing-box. Sing-box ставит SO_MARK=0x53 на свои исходящие
соединения; такие пакеты обходят DPI-обработку, что гарантирует:
1. Reality-маскировка VPN-handshake к собственному VPS не ломается
   nfqws2-десинхронизацией.
2. Производительность sing-box не страдает от лишних копирований
   через netlink.

**RETURN для VPN-эндпоинтов**: для каждого IP из `state.dpi_exclude_ips` (плюс
best-effort резолв первого `outbounds[].server`) вставляется правило
`-d <ip> -j RETURN` ПЕРЕД NFQUEUE-портами. Это защищает соединения
downstream-устройств (Beelink, RPi4) с собственным VPN-клиентом к тому же
VPN-эндпоинту — их трафик не имеет mark 0x53, поэтому без RETURN-правила
прошёл бы через nfqws2.

xt_multiport не загружен в стоковом ядре Keenetic 4.9, поэтому `--dport` —
одно правило на порт/диапазон.

**Почему `signcraze_dpi_fwd` в FORWARD, а не PREROUTING / POSTROUTING**
(refactor 2026-05-12, исправляет архитектурный bug commit `bc6361e`):

- **PREROUTING** (изначальный план): ловит пакет ДО routing-decision. Но
  LAN policy-устройства тут же перехватываются TPROXY-правилами на
  `mangle PREROUTING` и уходят в local socket — десинхронизация на их
  ClientHello теряется, потому что sing-box потом формирует свой
  собственный исходящий поток к ISP.
- **POSTROUTING** (legacy commit `bc6361e`): ловит ПОСЛЕ routing-decision,
  только исходящий трафик sing-box к VPS. На нём DPI бесполезен (Reality
  обходит) и не покрывает LAN не-policy устройств вообще — они идут
  стандартным маршрутом FORWARD → eth3, минуя нашу POSTROUTING-chain.
- **FORWARD** (текущая архитектура): ловит трафик ПОСЛЕ routing-decision
  для пакетов, идущих через роутер транзитом. LAN policy-устройства уже
  ушли через TPROXY в sing-box (в FORWARD не доходят). LAN не-policy идут
  напрямую через ISP — попадают в `signcraze_dpi_fwd` → nfqws2 десинкает.
  Self-traffic sing-box отсекается `! --mark 0x53`.

**Фильтр `--dport 443`** ограничивает обработку только TLS+QUIC — основной
канал DPI-блокировок. HTTP (port 80) включён для случаев когда ISP блокирует
по Host-header. Discord voice-порты включены для разблокировки голосовых
каналов.

`--queue-bypass`: если nfqws2 не запущен, пакеты проходят без обработки.

#### ip rule + ip route (маршрутизация помеченных пакетов в TUN)

```plain
ip rule:    32765: from all fwmark 0x53 lookup 83
ip route:   table 83: default dev signbox-tun
```

`mark=0x53` ставится iptables-правилами в `signcraze` (для policy) или
`signcraze_full` (для full mode). Пакеты с этой меткой уходят в TUN-интерфейс
`signbox-tun`, где их принимает sing-box. Loop-prevention: sing-box сам отправляет
исходящие пакеты без метки (через `direct` outbound), поэтому повторного захвата нет.

#### IP Policy в Keenetic RCI

```json
{
  "sign-craze": {
    "description": "sign-craze",
    "mark": "ffffaab",
    "table4": 4098,
    "table6": 4098,
    "permit": [{"enabled": true, "interface": "GigabitEthernet1"}],
    "multipath": true
  }
}
```

Создаётся при первом `--start` через RCI POST на `127.0.0.1:79/rci/`.
Сохраняется в startup-config через `/rci/system/configuration/save`.

#### Что НЕ создаётся в режиме policy

- ipset (`signcraze_ipv4/v6/excludes`) — не нужен, src-фильтрацию делает Keenetic.
- Цепочки `signcraze`, `signcraze_full`, `signcraze_dpi`, `signcraze_ports`.

### 3b. Режим `full` (legacy)

Эквивалент бывшего hybrid. Включается через `--mode full`.

#### iptables (mangle)

```plain
Chain PREROUTING (policy ACCEPT)
  -j signcraze_dpi          # цепочка DPI первой (пустая если DPIEnabled=false)
  -j signcraze              # mark-маршрутизация

Chain signcraze
  -m set --match-set signcraze_ipv4 dst -j MARK --set-mark 0x53 -m comment --comment "signcraze:mark-ipv4"
  -m set --match-set signcraze_ipv6 dst -j MARK --set-mark 0x53 -m comment --comment "signcraze:mark-ipv6"

Chain signcraze_full
  -p tcp -j MARK --set-mark 0x53 -m comment --comment "signcraze:mark-full-tcp"
  -p udp -j MARK --set-mark 0x53 -m comment --comment "signcraze:mark-full-udp"

Chain signcraze_dpi  # пустая если DPIEnabled=false
  -m mark ! --mark 0x53 -p tcp -j NFQUEUE --queue-num 300 --queue-bypass -m comment --comment "signcraze:dpi-tcp"
  -m mark ! --mark 0x53 -p udp -j NFQUEUE --queue-num 300 --queue-bypass -m comment --comment "signcraze:dpi-udp"
```

#### ip rule + ip route + ipset

```plain
ip rule:   32765: from all fwmark 0x53 lookup 83
ip route:  table 83: default dev signbox-tun
ipset:     signcraze_ipv4   Type: hash:net  Family: inet
           signcraze_ipv6   Type: hash:net  Family: inet6
           signcraze_excludes  Type: hash:net  Family: inet
```

### 3c. NDM netfilter.d hook (persistence)

Keenetic NDM пересобирает iptables при изменениях конфигурации (привязка
устройства к IP-policy через web UI, save startup-config, reconnect WAN).
После rebuild сторонние chain'ы из mangle (включая `signcraze_policy`)
теряются. ip rule и таблицы маршрутизации NDM не трогает.

Стандартный механизм восстановления — hook-скрипты в
`/opt/etc/ndm/netfilter.d/`, NDM вызывает их после каждого rebuild с env:

- `type` — `iptables` | `ip6tables` | `ipset`
- `table` — `mangle` | `nat` | `filter` | `raw` (для iptables/ip6tables)

`--install` создаёт `/opt/etc/ndm/netfilter.d/50-sign-craze` (executable).
Hook фильтрует event'ы по `type`/`table` (только mangle и ipset) и вызывает
`sign-craze --reapply`.

`--reapply` — hidden CLI-команда:

1. **Throttle 5 s** (v0.8.0): проверяет mtime маркера
   `/opt/var/run/sign-craze-reapply.last`. Если с последнего reapply прошло
   менее 5 с — exit 0. NDM netfilter.d hook генерирует пачку 5–10 вызовов за
   секунду при NDM-событиях; throttle предотвращает шторм reapply на медленном
   MIPS softfloat. После успешного apply маркер обновляется через `os.Chtimes`.
2. Non-blocking flock на `/opt/var/lock/sign-craze.lock`. При занятой блокировке
   (другой mutator работает) — exit 0; mutator сам сделает Apply.
3. Status sing-box: если процесс не запущен — exit 0 (правила без живого TUN
   маршрутизировали бы в несуществующий dev).
4. `Applier.Reconcile(ctx, mode)` — idempotent re-apply (не `Apply`): не
   выполняет pre-flight проверки и auto-rollback. Не вызывает `AttachTUN`.
5. Все ошибки логируются как Warn, exit 0 ВСЕГДА — hook не должен ломать
   обработку NDM event-chain.

`--uninstall` удаляет hook вместе с другими установленными файлами.

#### Окно reapply: leak-анализ

Между NDM flush и завершением hook'а (~100-500ms на MIPS softfloat) chain
`signcraze_policy` отсутствует. Пакет клиента в этом окне:

1. Keenetic mark пакета остаётся (`0xffffaab` или похожий).
2. Без `signcraze_policy` ремаркинга в `0x53` нет → ip rule
   `32765 fwmark 0x53 lookup 83` (TUN) не срабатывает.
3. Пакет идёт по Keenetic-правилам:
   - `ip rule 102: fwmark 0xffffaab lookup 4098` — Keenetic-managed table
     policy. Sign-craze туда не пишет; у policy без выходного канала пуста.
   - `ip rule 103: fwmark 0xffffaab blackhole` — auto-добавлено Keenetic'ом
     как fail-closed.
4. Пакет → `blackhole` → дроп.

**Утечки нет**: пакеты блекхольнутся пока reapply не вернёт chain. Side-effect —
кратковременная потеря connectivity у клиентов в политике.

**Известное ограничение**: если оператор вручную назначит sign-craze policy
WAN-fallback в Keenetic UI (`permit` с активным interface), table 4098
получит default через провайдера и в окне reapply возможен leak. Sign-craze
создаёт policy без `permit`-канала (`ndm.EnsurePolicy` без interface-параметра)
именно для предотвращения этого сценария.

---

## 4. Жизненный цикл

### Загрузка / init.d

Keenetic вызывает `/opt/etc/init.d/S99signcraze start` при старте Entware.
Shim делегирует в `sign-craze --service-start` (внутренняя команда, не отображается в help).

### `--service-start` (внутренняя)

Аналог `--start` для контекста init.d:

- Без интерактивного вывода (всё в slog).
- Ждёт сети: опрашивает `ip route show default` до 30 с.
- При ошибке: логирует ERROR, выходит с ненулевым кодом (init.d не будет повторять, syslog зафиксирует).

### `--start` в режиме `policy`

Перед применением iptables sign-craze выполняет последовательность:

1. `ndm.DetectWANInterface()` — найти Keenetic-имя WAN (или взять закешированный).
2. `ndm.EnsurePolicy(name=PolicyName, description=PolicyName, wanIface=<WAN>)` —
   идемпотентно создать или подтвердить policy через RCI.
3. `ndm.WaitForMark()` — дождаться, пока Keenetic присвоит mark (обычно <5s).
4. `ndm.SaveConfig()` — записать в startup-config (иначе policy не переживёт reboot).
5. Закешировать `mark`/`table4`/`wan_interface` в state.json.
6. Применить iptables (PolicyRules + опц. PolicyDPIRules).

Если RCI на `127.0.0.1:79` недоступен (например, sign-craze запущен в Docker
для тестов): `--start` возвращает ошибку с подсказкой переключиться в
`--mode full`.

### `--uninstall` в режиме `policy`

Перед удалением sing-box и iptables выполняется следующая последовательность:

1. `ndm.GetHostsWithPolicy(ctx, PolicyName)` — получить список MAC-адресов, привязанных к данной policy через `ip hotspot host`.
2. Для каждого MAC — `ndm.UnsetHostPolicy(ctx, mac)` (RCI: `{"ip":{"hotspot":{"host":{"mac":"<MAC>","policy":{"no":true}}}}}`). Ошибки логируются как WARN и не прерывают процесс.
3. `ndm.DeletePolicy(ctx, PolicyName)` — удалить само определение policy.
4. `ndm.SaveConfig()` — сохранить изменения в startup-config.

Без шага 2 сиротские записи `ip hotspot host <MAC> policy <name>` сохраняются в startup-config Keenetic (flash) и восстанавливаются NDM при каждом reboot — устранить их без factory reset невозможно.

Ошибки RCI на любом из шагов логируются как WARN, но не прерывают uninstall.

### Мониторинг процессов

Process watchdog реализован через `--service-watchdog` (см. ниже). Демон запускается init.d shim `S99signcraze` и автоматически восстанавливает firewall-правила при их пропаже. Восстановление после краша sing-box — через перезапуск init.d или вызов `--restart`.

### Firewall watchdog (`--service-watchdog`)

Watchdog — автономный демон, запускаемый init.d shim `S99signcraze` через
`nohup sign-craze --service-watchdog &`. Независим от `--ui on`: переживает
перезапуск web-интерфейса и работает даже при отключённом UI. PID-файл —
`/opt/var/run/sign-craze-watchdog.pid`.

Алгоритм: каждые 30 с выполняет реконсиляцию:

1. `IPTables.CheckCriticalRules(ctx, mode, keenMark)` — проверяет наличие критических правил через `iptables -C` (cheap, без полного Apply).
2. Если правила на месте — ничего не делать.
3. Если правила отсутствуют — вызвать `Applier.Reconcile(ctx, mode)`: idempotent re-apply без pre-flights и auto-rollback.
4. Все ошибки логируются как Warn.

**Lifecycle**: watchdog запускается shim'ом при старте Entware и остаётся живым независимо от состояния `--ui on/off`. Завершается при `S99signcraze stop`.

### Миграция routing.json TUN→TPROXY (v0.8.3)

При генерации конфига sing-box (`configParamsFromState`) если `state.inbound =
tproxy`, функция фильтрует TUN-inbound из `RoutingConfig.Inbounds` (тип `"tun"`)
и пересохраняет `routing.json`. Это устраняет проблему установок v0.6.x→v0.8.x:
после апгрейда до tproxy bootstrap мог оставить TUN inbound в routing.json, из-за
чего sing-box стартовал в TUN-режиме, тогда как iptables направляли marked трафик
в `127.0.0.1:7895` (TPROXY-порт), где никто не слушал.

### `--stop` при уже остановленном сервисе

Идемпотентно. Если PID-файл отсутствует или процесс не найден: логирует INFO "не запущен", выходит с кодом 0.

### `--start` при уже запущенном сервисе

Возвращает ошибку "уже запущен (pid X)". Kill + restart **не выполняет**.

### Перезагрузка конфига

SIGHUP-перезагрузки нет. Изменения конфига требуют `--restart`.

---

## 5. Файловая система (полная схема)

```plain
/opt/
├── sbin/
│   ├── sign-craze          # основной бинарь
│   ├── sing-box            # скачанный бинарь sing-box
│   └── mieru               # бинарь mieru-клиента (только если в outbound есть mierus://|mieru://)
├── etc/
│   ├── sign-craze/
│   │   ├── config.json             # конфиг sing-box (генерируется)
│   │   ├── nfqws2.conf             # конфиг nfqws2 (генерируется, опционально)
│   │   ├── dpi-hostlist.txt        # SNI-цели Selective DPI (генерируется, опционально)
│   │   ├── mieru-<tag>.conf.json   # конфиг mieru-клиента (генерируется, mode 0600, один файл на mieru-outbound)
│   │   └── admin.creds             # bcrypt-хэш пароля для web UI
│   ├── init.d/
│   │   └── S99signcraze    # init.d shim (генерируется)
│   └── ndm/
│       └── netfilter.d/
│           └── 50-sign-craze   # NDM hook для persistence (генерируется)
└── var/
    ├── lib/
    │   └── sign-craze/
    │       ├── geo/        # *.srs rule-set файлы
    │       └── backups/    # резервные копии конфигов с меткой времени
    ├── lock/
    │   └── sign-craze.lock
    ├── log/
    │   └── sign-craze/
    │       ├── sign-craze.log
    │       ├── sing-box.log
    │       └── mieru-<tag>.log     # stderr supervised peer mieru (опционально)
    └── run/
        ├── sign-craze-singbox.pid
        ├── sign-craze-nfqws2.pid
        ├── sign-craze/
        │   └── mieru-<tag>.pid     # PID-файл supervised peer mieru
        └── sign-craze-reapply.last   # mtime-маркер throttle --reapply (v0.8.0)
/opt/share/sign-craze/
└── ipset.dump        # snapshot ipset для reboot-survival (см. internal/firewall/ipset_persist.go)
/opt/var/lib/sign-craze/cache.db   # кеш SRS rule-set (sing-box experimental.cache_file)
```

---

## 6. Web UI API

Запускается командой `--ui on`, останавливается `--ui off`. HTTP-серверы встроены в основной процесс — отдельный процесс не создаётся.

### Аутентификация

Все эндпоинты обоих портов доступны без аутентификации (auth удалён).

#### bcrypt cost по архитектурам

При наличии защищённого доступа (`admin.creds`) bcrypt cost зависит от целевой архитектуры:

- **MIPS / MIPSLE** (Keenetic softfloat, ~400 MHz): `cost=10` — login ≤ 2s.
- **arm64, arm7, amd64**: `cost=12` — рекомендуемый production минимум.

Выбор происходит через Go build tags (`//go:build mips || mipsle`) — без runtime-проверки.

### Порт 9091 — Admin REST API

```plain
GET    /api/status              — состояние сервисов и версии
GET    /api/config              — текущий config.json sing-box (raw JSON)
POST   /api/config              — обновить config.json (валидация sing-box check -c перед сохранением)
GET    /api/ports               — список проксируемых портов
POST   /api/ports               — добавить порт, body: {"port": 80}
DELETE /api/ports/{port}        — удалить порт
GET    /api/excludes            — список IP/CIDR-исключений
POST   /api/excludes            — добавить исключение, body: {"cidr": "192.168.1.0/24"}
DELETE /api/excludes/{cidr}     — удалить исключение
GET    /api/dpi/targets         — текущий список DPI-целей (SNI); пустой массив = всё трафик
PUT    /api/dpi/targets         — задать список, body: {"targets": ["discord.com","youtube.com"]} или {"targets": []} для clear
GET    /api/dpi/presets         — встроенные пресеты: discord, youtube, discord-youtube
POST   /api/dpi/presets/{name}/apply — применить пресет по имени
```

Формат ответа `GET /api/status`:

```json
{
  "singbox":  {"running": true,  "pid": 1234},
  "nfqws2":   {"running": false, "pid": 0},
  "mode":     "policy",
  "core":     "<active_core>",
  "version":  {"sign_craze": "v1.4.2", "core": "v<core-version>"},
  "uptime_s": 3600
}
```

Коды ответов: `200 OK`, `400 Bad Request` (ошибка валидации), `500 Internal Server Error`.

### Инвариант: LAN-only доступ к Web UI

**Inv-Web-LAN-Only**: web-серверы (9090/9091/9092) слушают на `0.0.0.0`; правила в `filter/INPUT` (owner-комментарий `signcraze:wan-block`, идемпотентно через `EnsureRule`) дропают входящий трафик на порт 9090 от WAN-интерфейса (`DetectISPInterface`). Правила применяются при `--ui on` и снимаются при `--ui off`. Из локальной сети доступ открыт.

---

## 7. Supervised peers — внешние процессы, управляемые sign-craze

Раздел описывает протоколы, которые **не реализованы ни одним из встроенных ядер (sing-box / xray / mihomo)** и требуют запуска отдельного клиентского бинаря рядом с ядром. На момент v0.X.Y единственный такой случай — **mieru** (https://github.com/enfein/mieru). Архитектурное обоснование — ADR-0020.

### 7.1. mieru — URL-форматы импорта

sign-craze принимает два URL-формата mieru (флаг `--proxy` команды `--install`):

1. **`mierus://<user>:<pass>@<host>?port=<N>&protocol=TCP|UDP&multiplexing=<level>&profile=<name>#<tag>`**
   Human-readable, экспортируется командой mieru `mieru export config simple`. Обязательные поля: `user`, `pass`, `host`, `port`. Опциональные: `protocol` (default TCP), `multiplexing` (`MULTIPLEXING_OFF|LOW|MIDDLE|HIGH`, default `MULTIPLEXING_LOW`), `profile` (default `default`), `#<tag>` (default `mieru-out`).

2. **`mieru://<base64-protobuf>`**
   Полный protobuf-config клиента; экспортируется командой `mieru export config`. Парсится ручным wire-format декодером — sign-craze извлекает только: `profile.user.{name,password}`, `profile.servers[0].{ipAddress|domainName, portBindings[0].{port,protocol}}`, `profile.multiplexing.level`. Остальные поля игнорируются.

### 7.2. Состояние и порт

В `Outbound.Proto` для protocol=`mieru` дополнительно сохраняются:

| Поле | Описание |
|---|---|
| `MieruUsername` | имя пользователя mieru (для auth) |
| `MieruPassword` | пароль mieru |
| `MieruPort` | удалённый порт mieru-сервера |
| `MieruProtocol` | `TCP` или `UDP` |
| `MieruMultiplexing` | `MULTIPLEXING_OFF|LOW|MIDDLE|HIGH` |
| `MieruProfile` | имя профиля mieru (default `default`) |
| `MieruLocalPort` | локальный SOCKS5-порт, выделенный sign-craze (см. 7.3) |

**Inv-Peer-State-Persist**: `MieruLocalPort` персистируется в `state.json` и переиспользуется между `--start`/`--restart`. Меняется только при `--install --reinstall` или ручном удалении outbound из state.

### 7.3. Port allocation

sign-craze выделяет `MieruLocalPort` при первом `--start` после `--install`. Алгоритм:

1. Если `MieruLocalPort > 0` уже задан в outbound — используется без изменений (idempotent).
2. Иначе sign-craze пробует порты `40000 + idx*100`, `40000 + idx*100 + 1`, … (где `idx` — индекс outbound в `state.Outbounds`), через `net.Listen("tcp", "127.0.0.1:<port>") + Close`, пропуская занятые. Первый успешный — назначается и сохраняется в state.
3. Диапазон ограничен `40000–49999`. При выходе за диапазон — ошибка `peer: не удалось выделить локальный SOCKS5-порт для mieru<tag>`.

**Inv-Peer-Port-Idempotent**: повторный `--start` без изменений в outbound НЕ меняет `MieruLocalPort`.

### 7.4. Lifecycle и порядок

`--start` (упорядоченно):

1. Загрузка state, валидация.
2. `peer.AllocateMieruPorts` (только для mieru-outbound'ов; persist в state).
3. `peer.WriteMieruConfig` для каждого mieru-outbound → `/opt/etc/sign-craze/mieru-<tag>.conf.json` (mode 0600, atomic write).
4. `peer.StartMieruPeers` (по одному `service.Lifecycle.Start` на каждый mieru-outbound).
5. `ensureConfigFreshForCore` (sing-box config содержит `socks` outbound `127.0.0.1:<MieruLocalPort>`).
6. `applier.Apply` (firewall).
7. `coreLC.Start` (sing-box).

`--stop` — обратный порядок:

1. `stopWatchdog`.
2. `coreLC.Stop` (sing-box).
3. `peer.StopMieruPeers` (SIGTERM, grace 10 s, затем SIGKILL).
4. `firewall.Remove`.

**Inv-Peer-Order**: mieru стартует **до** sing-box; останавливается **после** sing-box. Любое отклонение от этого порядка — баг.

### 7.5. sing-box render для mieru

`renderCanonical(o)` при `o.Protocol == ProtocolMieru` строит **SOCKS5-bridge outbound**:

```json
{
  "type": "socks",
  "tag":  "<o.Tag>",
  "server": "127.0.0.1",
  "server_port": <o.Proto.MieruLocalPort>,
  "version": "5"
}
```

Если `MieruLocalPort == 0` — render возвращает ошибку `singbox render: mieru: LocalSocksPort не выделен (запустите --start)`. Это страховка: render не должен вызываться до `peer.AllocateMieruPorts`.

### 7.6. Файлы

| Путь | Описание | Mode |
|---|---|---|
| `/opt/sbin/mieru` | бинарь mieru-клиента | 0755 |
| `/opt/etc/sign-craze/mieru-<tag>.conf.json` | конфиг mieru-клиента (генерируется) | 0600 |
| `/opt/var/run/sign-craze/mieru-<tag>.pid` | PID-файл supervised peer | 0644 |
| `/opt/var/log/sign-craze/mieru-<tag>.log` | stderr + stdout mieru-процесса | 0640 |

### 7.7. init.d shim — отсутствует

mieru **не** имеет собственного S97mieru.sh init.d shim. Управление полностью делегировано sign-craze: `S99signcraze --service-start` через `doStart` поднимает все mieru-peers до старта sing-box. Соответствует существующему паттерну (sing-box и nfqws2 тоже не имеют отдельных init.d shim'ов).

### 7.8. CLI-команды (диагностика)

- `sign-craze --status` — в JSON-выводе появляется секция `"peers": [{"name":"mieru-<tag>","running":true,"pid":1234,"local_port":40000}, ...]`.
- `sign-craze --diag` — в support-bundle включается `mieru-<tag>.log` с redaction username/password в первой строке конфига (см. 7.9).

### 7.9. Redaction в логах и diag

При диагностическом выводе sign-craze redact'ит `username`/`password` mieru-клиента — заменяет на `***`. Это касается:

- `--diag` support-bundle (включает `mieru-<tag>.conf.json` и хвост `mieru-<tag>.log`).
- Сообщений об ошибках при failure mieru.Start.

Файлы `/opt/etc/sign-craze/mieru-<tag>.conf.json` на диске остаются полноценными (mode 0600).

### 7.10. Не-поддерживается

- **mieru-сервер (mita)**: sign-craze управляет только клиентским режимом mieru. Развёртывание сервера — задача оператора VPS.
- **HTTP-proxy режим mieru**: sign-craze всегда использует mieru как SOCKS5-bridge.
- **mieru без profile**: импорт URL без `profile=` использует `default` (первый профиль).

---

## 8. Supervised peers — naiveproxy

naiveproxy (https://github.com/klzgrad/naiveproxy) — второй supervised peer в sign-craze. Архитектурное обоснование — ADR-0021.

### Протокол naive (klzgrad/naiveproxy)

**URL формат:**
```
naive+https://<user>:<password>@<server>:<port>[?padding=true][&extra-headers=<encoded>]
naive+quic://<user>:<password>@<server>:<port>
```

**Архитектура:** process chain. sign-craze запускает upstream `klzgrad/naiveproxy` бинарь
как daemon на `127.0.0.1:18443` (default), и подключает sing-box через локальный SOCKS5
outbound. См. ADR-0021-naiveproxy-process-chain.

**CLI:**
- `--install --proxy naive+https://...` — установка с naive в качестве outbound.
- `--with-naive` — опт-ин на download/install naive бинаря при `--install` без `--proxy`.
- `--update-naive` — обновление naive бинаря с GitHub.

**Пути:**
- бинарь: `/opt/sbin/naive`
- PID: `/opt/var/run/sign-craze-naive.pid`
- stderr-лог: `/opt/var/log/sign-craze/naive.stderr.log`
- кеш tarball: `/opt/var/lib/sign-craze/cache/naive/`

**Поддерживаемые архитектуры:** arm64, armv7, mipsle (LE). **mips (big-endian) не
поддерживается** — klzgrad публикует только linux-mipsel (little-endian).

**Ограничения MVP:** ровно один naive outbound в state. Множественные naive
outbounds — Phase 2 (требует port allocator).

### 8.4 Lifecycle naive (порядок старт/стоп)

**Старт** (`--start`, после `core.Start`):
1. `core.Start` — поднять sing-box/xray/mihomo с outbound `naive` (process chain).
2. `naive.Start` — запустить бинарь `/opt/sbin/naive` с конфигом `/opt/etc/sign-craze/naive/config.json`, PID в `/opt/var/run/sign-craze-naive.pid`.
3. Проверка: TCP `127.0.0.1:<local_port>` отвечает в течение 5 с — иначе откат.

**Стоп** (`--stop`, обратный порядок):
1. `naive.Stop` — SIGTERM → ждать 5 с → SIGKILL → удалить PID-файл.
2. `core.Stop` — стандартный lifecycle ядра.

**Инвариант Inv-Naive-Order**: naive всегда стартует ПОСЛЕ ядра и стопится ДО ядра (process chain dependency). Нарушение → coredump в naive или зависшие соединения.

**Watchdog (Phase 2 из ADR-0021, pending)**: автоматический рестарт naive при крахе через `--service-watchdog`. Текущий статус — отслеживается в `tasks/todo.md` Phase 16.

**Логи**: stderr-лог `/opt/var/log/sign-craze/naive.stderr.log`. PID watchdog (после Phase 2): `/opt/var/run/sign-craze-naive-watchdog.pid`.

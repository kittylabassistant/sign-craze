# BEHAVIOR_SPEC.md

Функциональная спецификация sign-craze, написанная в режиме clean-room.
Исходники XKeen не читались. Только публичные источники.

> **v0.3.0 (breaking)**: TPROXY-режим заменён на TUN-режим. Стоковое
> ядро/iptables на Keenetic mipsel-3.4 не имеет xt_TPROXY. Sign-craze теперь
> использует sing-box `tun` inbound (interface_name=`signbox-tun`,
> address=`172.19.0.1/30`, stack=`system`, auto_route=`false`); iptables
> ставит только MARK + переход в нашу цепочку, а ip rule fwmark → table →
> default через `signbox-tun` поднимает помеченные пакеты в TUN. Разделы
> ниже про TPROXY — историческое описание (refresh в отдельном коммите).

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
- Создаёт init.d shim `/opt/etc/init.d/S05signcraze`, права `0755`.
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
sing-box:  запущен  (pid 1234)
nfqws2:    остановлен
режим:     proxy
версия:    sign-craze v0.1.0 / sing-box v1.10.0
```

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

### `--config-backup` / `--config-restore`

Аналог backup/restore только для config.json.

---

### `--ui on|off`

`on`: запускает встроенные HTTP-серверы:
- порт `9090` — Zashboard (Clash-совместимый dashboard, управление прокси и мониторинг трафика)
- порт `9091` — admin REST API sign-craze
- порт `9092` — Routing Editor SPA

Все серверы слушают на `0.0.0.0`. Iptables-правила в chain `signcraze_local` дропают входящий трафик на порты 9090/9091/9092 с WAN-интерфейса (определяется через `DetectISPInterface`). Доступ из LAN открыт без аутентификации.

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

```
sign-craze --dpi-targets discord.com,youtube.com,googlevideo.com
sign-craze --dpi-targets clear
```

### `--dpi-targets-list`

Печатает текущий список DPI targets (один на строку) либо сообщение, что desync
применяется ко всему трафику.

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

Минимальная структура tproxy inbound:

```json
{
  "log": { "level": "info", "output": "/opt/var/log/sign-craze/sing-box.log" },
  "inbounds": [
    {
      "type": "tproxy",
      "tag": "tproxy-in",
      "listen": "::",
      "listen_port": 7895,
      "tcp_fast_open": false,
      "sniff": true,
      "sniff_override_destination": false,
      "domain_strategy": "prefer_ipv4",
      "mark": 83
    }
  ],
  "outbounds": [ /* задаётся пользователем */ ],
  "route": { /* правила с SRS rule-set */ }
}
```

Ключевые параметры (настраиваются через sign-craze):

- `listen_port`: по умолчанию `7895`
- `mark`: `83` (= `0x53`) — пакеты от sing-box перемаркируются для предотвращения петли
- Уровень логирования: зеркалирует `SIGNCRAZE_LOG_LEVEL`

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

```
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
  -j signcraze_policy

Chain POSTROUTING
  -j signcraze_policy_dpi   # только если DPIEnabled=true

Chain signcraze_policy
  ! -s 127.0.0.0/8 ! -s 169.254.0.0/16 ! -i lo \
    -m mark --mark 0xffffaaXX -p tcp -j TPROXY --tproxy-port 7895 --tproxy-mark 0x53
    -m comment --comment "signcraze:tproxy-tcp"
  ! -s 127.0.0.0/8 ! -s 169.254.0.0/16 ! -i lo \
    -m mark --mark 0xffffaaXX -p udp -j TPROXY --tproxy-port 7895 --tproxy-mark 0x53
    -m comment --comment "signcraze:tproxy-udp"

Chain signcraze_policy_dpi  # только если DPIEnabled=true; в POSTROUTING
  -p tcp --dport 443 -j NFQUEUE --queue-num 300 --queue-bypass
  -p udp --dport 443 -j NFQUEUE --queue-num 300 --queue-bypass
```

**Почему `signcraze_policy_dpi` в POSTROUTING, а не PREROUTING:** в режиме
`policy` весь LAN-трафик с keenetic-mark переотмечается в `0x53` → ip rule
→ table 83 → signbox-tun. Sing-box получает соединение и **сам формирует
исходящий ClientHello к ISP**. Если NFQUEUE стоит в PREROUTING,
nfqws2-десинхронизация применяется к пакету от LAN-клиента, который
sing-box не пробрасывает — он создаёт свой собственный поток. Десинхрон
теряется, ISP видит чистый ClientHello и блокирует SNI=youtube.com.
NFQUEUE в POSTROUTING ловит ClientHello ПОСЛЕ sing-box, на пути к ISP.

**Фильтр `--dport 443`** ограничивает обработку только TLS+QUIC — основной
канал DPI-блокировок. HTTP (port 80) обычно не блокируется и не нуждается
в desync. Без фильтра NFQUEUE захватывал бы весь POSTROUTING-трафик,
включая loopback и TUN-интерфейсы (где DPI не нужен).

`--queue-bypass`: если nfqws2 не запущен, пакеты проходят без обработки.

#### ip rule + ip route (loop-prevention для sing-box)

```plain
ip rule:    32765: from all fwmark 0x53 lookup 83
ip route:   table 83: local 0.0.0.0/0 dev lo
```

`mark=0x53` ставится sing-box'ом на исходящих сокетах (SO_MARK через
`tproxy-mark`), чтобы пакеты от прокси не попадали повторно в собственный
TPROXY.

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

```
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

```plain
Chain PREROUTING (TPROXY)
  -m mark --mark 0x53 -p tcp -j TPROXY --tproxy-port 7895 --tproxy-mark 0x53
  -m mark --mark 0x53 -p udp -j TPROXY --tproxy-port 7895 --tproxy-mark 0x53
```

#### ip rule + ip route + ipset (как раньше)

```plain
ip rule:   32765: from all fwmark 0x53 lookup 83
ip route:  table 83: local 0.0.0.0/0 dev lo
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

1. Non-blocking flock на `/opt/var/lock/sign-craze.lock`. При занятой блокировке
   (другой mutator работает) — exit 0; mutator сам сделает Apply.
2. Status sing-box: если процесс не запущен — exit 0 (правила без живого TUN
   маршрутизировали бы в несуществующий dev).
3. `applierImpl.Apply(ctx, mode)` идемпотентно восстанавливает chain'ы и
   правила. Не вызывает `AttachTUN` (TUN жив пока sing-box жив; default-route
   в table 83 NDM не трогает).
4. Все ошибки логируются как Warn, exit 0 ВСЕГДА — hook не должен ломать
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

Keenetic вызывает `/opt/etc/init.d/S05signcraze start` при старте Entware.
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

Перед удалением sing-box и iptables: `ndm.DeletePolicy()` + `SaveConfig()`.
Ошибки RCI логируются как WARN, но не прерывают uninstall.

### Мониторинг процессов

sign-craze не демонизирует sing-box через watchdog-петлю (вне scope v0.1).
Восстановление после краша — через перезапуск init.d или вызов `--restart`.

### Firewall watchdog (`--ui on`)

При запущенном `--ui on` стартует фоновая горутина `firewall.Watchdog` (интервал 30 с).
Алгоритм каждые 30 с:

1. `IPTables.CheckCriticalRules(ctx, mode, keenMark)` — проверяет наличие критических правил через `iptables -C` (cheap, без полного Apply).
2. Если правила на месте — ничего не делать.
3. Если правила отсутствуют — вызвать `Applier.Reconcile(ctx, mode)`: idempotent re-apply без pre-flights и auto-rollback.
4. Все ошибки логируются как Warn.

**Ограничение**: watchdog активен, только пока работает процесс `sign-craze --ui on`. При его отсутствии восстановление — только через NDM hook `--reapply`.

**Назначение**: страховка против NDM-rebuild-сценариев, которые event-driven hook пропускает (например, мягкий rebuild без сигнала netfilter.d).

### `--stop` при уже остановленном сервисе

Идемпотентно. Если PID-файл отсутствует или процесс не найден: логирует INFO "не запущен", выходит с кодом 0.

### `--start` при уже запущенном сервисе

Возвращает ошибку "уже запущен (pid X)". Kill + restart **не выполняет**.

### Перезагрузка конфига

SIGHUP-перезагрузки нет. Изменения конфига требуют `--restart`.

### Бэкап при обновлении

Если флаг `--backup-on-update` установлен в конфиге, команды `--update` и `--update-core` автоматически создают бэкап перед заменой бинаря.

---

## 5. Файловая система (полная схема)

```plain
/opt/
├── sbin/
│   ├── sign-craze          # основной бинарь
│   └── sing-box            # скачанный бинарь sing-box
├── etc/
│   ├── sign-craze/
│   │   ├── config.json         # конфиг sing-box (генерируется)
│   │   ├── nfqws2.conf         # конфиг nfqws2 (генерируется, опционально)
│   │   ├── dpi-hostlist.txt    # SNI-цели Selective DPI (генерируется, опционально)
│   │   └── admin.creds         # bcrypt-хэш пароля для web UI
│   ├── init.d/
│   │   └── S05signcraze    # init.d shim (генерируется)
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
    │       └── sing-box.log
    └── run/
        ├── sign-craze-singbox.pid
        └── sign-craze-nfqws2.pid
```

---

## 6. Web UI API

Запускается командой `--ui on`, останавливается `--ui off`. HTTP-серверы встроены в основной процесс — отдельный процесс не создаётся.

### Аутентификация

Все эндпоинты обоих портов доступны без аутентификации (auth удалён).

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
  "mode":     "proxy",
  "version":  {"sign_craze": "v0.1.0", "sing_box": "v1.10.0"},
  "uptime_s": 3600
}
```

Коды ответов: `200 OK`, `400 Bad Request` (ошибка валидации), `401 Unauthorized`, `500 Internal Server Error`.

### Инвариант: LAN-only доступ к Web UI

**Inv-Web-LAN-Only**: web-серверы (9090/9091/9092) слушают на `0.0.0.0`; iptables-правила в chain `signcraze_local` дропают входящий трафик на эти порты от WAN-интерфейса (`DetectISPInterface`). Правила применяются при `--ui on` и снимаются при `--ui off`. Из локальной сети доступ открыт.

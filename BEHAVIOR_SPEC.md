# BEHAVIOR_SPEC.md

Функциональная спецификация sign-craze, написанная в режиме clean-room.
Исходники XKeen не читались. Только публичные источники.

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

Останавливает сервис (если запущен), удаляет правила iptables, удаляет init.d shim, удаляет `/opt/sbin/sing-box`, директории конфигов и состояния. Бинарь sign-craze **не удаляет**.

---

### `--purge`

Аналог `--uninstall` плюс удаление бинаря sign-craze, всех логов, всех geo-файлов.

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

`on`: запускает встроенный HTTP-сервер на порту `9091` (admin UI) и `9090` (clash API / Zashboard). Генерирует `/opt/etc/sign-craze/admin.creds` (bcrypt), если файл отсутствует.
`off`: останавливает HTTP-сервер.

---

### `--dpi on|off`

Включает или отключает DPI-обход через nfqws2. При включении: скачивает бинарь nfqws2 если отсутствует, генерирует nfqws2.conf, добавляет цепочки NFQUEUE. При отключении: удаляет цепочки NFQUEUE, останавливает nfqws2.

### `--dpi-strategy <preset|file://путь>`

Устанавливает стратегию DPI. Пресеты берутся из релиза nfqws2-keenetic. Сохраняет в конфиг.

### `--dpi-update`

Скачивает последний tarball nfqws2-keenetic для текущей архитектуры. Сервис **не перезапускает**.

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

Источник: README nfqws2-keenetic upstream (MIT).

```sh
ISP_INTERFACE=eth0            # определяется через `ip route show default`
NFQWS_QUEUE_NUM=200
POLICY_NAME=signcraze
NFQWS_ARGS="--dpi-desync=fake,split2 --dpi-desync-ttl=5 --dpi-desync-fooling=md5sig"
NFQWS_ARGS_QUIC="--dpi-desync=fake --dpi-desync-ttl=5"
NFQWS_ARGS_UDP=""
```

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
  -j signcraze_policy_dpi   # только если DPIEnabled=true; фильтр по Keenetic-mark
  -j signcraze_policy

Chain signcraze_policy
  ! -s 127.0.0.0/8 ! -s 169.254.0.0/16 ! -i lo \
    -m mark --mark 0xffffaaXX -p tcp -j TPROXY --tproxy-port 7895 --tproxy-mark 0x53
    -m comment --comment "signcraze:tproxy-tcp"
  ! -s 127.0.0.0/8 ! -s 169.254.0.0/16 ! -i lo \
    -m mark --mark 0xffffaaXX -p udp -j TPROXY --tproxy-port 7895 --tproxy-mark 0x53
    -m comment --comment "signcraze:tproxy-udp"

Chain signcraze_policy_dpi  # только если DPIEnabled=true
  -m mark --mark 0xffffaaXX -p tcp -j NFQUEUE --queue-num 200 --queue-bypass
    -m comment --comment "signcraze:dpi-tcp"
  -m mark --mark 0xffffaaXX -p udp -j NFQUEUE --queue-num 200 --queue-bypass
    -m comment --comment "signcraze:dpi-udp"
```

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
  -m mark ! --mark 0x53 -p tcp -j NFQUEUE --queue-num 200 --queue-bypass -m comment --comment "signcraze:dpi-tcp"
  -m mark ! --mark 0x53 -p udp -j NFQUEUE --queue-num 200 --queue-bypass -m comment --comment "signcraze:dpi-udp"
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
(В будущем: флаг `--watch` или cron-проверка здоровья.)

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
│   │   ├── config.json     # конфиг sing-box (генерируется)
│   │   ├── nfqws2.conf     # конфиг nfqws2 (генерируется, опционально)
│   │   └── admin.creds     # bcrypt-хэш пароля для web UI
│   └── init.d/
│       └── S05signcraze    # init.d shim (генерируется)
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

Все эндпоинты обоих портов требуют HTTP Basic Auth.

- Логин: `admin`
- Пароль: задаётся при первом запуске (`--ui on` → случайный пароль выводится один раз на stdout)
- Хэш bcrypt (cost=12) хранится в `/opt/etc/sign-craze/admin.creds`
- При повторном `--ui on` пароль не меняется; для сброса — удалить `admin.creds`

### Порт 9090 — Clash-совместимый API (для Zashboard)

```plain
GET  /            — {"hello":"clash","version":"sign-craze/<ver>"}
GET  /version     — {"version":"<ver>","premium":false}
GET  /configs     — {"mode":"proxy|dpi|hybrid","port":7895,"log-level":"info"}
GET  /proxies     — {"proxies":{}}
GET  /connections — {"downloadTotal":0,"uploadTotal":0,"connections":[]}
WS   /traffic     — {"up":0,"down":0} каждые 1 с
WS   /logs        — {"type":"info","payload":"..."} поток slog
GET  /*           — SPA fallback → index.html (встроенный Zashboard)
```

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

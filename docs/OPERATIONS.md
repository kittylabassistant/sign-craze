# OPERATIONS.md — Runbook оператора sign-craze

> Версия: 2026-07-01.

## Содержание

1. [Обзор и предусловия](#1-обзор-и-предусловия)
2. [Установка](#2-установка)
3. [Запуск, остановка, перезапуск](#3-запуск-остановка-перезапуск)
4. [Переключение режимов](#4-переключение-режимов)
5. [Обновление](#5-обновление)
6. [Backup / Restore](#6-backup--restore)
7. [DPI](#7-dpi)
8. [Web UI](#8-web-ui)
9. [Recovery](#9-recovery)
10. [Reapply (NDM hook)](#10-reapply-ndm-hook)
11. [Uninstall](#11-uninstall)
12. [Diag-bundle](#12-diag-bundle)
13. [Auto-update hostlist](#13-auto-update-hostlist)
14. [VPN-exclude](#14-vpn-exclude)

---

## 1. Обзор и предусловия

**sign-craze** — Go single-binary менеджер межсетевого экрана для [Keenetic](https://help.keenetic.com/hc/ru). Управляет [sing-box](https://sing-box.sagernet.org/) (прокси-ядро), [iptables](https://www.netfilter.org/projects/iptables/index.html)/[ipset](https://ipset.netfilter.org/), DPI-обходом ([nfqws2](https://github.com/nfqws/nfqws2-keenetic)) и интеграцией с Keenetic IP Policy через RCI.

**Предусловия:**
- Keenetic с установленным [Entware](https://entware.net/) (`/opt` смонтирован, ≥ 30 МБ свободно)
- SSH-доступ с root (или `sudo`)
- `opkg install ipset curl` — обязательно перед установкой ([opkg](https://openwrt.org/docs/guide-user/additional-software/opkg))

**Ключевые пути:**

| Артефакт                    | Путь                                          |
|-----------------------------|-----------------------------------------------|
| Бинарь sign-craze           | `/opt/sbin/sign-craze`                        |
| Бинарь sing-box             | `/opt/sbin/sing-box`                          |
| Конфиги                     | `/opt/etc/sign-craze/`                        |
| Geo-файлы (.srs)            | `/opt/var/lib/sign-craze/geo/`                |
| Backups                     | `/opt/var/lib/sign-craze/backups/`            |
| Логи                        | `/opt/var/log/sign-craze/`                    |
| Init.d shim                 | `/opt/etc/init.d/S99signcraze`                |
| NDM hook                    | `/opt/etc/ndm/netfilter.d/50-sign-craze`      |
| ipset-дамп                  | `/opt/share/sign-craze/ipset.dump`            |

---

## 2. Установка

> Если sign-craze уже установлен (существует `state.json` или `/opt/sbin/sing-box`), `--install` завершится ошибкой. Используйте `--reinstall` для переустановки поверх.

### 2.1 Интерактивная установка

```sh
sign-craze --install
```

Запускает wizard: предлагает ввести URL прокси (`socks5://`, `vless://`, `vmess://`, `ss://`, `trojan://`) или пропустить (stub `direct`). Загружает sing-box из GitHub Releases, валидирует конфиг **до** записи бинаря, создаёт init.d shim и NDM hook.

### 2.2 Non-interactive (CI / скрипты)

```sh
sign-craze --install-auto
```

Создаёт stub `direct` outbound без вопросов. Настроить прокси можно через Web UI или напрямую в `state.json` + `--restart`.

### 2.3 Offline-установка из локального tarball

```sh
sign-craze --install-offline /tmp/sing-box-linux-mipsle.tar.gz
```

Tarball должен быть официальным архивом sing-box. Архитектура определяется из имени файла или хоста.

### 2.4 Переустановка поверх существующей

```sh
sign-craze --reinstall
```

Эквивалентно `--install-auto --force`. `state.json` **перезаписывается** — сделайте backup перед выполнением.

**Проверка после установки:**

```sh
sign-craze --status
ls /opt/etc/ndm/netfilter.d/50-sign-craze
ls /opt/etc/init.d/S99signcraze
```

---

## 3. Запуск, остановка, перезапуск

```sh
sign-craze --start      # применить firewall + запустить sing-box
sign-craze --stop       # остановить sing-box + убрать правила
sign-craze --restart    # stop + start
sign-craze --status     # состояние, режим, версии
```

**`--start` выполняет:**
1. Загружает `state.json`; проверяет наличие `sing-box` и `config.json`.
2. В режиме `policy`: обращается к Keenetic RCI, создаёт/обновляет IP-policy, кеширует `PolicyMark` и `PolicyTable` в `state.json`.
3. Применяет [iptables-restore](https://ipset.netfilter.org/iptables-restore.man.html)/ipset (`applier.Apply`).
4. Восстанавливает ipset-дамп из `/opt/share/sign-craze/ipset.dump`.
5. Запускает sing-box, ждёт TUN-интерфейс.
6. Если `DPIEnabled` — запускает nfqws2.

**`--service-start`** — скрытая команда, вызывается только init.d shim. Перед запуском ждёт default route (таймаут: `state.BootTimeoutSec`, по умолчанию 60 с). Ошибки → `/opt/var/log/sign-craze/boot.log`.

**Проверка:**

```sh
sign-craze --status
# sing-box:  запущен  (pid 1234)
# nfqws2:    остановлен
# режим:     policy
# версия:    sign-craze v1.6.4 / sing-box v1.13.x
```

---

## 4. Переключение режимов

| Режим               | Механизм                                        | Когда использовать                              |
|---------------------|-------------------------------------------------|-------------------------------------------------|
| `policy` (default)  | Keenetic RCI IP Policy + [fwmark](https://www.man7.org/linux/man-pages/man7/socket.7.html) из NDM | Выборочная маршрутизация через Keenetic UI      |
| `full`              | [ipset](https://ipset.netfilter.org/) `signcraze_ipv4/v6` по dst-IP             | Нет доступа к RCI / альтернативная прошивка     |

Устаревшие псевдонимы `proxy`, `dpi`, `hybrid` автоматически мигрируют в `policy`. Выбор режима также влияет на правила [ip rule (policy routing)](https://www.man7.org/linux/man-pages/man8/ip-rule.8.html).

```sh
sign-craze --mode policy   # переключить
sign-craze --mode full
sign-craze --restart       # применить — обязательно
```

**Pre-flight в режиме `policy`:** sign-craze вызывает `ndm.EnsurePolicy` — если RCI недоступен (нестандартная прошивка, CI), `--start` упадёт:

```
--start: ndm: EnsurePolicy: ... (укажите вручную через state.WANInterface)
```

В этом случае: `--mode full` или задать `WANInterface` в `state.json` вручную.

**Проверка:**

```sh
sign-craze --status          # режим: policy|full
ip rule show | grep 0x53     # [fwmark/SO_MARK](https://www.man7.org/linux/man-pages/man7/socket.7.html)-правило маршрутизации
```

---

## 5. Обновление

### 5.1 Обновить sign-craze (self-update)

```sh
sign-craze --update
```

Скачивает последний GitHub Release для текущей архитектуры, проверяет SHA256, атомарно заменяет `/opt/sbin/sign-craze`. Сервис **не перезапускается** автоматически - `--restart` после.

### 5.2 Обновить geo-файлы (SRS rule-set)

```sh
sign-craze --update-geo
```

Загружает [geosite/geoip/.srs](https://sing-box.sagernet.org/configuration/rule-set/) в `/opt/var/lib/sign-craze/geo/`, декомпилирует в CIDR, заполняет `signcraze_ipv4/ipv6`, сохраняет дамп в `/opt/share/sign-craze/ipset.dump`. Если sing-box ещё не установлен — заполнение ipset пропускается с предупреждением.

Рекомендуется через `cron`:

```sh
# /opt/etc/cron.weekly/sign-craze-geo
sign-craze --update-geo
```

### 5.3 Обновить только sing-box

```sh
sign-craze --update-core
```

Загружает новый бинарь sing-box, валидирует с текущим `config.json` **до** замены. При ошибке валидации старый бинарь остаётся.

**Откат sing-box:** атомарная запись сохраняет `.bak`:

```sh
cp /opt/sbin/sing-box.bak /opt/sbin/sing-box
sign-craze --restart
```

**Проверка:**

```sh
sign-craze --version
```

### 5.4 Multi-core — управление прокси-ядром

Sign-craze поддерживает несколько прокси-ядер: `sing-box` (по умолчанию), `xray`, `mihomo`.

```sh
sign-craze --core-list                    # список установленных ядер
sign-craze --core sing-box                # переключить активное ядро (требует --restart)
sign-craze --core xray
sign-craze --core mihomo
sign-craze --restart                      # применить смену ядра

sign-craze --core-install xray            # скачать и установить ядро из GitHub Releases
sign-craze --core-install mihomo
```

| Флаг | Описание |
|------|----------|
| `--core-list` | Показать все установленные ядра и текущее активное |
| `--core <sing-box\|xray\|mihomo>` | Переключить активное ядро; требует `--restart` |
| `--core-install <name>` | Скачать и установить ядро для текущей архитектуры |

> Смена ядра не пересоздаёт конфиг автоматически — конфиг генерируется заново при следующем `--restart`. `--restart` вызывает `ensureConfigFreshForCore` — генерирует конфиг только для **активного** ядра.

### routing.json — core-agnostic

`routing.json` не привязан к конкретному ядру. Он описывает routing-политику (inbound, outbounds, правила) на нейтральном уровне, а `Apply` транслирует его в формат активного ядра:

| Ядро | Формат конфига | Метод трансляции routing.json |
|------|---------------|-------------------------------|
| `sing-box` | JSON (config.json) | прямой маппинг, SRS rule-set |
| `xray` | JSON (config.json) | `geosite:`/`geoip:` prefix-матчер; `rule_set` не пишется |
| `mihomo` | YAML (config.yaml) | `.mrs` → `rule-providers` секция |

Правила из `routing.json` переносятся при смене ядра без изменений — никакой ручной правки не требуется.

### Что делает Apply per-core

**`--restart` (или Apply после `--core <name>`):**

1. `ensureConfigFreshForCore` определяет активное ядро через `state.Core`.
2. Генерирует конфиг ядра из `routing.json` + `state.json`.
3. Для **[xray](https://xtls.github.io/)**: `rule_set` URL-источники транслируются в `geosite:` / `geoip:` матчеры; `.dat`-файлы ядро ищет сам.
4. Для **[mihomo](https://wiki.metacubex.one/)**: `.srs` URL конвертируется в `.mrs` и прописывается в `rule-providers`.
5. Для **sing-box**: `.srs` rule-set передаётся напрямую; TUN inbound создаётся если `needsTUN(core) == true`.
6. Firewall (iptables/ipset) применяется через единый `Applier.Apply` — не зависит от ядра.

### Presets per-core

Web UI (`:9092`) и `--preset` транслируют preset-источники через per-core таблицу соответствия (`ruleSetSources`). URL пресета резолвится через `c.GeoFormat()` — правильный формат выбирается автоматически:

| Preset-источник | sing-box | xray | mihomo |
|----------------|----------|------|--------|
| geoip-ru | `.srs` URL | `geoip:ru` | `.mrs` URL |
| geosite-category-ads | `.srs` URL | `geosite:category-ads` | `.mrs` URL |
| custom domain list | inline domains | inline domains | inline domains |

Для смены preset после переключения ядра: откройте Web UI `:9092`, выберите preset, нажмите Apply — конфиг активного ядра будет перегенерирован.

### Несовместимости — warnings, не блокировки

`POST /api/validate` (и Web UI) поверхностно проверяют routing.json на несовместимости с активным ядром и возвращают warnings в JSON, но **не блокируют Apply**:

| Условие | Ядро | Уровень |
|---------|------|---------|
| TUN inbound в routing.json | xray, mihomo | warning |
| `.srs` URL в rule_set | xray | warning (нет поддержки SRS) |
| `.dat`-URL в rule_set | sing-box | warning |

Чтобы убрать warning TUN-inbound при переходе на xray — переапплите preset из Web UI: конфиг пересоздастся без TUN.

**Проверка:**

```sh
sign-craze --status    # active core: xray|mihomo|sing-box
curl http://localhost:9092/api/validate   # проверить warnings
```

### Установка с [mieru](https://github.com/enfein/mieru) (enfein/mieru)

`sign-craze --install --with-mieru` — скачивает и устанавливает бинарь [mieru](https://github.com/enfein/mieru) параллельно с sing-box. После установки выбрать outbound: `--proxy mierus://<server>:<port>?username=<user>&password=<pass>` или редактирование `routing.json` через UI.

### Установка с [naive](https://github.com/klzgrad/naiveproxy) (klzgrad/naiveproxy)

`sign-craze --install --with-naive` — скачивает и устанавливает бинарь [naiveproxy](https://github.com/klzgrad/naiveproxy) параллельно с sing-box. После установки выбрать outbound: `--proxy naive+https://<user>:<pass>@<server>:<port>` или редактирование `routing.json` через UI.

---

## 6. Backup / Restore

### 6.1 Полный backup

```sh
sign-craze --backup
# /opt/var/lib/sign-craze/backups/backup-2026-05-03T12-00-00.tar.gz
```

Архивирует весь `/opt/etc/sign-craze/` (config.json, state.json). Geo-файлы и кэш не включаются — восстанавливаются через `--update-geo`.

### 6.2 Восстановление

```sh
# Сервис останавливается автоматически
sign-craze --restore /opt/var/lib/sign-craze/backups/backup-2026-05-03T12-00-00.tar.gz
sign-craze --start
```

**Проверка:**

```sh
ls /opt/etc/sign-craze/
sign-craze --status
```

---

## 7. DPI

```sh
sign-craze --dpi on     # скачать nfqws2, сгенерировать конфиг, включить
sign-craze --dpi off
sign-craze --restart    # применить
```

Стратегия:

```sh
sign-craze --dpi-strategy default
sign-craze --dpi-strategy file:///opt/etc/sign-craze/nfqws2-custom.conf
```

Формат пути: `file://` + абсолютный путь (согласно справке `--dpi-strategy`).
Пример: `file:///opt/etc/sign-craze/nfqws2-custom.conf` — три слеша: два от `file://` + один от абсолютного пути.

Обновить бинарь:

```sh
sign-craze --dpi-update
```

### 7.1 Selective DPI (hostlist)

По умолчанию nfqws2 пытается обходить DPI для **всего** трафика. Если нужен desync только для конкретных доменов (SNI):

```sh
sign-craze --dpi-targets discord.com,youtube.com,googlevideo.com
sign-craze --restart   # nfqws2 подхватит --hostlist=<path> только после перезапуска
```

Встроенные пресеты доступны через Web API (`GET /api/dpi/presets`): `discord`, `youtube`, `discord-youtube`.

Сброс (desync для всего трафика):

```sh
sign-craze --dpi-targets clear
sign-craze --restart
```

Просмотр текущего списка:

```sh
sign-craze --dpi-targets-list
```

Файл hostlist: `/opt/etc/sign-craze/dpi-hostlist.txt` (генерируется автоматически, один домен на строку).

**Проверка:**

```sh
sign-craze --status   # nfqws2: запущен (pid ...)
sign-craze --dpi-targets-list
```

---

## 8. Web UI

```sh
sign-craze --ui on    # блокирующий вызов до SIGTERM
```

| Порт   | Назначение                                        |
|--------|---------------------------------------------------|
| `:9090`| Zashboard — Clash-совместимый dashboard (reverse proxy к mihomo/sing-box API) |
| `:9091`| Admin API (конфиг, порты, исключения, DPI targets) |
| `:9092`| Routing Editor SPA (правила маршрутизации)         |

> **Firewall watchdog**: при активном `--ui on` запускается фоновая проверка iptables-правил каждые 30 с. Если критические правила отсутствуют (например, NDM пересобрал цепочки без вызова hook'а), watchdog восстанавливает их через `Applier.Reconcile`. При отсутствии `--ui on` восстановление — только через NDM hook `50-sign-craze`.

**Аутентификация:** отсутствует (auth удалён в v1.6.1).

**Проверка:**

```sh
curl http://localhost:9091/api/status
```

---

## 9. Recovery

### 9.1 Партиальная установка прервана

Признак: `state.json` отсутствует, но `/opt/sbin/sing-box` существует (или наоборот).

```sh
sign-craze --reinstall   # idempotent
sign-craze --start
```

Если не помогает:

```sh
sign-craze --uninstall
sign-craze --install
```

### 9.2 sing-box не стартует

```sh
sign-craze --start 2>&1
cat /opt/var/log/sign-craze/sing-box.log | tail -30
cat /opt/var/log/sign-craze/sing-box.stderr.log
```

Частые причины:
- **Битый config.json** — `sign-craze --reinstall` или восстановить из backup
- **TUN занят** (`TUNSETIFF: device or resource busy`) — `--stop` принудительно удаляет TUN; повторить `--start`
- **Несовместимая архитектура** — `sign-craze --version`; `--update-core`

### 9.3 NDM hook сломан

```sh
ls -la /opt/etc/ndm/netfilter.d/50-sign-craze   # 0755
sign-craze --reinstall   # пересоздаёт shim и hook
```

Применить вручную без перезапуска sing-box:

```sh
sign-craze --reapply   # (hidden) — восстанавливает iptables-правила
```

### 9.4 IP-policy исчезла из Keenetic UI

Признак: `--diag` → `keenetic-policy FAIL: policy "sign-craze" не найдена в RCI`.

```sh
sign-craze --stop
sign-craze --start   # EnsurePolicy пересоздаст
```

Если RCI недоступен:

```sh
sign-craze --mode full
sign-craze --restart
```

### 9.5 Сталый lock-файл

Ошибка: `sign-craze: cannot acquire lock`.

```sh
sign-craze --diag                         # lock-free FAIL/PASS
kill $(cat /opt/var/run/sign-craze-singbox.pid)
sign-craze --stop
```

### 9.6 Откат после неудачного --update

```sh
cp /opt/sbin/sign-craze.bak /opt/sbin/sign-craze
sign-craze --restart
```

---

## 10. Reapply (NDM hook)

**Когда срабатывает:** Keenetic NDM пересобирает iptables при:
- привязке/отвязке устройства к IP-policy через UI
- `save startup-config`
- reconnect WAN

После rebuild chain `signcraze_policy` теряется. NDM вызывает все исполняемые из `/opt/etc/ndm/netfilter.d/` с env `type` (iptables/ip6tables/ipset) и `table` (mangle/nat/filter).

**`50-sign-craze` реагирует только на:** `iptables/mangle`, `ip6tables/mangle`, `ipset/*` → вызывает `sign-craze --reapply`.

**`--reapply`:**
1. Non-blocking `flock` — занят → exit 0
2. PID-файл sing-box не существует → exit 0
3. `applier.Apply` (idempotent, без AttachTUN)
4. Все ошибки — Warn, exit 0 всегда (NDM event chain)

**Окно без защиты** (~100-500 мс на MIPS): пакеты в blackhole Keenetic. Утечки трафика нет.

**Проверка:**

```sh
ls -la /opt/etc/ndm/netfilter.d/50-sign-craze   # 0755
cat /opt/etc/ndm/netfilter.d/50-sign-craze
iptables -t mangle -L signcraze_policy -n
```

---

## 11. Uninstall

### 11.1 Удаление

```sh
sign-craze --uninstall
```

Удаляет: sing-box бинарь, init.d shim, NDM hook, конфиги (`/opt/etc/sign-craze/`), state/cache/geo/backups (`/opt/var/lib/sign-craze/`), PID-файлы, бинарь sign-craze, логи (`/opt/var/log/sign-craze/`). В режиме `policy` - удаляет IP-policy через RCI и `system configuration save`.

**Проверка:**

```sh
ls /opt/sbin/sing-box 2>/dev/null
ls /opt/sbin/sign-craze 2>/dev/null
iptables -t mangle -L signcraze_policy 2>&1   # No chain/target
```

---

## 12. Diag-bundle

```sh
sign-craze --diag
```

Выводит результат в формате `NAME [STATUS] DETAIL`:

```
binary-self          [PASS] /opt/sbin/sign-craze
binary-singbox       [PASS] /opt/sbin/sing-box
config-valid         [PASS] sing-box check ok
iptables             [PASS] доступен
ip6tables            [PASS] доступен
ipset                [PASS] доступен
default-route        [PASS] default via 192.168.1.1 dev eth0
service-singbox      [PASS] pid 1234
service-nfqws2       [WARN] не запущен
geo-files            [WARN] 3 файлов, последнее обновление 10h0m0s назад
lock-free            [PASS] свободна
keenetic-policy      [PASS] name=sign-craze mark=0xffffaab table=4098 permit=false
```

Возвращает ненулевой exit при наличии хотя бы одного `FAIL`.

Что проверяется:
- Бинари (`sign-craze`, `sing-box`)
- Валидность `config.json` (`sing-box check`)
- iptables, ip6tables, ipset
- Default-маршрут
- Процессы sing-box и nfqws2
- Возраст geo (WARN > 7 дней)
- Lock-файл
- IP-policy в Keenetic RCI (только в `policy`)

Сбор для support:

```sh
sign-craze --diag > /tmp/diag.txt 2>&1
cat /opt/var/log/sign-craze/sing-box.log >> /tmp/diag.txt
cat /opt/var/log/sign-craze/boot.log >> /tmp/diag.txt
```

---

## 13. Auto-update hostlist

Автоматическая загрузка и обновление hostlist для DPI-обхода по расписанию.

```sh
sign-craze --dpi-update-urls https://raw.githubusercontent.com/bol-van/zapret/master/ipset/zapret-hosts-user.txt.example,https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-youtube.txt,https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-discord.txt
sign-craze --dpi-update-interval 24
sign-craze --dpi-update-now
sign-craze --restart
```

| Флаг | Описание |
|------|----------|
| `--dpi-update-urls` | Список URL через запятую — источники hostlist |
| `--dpi-update-interval` | Интервал обновления в часах (24 = раз в сутки) |
| `--dpi-update-now` | Принудительно скачать hostlist немедленно |
| `--dpi-exclude-ips` | IP-адреса через запятую — RETURN перед [NFQUEUE](https://www.netfilter.org/projects/libnetfilter_queue/) |
| `--dpi-exclude-ips-list` | Показать текущие IP-исключения из DPI |

**Проверка:**

```sh
ls -la /opt/etc/sign-craze/dpi-hostlist.txt
sign-craze --dpi-targets-list
sign-craze --dpi-exclude-ips-list
```

---

## 14. VPN-exclude

Исключение IP VPN-эндпоинта из DPI/proxy-правил — трафик к VPN-серверу идёт напрямую.

**1. Найти IP VPN-эндпоинта:**

```sh
tcpdump -i eth3 host <vpn-server-domain>
# Смотреть на первый IP в SYN-пакетах к VPN-порту
```

**2. Добавить исключение:**

```sh
sign-craze --dpi-exclude-ips <vpn_ip>
# Несколько IP — через запятую: --dpi-exclude-ips 1.2.3.4,5.6.7.8
```

**3. Применить:**

```sh
sign-craze --restart
```

**Проверка:**

```sh
sign-craze --status
iptables -t mangle -L signcraze_policy_dpi -n -v | grep RETURN
```


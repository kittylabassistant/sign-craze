# OPERATIONS.md — Runbook оператора sign-craze

## 1. Обзор и предусловия

**sign-craze** — Go single-binary менеджер межсетевого экрана для Keenetic. Управляет sing-box (прокси-ядро), iptables/ipset, DPI-обходом (nfqws2) и интеграцией с Keenetic IP Policy через RCI.

**Предусловия:**
- Keenetic с установленным Entware (`/opt` смонтирован, ≥ 30 МБ свободно)
- SSH-доступ с root (или `sudo`)
- `opkg install ipset curl` — обязательно перед установкой

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
3. Применяет iptables/ipset (`applier.Apply`).
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
# версия:    sign-craze v0.x.x / sing-box v1.x.x
```

---

## 4. Переключение режимов

| Режим               | Механизм                                        | Когда использовать                              |
|---------------------|-------------------------------------------------|-------------------------------------------------|
| `policy` (default)  | Keenetic RCI IP Policy + fwmark из NDM          | Выборочная маршрутизация через Keenetic UI      |
| `full`              | ipset `signcraze_ipv4/v6` по dst-IP             | Нет доступа к RCI / альтернативная прошивка     |

Устаревшие псевдонимы `proxy`, `dpi`, `hybrid` автоматически мигрируют в `policy`.

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
ip rule show | grep 0x53     # fwmark-правило маршрутизации
```

---

## 5. Обновление

### 5.1 Обновить sign-craze (self-update)

```sh
sign-craze --update
```

Скачивает последний GitHub Release для текущей архитектуры, проверяет SHA256, атомарно заменяет `/opt/sbin/sign-craze`. Сервис **не перезапускается** автоматически — `--restart` после.

### 5.2 Обновить geo-файлы (SRS rule-set)

```sh
sign-craze --update-geo
```

Загружает `.srs` в `/opt/var/lib/sign-craze/geo/`, декомпилирует в CIDR, заполняет `signcraze_ipv4/ipv6`, сохраняет дамп в `/opt/share/sign-craze/ipset.dump`. Если sing-box ещё не установлен — заполнение ipset пропускается с предупреждением.

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

> Смена ядра не пересоздаёт конфиг автоматически — конфиг генерируется заново при следующем `--restart`.

---

## 6. Backup / Restore

### 6.1 Полный backup

```sh
sign-craze --backup
# /opt/var/lib/sign-craze/backups/backup-2026-05-03T12-00-00.tar.gz
```

Архивирует весь `/opt/etc/sign-craze/` (config.json, state.json, admin.creds). Geo-файлы и кэш не включаются — восстанавливаются через `--update-geo`.

> `admin.creds` — reserved-файл; basic auth не применяется с v0.5.2, но файл создаётся `LoadOrCreateCreds` для совместимости.

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

```sh
```

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

**Аутентификация:** отсутствует (auth удалён).

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

Удаляет: sing-box бинарь, init.d shim, NDM hook, конфиги (`/opt/etc/sign-craze/`), state/cache/geo/backups (`/opt/var/lib/sign-craze/`), PID-файлы, бинарь sign-craze, логи (`/opt/var/log/sign-craze/`). В режиме `policy` — удаляет IP-policy через RCI и `system configuration save`.

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

## 13. Deploy v0.8.x на Keenetic

> **BusyBox `wget` не поддерживает HTTPS на Entware.** Для загрузки обязательно нужен `/opt/bin/curl` (из пакета `curl`). Убедитесь, что пакет установлен: `opkg install curl`.

Выберите архитектуру:

| Модель | ARCH |
|--------|------|
| KN-1410, KN-1810 | `mips` |
| KN-1910, KN-2010 и новее | `mipsle` |
| Современные ARM-роутеры | `arm7` или `arm64` |

```bash
ARCH=mips  # подставить нужную архитектуру
VERSION=v0.8.3  # целевая версия
ssh -p 222 root@<router> "
  /opt/bin/curl -fsSL -o /tmp/sc.new https://github.com/kittylabassistant/sign-craze/releases/download/${VERSION}/sign-craze-${ARCH} &&
  /opt/bin/curl -fsSL -o /tmp/sc.sha https://github.com/kittylabassistant/sign-craze/releases/download/${VERSION}/sign-craze-${ARCH}.sha256 &&
  cd /tmp && sha256sum -c sc.sha &&
  mv /opt/sbin/sign-craze /opt/sbin/sign-craze.bak &&
  mv /tmp/sc.new /opt/sbin/sign-craze &&
  chmod +x /opt/sbin/sign-craze &&
  /opt/sbin/sign-craze --restart
"
```

**Проверка после deploy:**

```sh
sign-craze --version
# → v0.8.3

sign-craze --status
# → sing-box+nfqws2 запущены

iptables -t mangle -L POSTROUTING -n -v | grep signcraze_policy_dpi
# → -o eth3 (WAN-интерфейс)

iptables -t mangle -L signcraze_policy_dpi -n -v | head -5
# → первые правила RETURN

ls -la /opt/etc/sign-craze/dpi-hostlist.txt
# → файл существует

tail /opt/var/log/sign-craze/sign-craze.log | grep -E "reapply|dpi"
# → не более 12 reapply/час
```

---

## 14. Auto-update hostlist (24h)

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

**Проверка:**

```sh
ls -la /opt/etc/sign-craze/dpi-hostlist.txt
sign-craze --dpi-targets-list
```

---

## 15. VPN-exclude

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

---

## 16. Rollback v0.8.x

Если после deploy возникли проблемы — откат к предыдущему бинарю (`sign-craze.bak`):

```bash
ssh root@<router> "/opt/sbin/sign-craze --stop && mv /opt/sbin/sign-craze /tmp/sc.failed && mv /opt/sbin/sign-craze.bak /opt/sbin/sign-craze && /opt/sbin/sign-craze --start"
```

Проверить версию после отката:

```sh
sign-craze --version
sign-craze --status
```

> Если `.bak` отсутствует — восстановить через `--install-offline` или скачать нужную версию из GitHub Releases вручную (см. секцию [13. Deploy v0.8.x](#13-deploy-v08x-на-keenetic)).

---

## 17. Verify и migration после upgrade с v0.6.x

### Проверка inbound-конфига

```sh
netstat -tlnp | grep 7895
# Ожидаемо: sing-box LISTEN на TCP+UDP 0.0.0.0:7895
# Если пусто — возможна проблема с TUN-inbound
```

Если `netstat` не показывает порт — проверить наличие TUN-inbound в `routing.json`:

```sh
cat /opt/etc/sign-craze/routing.json | jq '.inbounds'
# Если вывод непустой и содержит type:"tun" — migration не сработала автоматически
```

**Ручное исправление:**

```sh
jq 'del(.inbounds)' /opt/etc/sign-craze/routing.json > /tmp/r && \
  mv /tmp/r /opt/etc/sign-craze/routing.json && \
  sign-craze --restart
```

После перезапуска повторно проверить `netstat -tlnp | grep 7895`.

### Проверка hostlist auto-update

```sh
ls -la /opt/etc/sign-craze/dpi-hostlist.txt
# Файл должен появиться через 24h после установки или после --dpi-update-now

wc -l /opt/etc/sign-craze/dpi-hostlist.txt
# Ожидаемо: десятки–сотни строк
```

### Проверка DNS fallback (v0.8.1+)

Если системный resolver возвращает SERVFAIL для github.com (например, DNSCrypt фильтрует upstream) — `--dpi-update-now` должен использовать fallback DNS и завершаться успешно:

```sh
sign-craze --dpi-update-now
# Ожидаемо: успешно скачан hostlist, обновлён счётчик строк
# При SERVFAIL на системном resolver — fallback DNS подхватывает автоматически
```

---

## 18. CLI auto-update hostlist

Управление автоматическим обновлением DPI-hostlist из внешних источников.

**Установить интервал (часы):**

```sh
sign-craze --dpi-update-interval 24
```

**Задать источники (URL через запятую):**

```sh
sign-craze --dpi-update-urls "https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-general.txt,https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-youtube.txt"
```

**Принудительное обновление немедленно:**

```sh
sign-craze --dpi-update-now
```

**VPN-исключения — RETURN перед NFQUEUE:**

```sh
sign-craze --dpi-exclude-ips "1.2.3.4,5.6.7.8"
sign-craze --restart
```

**Проверить текущий список исключений:**

```sh
sign-craze --dpi-exclude-ips-list
```

| Флаг | Описание |
|------|----------|
| `--dpi-update-interval` | Интервал авто-обновления в часах |
| `--dpi-update-urls` | Список URL через запятую — источники hostlist |
| `--dpi-update-now` | Скачать hostlist немедленно, не ждать таймера |
| `--dpi-exclude-ips` | IP-адреса через запятую — RETURN перед NFQUEUE |
| `--dpi-exclude-ips-list` | Показать текущие исключения |

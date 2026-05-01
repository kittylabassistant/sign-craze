# Инструкция проверки sign-craze на Keenetic KN-1810 (mipsle)

> **Архитектура v0.3+**: TUN-mode (не TPROXY). sing-box создаёт интерфейс `signbox-tun`,
> sign-craze управляет маршрутизацией через `ip rule fwmark → table → default dev signbox-tun`.
> Default `policy` mode — интеграция с Keenetic IP-policy (трафик помеченных
> устройств перехватывается через PolicyMark из RCI).

## 0. Предварительные требования

На роутере должен быть установлен **Entware** (OPKG). Проверка:

```sh
opkg --version
ls /opt/sbin
```

Если нет — установить через "Приложения" в веб-интерфейсе Keenetic.

Доступ по SSH:

```sh
ssh root@192.168.1.1
```

**Keenetic NDM rebuild iptables.** Keenetic пересобирает iptables при изменениях конфигурации (привязка устройства к IP-policy через web UI, save startup-config, reconnect WAN). После rebuild сторонние chain'ы из mangle теряются. Стандартный механизм восстановления — hook-скрипты в `/opt/etc/ndm/netfilter.d/`, NDM вызывает их после каждого rebuild с env `type=` и `table=`.

Sign-craze v0.3.x ставит свой hook автоматически при `--install`:

```sh
ls -la /opt/etc/ndm/netfilter.d/50-sign-craze   # должен быть, executable
```

Hook вызывает hidden CLI-команду `sign-craze --reapply`, которая идемпотентно переустанавливает mangle-чейн `signcraze_policy` (и ipset для full mode), не трогая sing-box и TUN — они переживают rebuild (NDM не управляет процессами и rtnetlink).

**Совместимость с XKeen** (если установлен на том же роутере): XKeen использует тот же hook-механизм, его hook ставится как `/opt/etc/ndm/netfilter.d/xkeen` (или похоже). Оба инструмента могут сосуществовать, поскольку реапплаят свои собственные chain'ы независимо. Однако если оба пытаются перехватить трафик одной и той же IP-policy — конфликт по mark. Базовый рекомендованный сценарий: одновременно запускать только sign-craze ИЛИ только XKeen.

## 1. Получить бинарь

### Вариант A — собрать локально и SCP

С хоста разработки:

```sh
cd ~/Документы/sign-craze
CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat \
  go build -ldflags="-s -w" -trimpath \
  -o /tmp/sign-craze-mipsle ./cmd/sign-craze
scp /tmp/sign-craze-mipsle root@192.168.1.1:/opt/sbin/sign-craze
```

### Вариант B — выпустить v0.2.0 и взять с GitHub

```sh
# с хоста
git tag v0.2.0 && git push origin v0.2.0
# подождать CI завершения
```

На роутере:

```sh
wget -O /opt/sbin/sign-craze \
  https://github.com/kittylabassistant/sign-craze/releases/download/v0.2.0/sign-craze-mipsle
chmod +x /opt/sbin/sign-craze
```

## 2. Smoke-тест — версия

```sh
sign-craze --version
```

Ожидается: `sign-craze v0.2.0 (...)` + `sing-box не установлен`.

```sh
sign-craze --help
```

Ожидается список ~28 команд, без `--service-start`.

## 3. Установка

### 3.1 Подготовка прокси

Иметь готовый URL прокси. Минимум для теста — любой публичный VLESS/socks5. Например:

```plain
socks5://USER:PASS@HOST:PORT
```

### 3.2 Запуск install

```sh
sign-craze --install
```

Wizard спросит:

- Выбор `[1/2/3]` — ввести `1` (URL)
- URL — вставить готовый

**Что должно произойти:**

- `/opt/var/lib/sign-craze/`, `/opt/etc/sign-craze/` созданы
- скачан `/opt/sbin/sing-box`
- сгенерирован `/opt/etc/sign-craze/config.json`
- сгенерирован `/opt/etc/sign-craze/state.json`
- создан `/opt/etc/init.d/S05signcraze`
- создан `/opt/etc/ndm/netfilter.d/50-sign-craze` (NDM hook для persistence)

Проверить:

```sh
ls -la /opt/sbin/sing-box /opt/etc/sign-craze/ /opt/etc/init.d/S05signcraze
ls -la /opt/etc/ndm/netfilter.d/50-sign-craze       # NDM hook executable
cat /opt/etc/ndm/netfilter.d/50-sign-craze          # должен вызывать --reapply
cat /opt/etc/sign-craze/state.json                  # Outbound видимый
sing-box version
sing-box check -c /opt/etc/sign-craze/config.json   # должен пройти
```

## 4. Запуск (TUN-mode, v0.3+)

Архитектура v0.3+:

- sing-box создаёт TUN-интерфейс `signbox-tun`.
- `policy` mode (default): Keenetic IP-policy маркит пакеты привязанных устройств своим mark (`0xffffaab` или похожим). Sign-craze в `mangle/PREROUTING` ремаркирует их в `0x53`. `ip rule fwmark 0x53 lookup 83` поднимает в таблицу 83 → `default dev signbox-tun` → sing-box → proxy.
- `full` mode (legacy): без интеграции с Keenetic, использует ipset `signcraze_ipv4`/`signcraze_ipv6` и собственные chains `signcraze`/`signcraze_dpi`/`signcraze_ports`.

```sh
sign-craze --start
```

Ожидается:

- `ndm: policy готова` с `mark=0x...` и `table4=...`.
- `firewall: применение правил mode=policy` (или `mode=full`).
- `firewall: TUN подключён dev=signbox-tun table=83`.
- `Сервис запущен (sing-box pid=NNN, режим=policy)`.

**Проверки на роутере (policy mode):**

```sh
# 1. Процесс жив, TUN поднят
ps | grep sing-box
cat /opt/var/run/sign-craze-singbox.pid
ip link show signbox-tun       # state UP, MTU 1500
ip addr show signbox-tun       # 172.19.0.1/30 + IPv6

# 2. ip rule + table 83 (loopMark sign-craze)
ip rule | grep 0x53            # ожидаем "from all fwmark 0x53 lookup 83"
ip route show table 83         # ожидаем "default dev signbox-tun"

# 3. Mangle chains (НЕ "signcraze" — старое имя; в policy mode это signcraze_policy)
iptables -t mangle -L signcraze_policy -n -v
iptables -t mangle -L PREROUTING -n -v | grep signcraze_policy
# с DPI: iptables -t mangle -L signcraze_policy_dpi -n -v

# 4. PolicyMark из state совпадает с mark в Keenetic IP-policy
cat /opt/etc/sign-craze/state.json | grep -i policy
# В roadmap §6 ниже — как сверить с Keenetic UI / RCI.
```

**Проверки full mode** (если переключали `--mode full`):

```sh
iptables -t mangle -L signcraze -n -v
iptables -t mangle -L signcraze_dpi -n -v
iptables -t mangle -L signcraze_ports -n -v
iptables -t mangle -L PREROUTING -n -v | grep signcraze
ipset list -n | grep signcraze
```

## 5. Status + Diag

```sh
sign-craze --status
```

Должно показать: sing-box запущен (pid X), nfqws2 остановлен, режим proxy.

```sh
sign-craze --diag
```

Желательно: всё PASS. WARN допустимы для `geo-files` (если ещё не качали).

## 6. Проверка трафика через прокси (policy mode)

Идея: устройство клиента (LAN) добавлено в Keenetic IP-policy `sign-craze`. Keenetic ставит mark на его пакеты → sign-craze ремаркит в `0x53` → пакет уходит в `signbox-tun` → sing-box → VPS.

### 6.1. Сверить PolicyMark

```sh
# Какой mark Keenetic присвоил policy "sign-craze":
ndmc -c "show ip policy"
# или через RCI:
curl -s 'http://localhost:79/rci/show/ip/policy' | head

# Сверить с state:
grep -i policymark /opt/etc/sign-craze/state.json
```

Mark в Keenetic должен совпадать с PolicyMark в state. Если не совпадает — `sign-craze --restart` (читает заново из RCI).

### 6.2. Привязка устройства

В Keenetic web UI: «Список устройств» → выбрать клиента → «Профиль доступа в интернет» → выставить `sign-craze`. Сохранить.

Проверить, что Keenetic реально маркит пакеты клиента (заменить `<CLIENT_IP>`):

```sh
# Счётчик попаданий в signcraze_policy должен расти при трафике клиента:
iptables -t mangle -L signcraze_policy -n -v
sleep 5
iptables -t mangle -L signcraze_policy -n -v
# pkts/bytes выросли → Keenetic mark работает, ремаркинг работает.
```

Если **pkts=0** даже при активном клиенте — Keenetic не ставит mark. Причины: устройство не в политике, политика без выходного канала, profile не сохранён. См. §6.4 диагностика.

### 6.3. Проверить смену IP

С клиентского устройства (телефон/ноут в LAN):

```sh
# До привязки к политике — IP провайдера:
curl -s ifconfig.me

# После привязки + reconnect WiFi (сбросить conntrack-сессии):
curl -s ifconfig.me   # должен показать IP VPS-сервера sing-box
```

Если IP не сменился — переходить к §6.4.

### 6.4. Диагностика «IP не меняется»

Идти сверху вниз — каждый шаг отсекает уровень:

```sh
# A. Keenetic ставит mark на пакеты клиента?
iptables -t mangle -L signcraze_policy -n -v
# pkts==0 → клиент не в policy / policy без mark.
#   - проверить web UI Keenetic: устройство привязано к "sign-craze".
#   - sign-craze --restart (перечитает PolicyMark).
#   - reconnect клиента (DHCP renew / WiFi off-on) — Keenetic привязывает по MAC.

# B. Ремаркинг 0x53 работает? (счётчик правил MARK):
iptables -t mangle -L signcraze_policy -n -v -x
# у правила -j MARK --set-xmark должен расти pkts при трафике клиента.

# C. ip rule + table подняты?
ip rule | grep 0x53                     # должно быть "fwmark 0x53 lookup 83"
ip route show table 83                  # "default dev signbox-tun"
# Если пусто — sign-craze --restart.

# D. Пакеты реально идут в TUN?
tcpdump -i signbox-tun -n -c 20 &
# с клиента: curl ifconfig.me
# должны появиться пакеты на signbox-tun. Если 0 — проблема выше (A/B/C).

# E. Sing-box принимает и форвардит?
tail -50 /opt/var/log/sign-craze/sing-box.log
tail -50 /opt/var/log/sign-craze/sing-box.stderr.log
# искать ошибки outbound/dial. На MIPS slow CPU connect к VPS может занимать 1–3s.

# F. Conntrack с устаревшим маршрутом
# Старые TCP-сессии клиента (открытые до --start) ходят без mark.
# Workaround: перезагрузить клиента или conntrack -F (если есть).
conntrack -F 2>/dev/null || echo "conntrack tool отсутствует — reconnect клиента"

# G. Loopback из самого роутера НЕ прокси-роутится в policy mode!
# curl ifconfig.me с самого Keenetic пойдёт прямо: трафик роутера НЕ привязан
# к policy. Тестировать ТОЛЬКО с LAN-клиента.
```

### 6.5. Geo-маршрутизация (опц.)

```sh
sign-craze --update-geo
ls /opt/var/lib/sign-craze/geo
```

В TUN-mode geo-категории применяются sing-box'ом внутри TUN: входящий трафик клиента уже в TUN, дальше sing-box по geosite/geoip разделяет на `direct`/`proxy` outbound. Конфиг: `route.rules` в `config.json`.

## 7. Web UI

```sh
sign-craze --ui on &
```

Запомнить пароль из stdout (выводится один раз).

С клиента в браузере: `http://192.168.1.1:9090` (Zashboard) или `:9091` (admin API).
Логин `admin`, пароль из stdout.

Останов:

```sh
kill %1   # bash job
```

## 8. Команды управления

```sh
# Порты
sign-craze --port-add 80
sign-craze --port-add 1000-1100
sign-craze --port-list
sign-craze --port-del 80

# Excludes
sign-craze --exclude-add 192.168.0.0/16
sign-craze --exclude-add 1.1.1.1
sign-craze --exclude-list
sign-craze --exclude-del 192.168.0.0/16

# Применить — нужен restart
sign-craze --restart
# проверить chain signcraze_ports заполнилась
iptables -t mangle -L signcraze_ports -n
ipset list signcraze_excludes
```

## 9. Mode switching

```sh
sign-craze --mode hybrid
sign-craze --restart
iptables -t mangle -L signcraze_dpi -n -v   # должны появиться NFQUEUE правила

sign-craze --mode proxy
sign-craze --restart
```

## 10. DPI (опционально)

```sh
sign-craze --dpi on
ls /opt/sbin/nfqws2
cat /opt/etc/sign-craze/nfqws2.conf
sign-craze --restart
ps | grep nfqws2
```

## 11. Stop

```sh
sign-craze --stop
ps | grep sing-box                                # пусто
ip link show signbox-tun 2>&1                     # "does not exist"
ip rule | grep 0x53                               # пусто
ip route show table 83                            # пусто
iptables -t mangle -L signcraze_policy -n 2>&1    # "No chain" (policy mode)
iptables -t mangle -L signcraze -n 2>&1           # "No chain" (full mode)
ipset list -n | grep signcraze                    # пусто (full mode)
```

## 12. Init.d автостарт

```sh
# Симуляция загрузки
/opt/etc/init.d/S05signcraze start
sleep 3
sign-craze --status                          # запущен

# Перезагрузить роутер
reboot
# подождать загрузки, SSH снова
ssh root@192.168.1.1
sign-craze --status                          # должен быть запущен
```

## 13. Backup/Restore

```sh
sign-craze --backup
ls /opt/var/lib/sign-craze/backups/
# скопировать архив на хост
scp root@192.168.1.1:/opt/var/lib/sign-craze/backups/backup-*.tar.gz ./

# восстановление
sign-craze --restore /opt/var/lib/sign-craze/backups/backup-2026-04-28T...tar.gz
```

## 14. Uninstall

```sh
sign-craze --uninstall
ls /opt/sbin/sing-box                                # отсутствует
ls /opt/etc/sign-craze/                              # отсутствует
ls /opt/etc/init.d/S05signcraze                      # отсутствует
ls /opt/etc/ndm/netfilter.d/50-sign-craze            # отсутствует
ls /opt/sbin/sign-craze                              # ОСТАЛСЯ

# Полная очистка
sign-craze --purge
ls /opt/sbin/sign-craze                              # отсутствует
```

## 15. Дополнительные команды установки

```sh
# Non-interactive (для CI / повторных тестов)
sign-craze --install-auto --url 'socks5://USER:PASS@HOST:PORT'

# Offline режим (бинарь sing-box уже лежит локально)
sign-craze --install-offline --singbox /tmp/sing-box

# Переустановка поверх (state.json должен сохраниться)
sign-craze --reinstall
cat /opt/etc/sign-craze/state.json   # outbound прежний
```

## 16. Self-update + sing-box update

```sh
# Обновление sign-craze бинаря
sign-craze --update
sign-craze --version   # должна быть новая версия

# Обновление sing-box
sing-box version          # текущая
sign-craze --update-core
sing-box version          # новая
sign-craze --restart
sign-craze --status       # запущен на новой версии
```

## 17. DPI strategies + update

```sh
# Список стратегий
sign-craze --dpi-strategy list

# Применить
sign-craze --dpi-strategy <name>
cat /opt/etc/sign-craze/nfqws2.conf   # обновился

# Обновление списков DPI
sign-craze --dpi-update
ls /opt/etc/sign-craze/dpi/    # списки доменов
```

## 18. Config backup/restore (отдельно от полного --backup)

```sh
# Только конфиг (state + sing-box config)
sign-craze --config-backup
ls /opt/var/lib/sign-craze/backups/config-*.tar.gz

# Откат только конфига (без geo/sing-box бинаря)
sign-craze --config-restore /opt/var/lib/sign-craze/backups/config-*.tar.gz
```

## 19. IPv6 проверки

В TUN-mode IPv6 идёт **внутри TUN** (sing-box получает v6-пакеты на `fdfe:dcba:9876::1/126` и форвардит). Policy mode для v6 требует `ip6tables` и `ip -6 rule`.

```sh
# TUN имеет IPv6
ip -6 addr show signbox-tun
# fdfe:dcba:9876::1/126 ожидается

# Если IPv6 у провайдера активен — должен быть mangle-хук v6:
ip6tables -t mangle -L signcraze_policy -n -v
ip6tables -t mangle -L PREROUTING -n -v | grep signcraze_policy

# IPv6 routing
ip -6 rule | grep 0x53
ip -6 route show table 83        # "default dev signbox-tun"

# Full mode
ip6tables -t mangle -L signcraze -n -v
ipset list signcraze_ipv6 2>/dev/null
ipset list signcraze_excludes_v6 2>/dev/null
```

**Если IPv6 disabled** в kernel sysctl (`/proc/sys/net/ipv6/conf/all/disable_ipv6=1`):

- sing-box не сможет назначить `fdfe:dcba:9876::1/126` на TUN → краш на старте.
- Workaround: убрать v6 адрес из state.json `TUNAddresses` (оставить только v4).

## 20. DNS перехват (если включён)

```sh
# Проверка правил для UDP/53
iptables -t nat -L signcraze_dns -n -v 2>/dev/null
iptables -t mangle -L signcraze -n -v | grep -E 'udp.*53|dpt:53'

# Локально на роутере — должен резолвить через sing-box
nslookup ya.ru 127.0.0.1
nslookup youtube.com 127.0.0.1
```

## 21. Concurrency / lock

```sh
# Параллельный запуск двух мутирующих команд
sign-craze --restart &
sign-craze --port-add 8080
wait

# Должна быть ошибка lock у одной из команд:
# "another sign-craze instance running" или подобное
ls /opt/var/run/sign-craze.lock
```

## 22. Логи

```sh
ls -lh /opt/var/log/sign-craze/
tail -50 /opt/var/log/sign-craze/sign-craze.log
tail -50 /opt/var/log/sign-craze/sing-box.log

# Проверить ротацию (если файлы > N MB → должен появиться .1.gz)
du -sh /opt/var/log/sign-craze/*
```

## 23. Zombie / краш sing-box

```sh
# Убить sing-box жёстко
kill -9 $(cat /opt/var/run/sign-craze-singbox.pid)

# --status должен показать stopped (не "running" из-за zombie)
sign-craze --status
# Ожидается: sing-box остановлен (regression check для коммита 629ac65)

# Восстановление
sign-craze --start
sign-craze --status   # running
```

## 24. WebSocket / UI live-update

```sh
sign-craze --ui on &
# в браузере открыть Zashboard → Connections / Logs
# отправить трафик через клиент
# счётчики должны обновляться live, без F5

# проверить reconnect: убить UI и поднять снова
kill %1
sign-craze --ui on &
# страница в браузере должна сама восстановить WS
```

## 25. State integrity при reboot во время записи

```sh
# Сценарий: запись state + reboot
sign-craze --port-add 9999 &
sleep 0.1
reboot
# после загрузки
ssh root@192.168.1.1
cat /opt/etc/sign-craze/state.json | jq .   # JSON валидный
sign-craze --status                          # запустился
```

## 26. Diag после stop

```sh
sign-craze --stop
sign-craze --diag
# Ожидается: WARN на sing-box/iptables (остановлено), не FAIL
```

---

## Что записывать в отчёт

| Шаг | Что отметить |
| --- | --- |
| 1 | размер бинаря до/после UPX (`ls -lh /opt/sbin/sign-craze`) |
| 4 | RAM: RSS sing-box + sing-craze + nfqws2 (`top \| grep -E 'sing-box\|sign-craze\|nfqws2'`) |
| 5 | вывод `--diag` целиком |
| 6 | смена IP при прокси-маршрутизации (есть/нет) |
| 7 | RSS sign-craze под нагрузкой UI (после WS-подписки) |
| 9 | работа hybrid режима |
| 12 | автостарт после reboot |
| 19 | IPv6 цепочки заполнены (если IPv6 у провайдера есть) |
| 21 | lock работает — параллельная команда падает с ошибкой |
| 23 | zombie sing-box → `--status` корректно показывает stopped |
| 25 | state.json не corrupt после reboot во время записи |

## Известные подводные камни на mipsel

- **Размер бинаря:** без UPX ~9MB. С UPX будет ~3MB. Если ставишь свежесобранный — без UPX. Если из релиза — с UPX.
- **GOMIPS=softfloat обязателен** — KN-1810 не имеет FPU, hardfloat будет SIGILL.
- **Лимит RAM 256MB на KN-1810:** sing-box обычно <30MB RSS. Если выше — подозрительно.
- **TUN-модуль обязателен** — `ls -l /dev/net/tun` должен показать char device. На стоковой Keenetic-прошивке TUN включён (используется встроенным OpenVPN/WireGuard).
- **xt_comment НЕ требуется** — sign-craze v0.3+ не использует `-m comment` (busybox iptables его не имеет).
- **xt_TPROXY НЕ требуется** — sign-craze v0.3+ перешёл на TUN-mode (не TPROXY).
- **iptables match `set` нужен только для `full` mode**: `opkg install iptables-mod-ipset` если планируется legacy режим.
- **ipset нужен только для `full` mode**: `opkg install ipset`.
- **NFQUEUE для DPI**: `opkg install kmod-nfnetlink-queue iptables-mod-nfqueue`.
- **iptables backend**: проверь `iptables --version` — Keenetic обычно `iptables-legacy`. Если `nf_tables` — поведение MARK/CONNMARK может отличаться.
- **fwmark `0x53` единственный**: `ip rule | grep 0x53` — других правил с этой меткой быть не должно (коллизия с другим софтом → silent breakage).
- **Loopback с самого роутера НЕ проксируется** в `policy` mode. Keenetic ставит mark только на пакеты LAN-устройств, привязанных к политике. `curl ifconfig.me` с самого Keenetic SSH пойдёт прямо. Тестировать ТОЛЬКО с LAN-клиента.
- **Conntrack помнит старые сессии** — после `--start` уже открытые соединения клиента продолжат идти прямо (без mark). Reconnect клиента (Wi-Fi off/on) или `conntrack -F` сбросит.
- **DNS-server `local`** в config.json — sing-box 1.13 использует тип `local` (системный resolver), не `udp` с `detour: direct` (запрещено в 1.13).
- **NDM rebuilds iptables** — Keenetic пересобирает iptables при привязке устройства к policy / save startup-config / WAN reconnect, теряя сторонние mangle-чейны. Решение — netfilter.d hook (`/opt/etc/ndm/netfilter.d/50-sign-craze`), который sign-craze ставит автоматически при `--install`. Hook вызывает `sign-craze --reapply`. Если hook отсутствует или не executable — `signcraze_policy` будет пропадать после первого NDM-event'а.
- **Лог-файл пуст при интерактивном запуске** — `/opt/var/log/sign-craze/sign-craze.log` пишется только когда `stderr` не терминал (init.d/cron). При запуске из ssh-shell лог идёт в stderr (`internal/log/log.go:42`). Для диагностики смотреть `tail` на live-выводе команды или запустить через `sign-craze --start 2>&1 | tee /tmp/run.log`.

Если что-то не работает — пришли вывод `sign-craze --diag`, `sign-craze --status`, `tail -50 /opt/var/log/sign-craze/sing-box.log` и `tail -50 /opt/var/log/sign-craze/sing-box.stderr.log`.

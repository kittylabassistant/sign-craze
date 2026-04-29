# Инструкция проверки sign-craze на Keenetic KN-1810 (mipsle)

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

Проверить:

```sh
ls -la /opt/sbin/sing-box /opt/etc/sign-craze/ /opt/etc/init.d/S05signcraze
cat /opt/etc/sign-craze/state.json   # Outbound видимый
sing-box version
sing-box check -c /opt/etc/sign-craze/config.json   # должен пройти
```

## 4. Запуск

```sh
sign-craze --start
```

Ожидается: `Сервис запущен (sing-box pid=NNN, режим=proxy)`.

**Проверки на роутере:**

```sh
# процесс жив
ps | grep sing-box

# PID-файлы
cat /opt/var/run/sign-craze-singbox.pid

# iptables-цепочки созданы
iptables -t mangle -L signcraze -n -v
iptables -t mangle -L signcraze_dpi -n -v
iptables -t mangle -L signcraze_ports -n -v
iptables -t mangle -L PREROUTING -n -v | grep signcraze

# ipset
ipset list -n | grep signcraze

# ip rule
ip rule | grep 0x53
ip route show table 83
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

## 6. Проверка трафика через прокси

### С самого роутера

```sh
# IP без прокси (прямое подключение)
curl -s ifconfig.me

# IP через ipset (добавить тестовый IP в signcraze_ipv4 — туда трафик пойдёт через прокси)
ipset add signcraze_ipv4 1.1.1.1
curl --resolve ifconfig.me:443:1.1.1.1 https://ifconfig.me
# (это спорный тест — лучше с клиентского устройства)
```

### С клиентского устройства

На устройстве в LAN роутера:

```sh
# исходный IP (прямой)
curl ifconfig.me
```

На роутере добавить geo-файлы:

```sh
sign-craze --update-geo
ls /opt/var/lib/sign-craze/geo
```

После добавления нужных категорий в outbounds (через `--ui` или прямо в `state.json`) → проверять что для проксированных доменов внешний IP меняется на прокси-сервер.

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
ps | grep sing-box                          # пусто
iptables -t mangle -L signcraze -n -v 2>&1  # No chain
ipset list -n | grep signcraze              # пусто
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
ls /opt/sbin/sing-box                        # отсутствует
ls /opt/etc/sign-craze/                      # отсутствует
ls /opt/sbin/sign-craze                      # ОСТАЛСЯ

# Полная очистка
sign-craze --purge
ls /opt/sbin/sign-craze                      # отсутствует
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

```sh
# IPv6 цепочки
ip6tables -t mangle -L signcraze -n -v
ip6tables -t mangle -L signcraze_dpi -n -v
ip6tables -t mangle -L PREROUTING -n -v | grep signcraze

# IPv6 ipset
ipset list signcraze_ipv6
ipset list signcraze_excludes_v6

# IPv6 routing
ip -6 rule | grep 0x53
ip -6 route show table 83
```

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
- **iptables в Keenetic** может быть только базовый, без `multiport` модуля. Проверь:

  ```sh
  iptables -m multiport --help 2>&1 | head
  iptables -m comment --help 2>&1 | head
  ```

  Если модуль отсутствует — нужно `opkg install iptables-mod-extra` (или аналог).
- **TPROXY** аналогично: `opkg install kmod-ipt-tproxy iptables-mod-tproxy`.
- **ipset** должен быть: `opkg install ipset`.
- **NFQUEUE** для DPI: `opkg install kmod-nfnetlink-queue iptables-mod-nfqueue`.
- **iptables backend**: проверь `iptables --version` — Keenetic обычно `iptables-legacy`. Если `nf_tables` — поведение MARK/CONNMARK может отличаться.
- **fwmark `0x53` единственный**: `ip rule | grep 0x53` — других правил с этой меткой быть не должно (коллизия с другим софтом → silent breakage).

Если что-то не работает — пришли вывод `sign-craze --diag` и вывод проблемной команды.

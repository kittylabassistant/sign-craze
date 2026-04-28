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

---

## Что записывать в отчёт

| Шаг | Что отметить |
| --- | --- |
| 1 | размер бинаря (`ls -lh /opt/sbin/sign-craze`) |
| 4 | RAM использование sing-box (`top \| grep sing-box`) |
| 5 | вывод `--diag` целиком |
| 6 | смена IP при прокси-маршрутизации (есть/нет) |
| 9 | работа hybrid режима |
| 12 | автостарт после reboot |

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

Если что-то не работает — пришли вывод `sign-craze --diag` и вывод проблемной команды.

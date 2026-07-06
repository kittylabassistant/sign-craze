# Установка sign-craze

Полная инструкция от чистой флешки до рабочего прокси на роутере [Keenetic](https://help.keenetic.com/hc/ru).

> [!WARNING]
> Все команды выполняются на ваш страх и риск. Перед форматированием убедитесь, что выбран правильный диск — ошибка может уничтожить данные на системном накопителе. Делайте резервную копию важных данных.

## Содержание

1. [Что понадобится](#1-что-понадобится)
2. [Подготовка флешки: разметка + swap](#2-подготовка-флешки-разметка--swap)
3. [Установка Entware на Keenetic](#3-установка-entware-на-keenetic)
4. [Создание swap 1 GB](#4-создание-swap-1-gb)
5. [Установка sign-craze](#5-установка-sign-craze)
6. [Конфигурация sign-craze](#6-конфигурация-sign-craze) (включая выбор ядра `--core`)
7. [Запуск](#7-запуск)
8. [Web UI (опционально)](#8-web-ui-опционально)
9. [Гео-фильтрация (опционально)](#9-гео-фильтрация-опционально)
10. [Проверка прокси с клиента](#10-проверка-прокси-с-клиента)
11. [Автозапуск после ребута](#11-автозапуск-после-ребута)
12. [Диагностика при проблемах](#12-диагностика-при-проблемах)

---

## 1. Что понадобится

- Роутер Keenetic с USB-портом и поддержкой OPKG (большинство моделей кроме самых младших).
- USB-флешка **≥ 4 GB** (sign-craze + [sing-box](https://sing-box.sagernet.org/) + geo-файлы + swap-файл 1 GB + запас под [Entware](https://entware.net/)).
- ПК с Linux, macOS или Windows.
- Подписка на VPN/прокси-сервис: URL формата `vless://...`, `vmess://...`, `ss://...`, `trojan://...`, `http://...` или `socks5://...`.
- SSH-клиент (встроен в Linux/macOS, для Windows — PuTTY или встроенный `ssh.exe`).
- Пакеты `iptables` и `ipset` в Entware (`opkg install iptables ipset`) — нужны для firewall-правил sign-craze; доустановите их вручную перед `sign-craze --start`.

---

## 2. Подготовка флешки: разметка + swap

Цель: получить флешку с одним ext4-разделом (и опционально swap-разделом). Если хотите swap-файлом вместо раздела — оставьте всю флешку под ext4 и переходите к [шагу 4](#4-создание-swap-1-gb).

Выберите **один из 6 методов** в зависимости от вашей ОС.

### Метод A — Linux CLI (bash/parted)

```bash
# 1. Найти устройство флешки
lsblk

# Вы увидите примерно:
#  sda      8:0    1   16G  0 disk
#  └─sda1   8:1    1   16G  0 part
# Запомните имя без цифры (sda, sdb, sdc...). НЕ перепутайте с системным диском!

# 2. Размонтировать все разделы флешки
sudo umount /dev/sdX*    # X = ваша буква, e.g. /dev/sdb*

# 3. Очистить старые сигнатуры
sudo wipefs -a /dev/sdX

# 4. Вариант 1 — только ext4 (swap будет файлом, см. шаг 4):
sudo parted /dev/sdX --script mklabel msdos
sudo parted /dev/sdX --script mkpart primary ext4 1MiB 100%
sudo mkfs.ext4 -L Entware /dev/sdX1

# 4. Вариант 2 — ext4 + отдельный swap-раздел 1 GB:
sudo parted /dev/sdX --script mklabel msdos
sudo parted /dev/sdX --script mkpart primary ext4 1MiB -1024MiB
sudo parted /dev/sdX --script mkpart primary linux-swap -1024MiB 100%
sudo mkfs.ext4 -L Entware /dev/sdX1
sudo mkswap -L SWAP /dev/sdX2

# 5. Извлечь
sudo eject /dev/sdX
```

### Метод B — macOS Terminal (через Homebrew)

macOS не поддерживает ext4 нативно. Установите `e2fsprogs`:

```bash
# 1. Поставить e2fsprogs
brew install e2fsprogs

# 2. Найти диск
diskutil list

# Найдите вашу флешку (e.g. /dev/disk4). Будьте ОЧЕНЬ внимательны — внутренний SSD не трогать!

# 3. Размонтировать
diskutil unmountDisk /dev/diskN     # N = номер вашей флешки

# 4. Стереть и создать MBR с одним пустым разделом
diskutil eraseDisk free FREE MBR /dev/diskN
diskutil partitionDisk /dev/diskN 1 MBR "Free Space" "FREE" 100%

# 5. Отформатировать в ext4
sudo $(brew --prefix e2fsprogs)/sbin/mkfs.ext4 -L Entware /dev/rdiskNs1

# 6. Извлечь
diskutil eject /dev/diskN
```

> [!NOTE]
> После форматирования диск в Finder не появится — это нормально, macOS не умеет монтировать ext4 без сторонних драйверов. Swap делайте swap-файлом на роутере (шаг 4).

### Метод C — Windows PowerShell + diskpart

PowerShell сам ext4 не умеет. Сначала очистите флешку через `diskpart`, затем отформатируйте через GUI-утилиту (методы D/E/F).

```powershell
# Запустить PowerShell от имени администратора
diskpart
```

В интерактивной сессии diskpart:

```
list disk
select disk N            REM N — номер флешки. ОСТОРОЖНО!
clean
create partition primary
exit
```

Далее перейдите к одному из GUI-методов (D, E или F) для форматирования в ext4.

### Метод D — Paragon Partition Manager Free (Windows)

1. Скачать с https://www.paragon-software.com/free/pm-express/ (Free версия).
2. Установить и запустить от администратора.
3. В списке дисков выбрать USB-флешку → правой кнопкой по разделу → **Format Partition**.
4. **File system:** `Ext4`
5. **Volume label:** `Entware`
6. Нажать **Format** → затем **Apply Changes** в верхней панели.
7. (Опционально) Создать второй раздел 1024 MB с типом **Linux Swap**.

### Метод E — MiniTool Partition Wizard Free (Windows)

1. Скачать с https://www.partitionwizard.com/free-partition-manager.html (Free).
2. Установить и запустить от администратора.
3. Выбрать USB-флешку → правой кнопкой по разделу → **Format Partition**.
4. **File System:** `Ext4`, **Partition Label:** `Entware`.
5. Нажать **OK** → затем **Apply** в нижнем левом углу.
6. (Опционально) Свободное место → **Create** → **File System:** `Linux Swap`, **Size:** `1024 MB` → **Apply**.

### Метод F — GParted Live USB (универсальный, Windows/macOS/Linux)

Загрузочный диск с GParted работает на любом ПК.

1. Скачать ISO: https://gparted.org/download.php
2. Записать ISO на **отдельную** загрузочную флешку:
   - Windows: [Rufus](https://rufus.ie/)
   - macOS/Linux: [balenaEtcher](https://www.balena.io/etcher/) или `dd`
3. Загрузиться с этой флешки (BIOS/UEFI Boot Menu, обычно `F12` / `F2` / `Esc` при включении).
4. В GParted Live выбрать целевую флешку (правый верхний угол: **Device**).
5. Создать таблицу разделов: **Device** → **Create Partition Table** → `msdos`.
6. Создать ext4-раздел: правой кнопкой на свободном месте → **New** → **File system:** `ext4`, **Label:** `Entware`, размер = всё минус 1024 MB.
7. Создать swap: правой кнопкой на оставшемся свободном месте → **New** → **File system:** `linux-swap`, размер `1024 MB`.
8. **Edit** → **Apply All Operations** → подтвердить.
9. Извлечь и подключить к роутеру.

---

## 3. Установка Entware на Keenetic

### 3.1. Подготовка веб-интерфейса

1. Открыть веб-интерфейс роутера (`http://192.168.1.1` или `http://my.keenetic.net`).
2. **Общие настройки** → **Обновления и компоненты** → **Изменить набор компонентов**.
3. Убедиться, что установлены:
   - **OPKG** (менеджер пакетов)
   - **Файловая система Ext4**
   - **Файл подкачки** (для swap)
4. Применить, дождаться перезагрузки.

### 3.2. Подключение флешки

1. Подключить отформатированную флешку к USB-порту роутера.
2. Веб-интерфейс → **Приложения** → **USB-накопители**.
3. Дождаться появления флешки в списке.

### 3.3. Установка Entware через веб-интерфейс

1. Веб-интерфейс → **Приложения** → раздел **OPKG**.
2. **Включить** OPKG.
3. **Накопитель:** выбрать раздел флешки (`Entware` если ставили label).
4. Применить. Роутер скачает и установит Entware (~3–5 минут).

### 3.4. Включение SSH

1. Веб-интерфейс → **Управление** → **Пользователи и доступ**.
2. Создать пользователя или выбрать существующего → дать права **admin**.
3. Разрешить доступ по **SSH** для этого пользователя.

### 3.5. Проверка по SSH

```sh
ssh admin@192.168.1.1     # либо адрес вашего роутера

# Внутри сессии — попасть в Entware shell:
opkg
# или сразу:
exec sh
opkg update
opkg install nano htop
```

> [!NOTE]
> На некоторых прошивках Keenetic после `ssh admin@...` вы попадаете в CLI Keenetic, а не в Entware. Команда `exec sh` переключает в shell Entware. Альтернатива — `ssh root@...` если включён рутовый доступ.

---

## 4. Создание swap 1 GB

Если в шаге 2 вы не создавали отдельный swap-раздел — используйте swap-файл (рекомендуется, проще).

### 4.1. Swap-файл (рекомендуется)

```sh
# По SSH на роутере, в Entware shell

# Создать файл 1 GB
dd if=/dev/zero of=/opt/swap bs=1M count=1024
chmod 600 /opt/swap
mkswap /opt/swap
swapon /opt/swap

# Проверка
free -m
swapon -s
```

### 4.2. Автозапуск swap после ребута

Создать init.d-скрипт `/opt/etc/init.d/S02swap`:

```sh
cat > /opt/etc/init.d/S02swap <<'EOF'
#!/bin/sh
ENABLED=yes
PROCS=swapon
ARGS="/opt/swap"
PREARGS=""
DESC=$PROCS
PATH=/opt/sbin:/opt/bin:/sbin:/bin:/usr/sbin:/usr/bin

. /opt/etc/init.d/rc.func
EOF
chmod +x /opt/etc/init.d/S02swap
```

### 4.3. Если swap-раздел был создан в шаге 2

```sh
# Найти swap-раздел
blkid | grep swap
# Например: /dev/sda2: LABEL="SWAP" TYPE="swap"

swapon /dev/sda2
free -m
```

Автозапуск аналогично — `S02swap` с `ARGS="/dev/sda2"`.

---

## 5. Установка sign-craze

### 5.1. Способ 1: curl | sh (рекомендуется)

> **Важно**: [BusyBox](https://busybox.net/) `wget` на Keenetic собран **без SSL** и не поддерживает HTTPS — выдаст `not an http or ftp url`. Также BusyBox `od` не поддерживает опцию `-t`. Поэтому установка идёт через `curl` (с `-k` — без проверки сертификатов) или через `wget-ssl` из Entware.

#### 5.1.1. Установить curl (один раз)

```sh
opkg update
opkg install curl
```

Альтернатива — `wget-ssl`:

```sh
opkg install wget-ssl
```

#### 5.1.2. Запустить установщик

```sh
# По SSH, в Entware shell на роутере
curl -fsSL https://github.com/kittylabassistant/sign-craze/releases/latest/download/install.sh | sh
```

Если GitHub недоступен напрямую, передайте прокси через переменную окружения установщику:

```sh
https_proxy=http://<host>:<port> curl -fsSL https://github.com/kittylabassistant/sign-craze/releases/latest/download/install.sh | sh
```

Скрипт автоматически:

- Выберет загрузчик: `curl -fsSL` или `wget` (с проверкой TLS). Если корневых CA на роутере нет — `opkg install ca-certificates`.
- Определит архитектуру (`mipsle` / `mips` / `arm7` / `arm64`); endianness MIPS — через `od -x` (BusyBox-совместимо).
- Проверит свободное место на `/opt` (нужно ≥ 30 МБ).
- Скачает соответствующий бинарь с GitHub Releases.
- Проверит SHA256 (если файл `.sha256` доступен).
- Атомарно установит в `/opt/sbin/sign-craze`.

#### 5.1.3. Возможные ошибки

| Ошибка | Причина | Решение |
|--------|---------|---------|
| `wget: not an http or ftp url: https://...` | BusyBox `wget` без SSL | `opkg install curl` или `wget-ssl` |
| `od: invalid option -- 't'` | Старая версия скрипта (BusyBox `od` без `-t`) | Обновить `install.sh` до последнего релиза |
| `od: invalid option -- 'A'` | Старая версия скрипта (BusyBox `od` без `-A`/`-N`) | Обновить `install.sh` до последнего релиза |
| `Не удалось определить endianness MIPS` | `od` вывел неожиданный формат | Сообщить вывод `printf '\\001\\002' \| od -x` в issue |
| `Нужен curl или wget с поддержкой SSL` | Ни `curl`, ни `wget-ssl` не установлены | `opkg install curl` |
| `Недостаточно места в /opt` | < 30 МБ свободно | Очистить `/opt`, либо подключить swap/USB |

Проверка:

```sh
sign-craze --version
```

Ожидаемый вывод:

```
sign-craze v1.6.3 (commit <актуальный>, built 2026-06-30)
sing-box: not installed
```

---

## 6. Конфигурация sign-craze

### 6.1. Выбор прокси-ядра (`--core`)

sign-craze поддерживает три прокси-ядра. По умолчанию используется **sing-box**. Выбрать ядро можно при установке или позже:

```sh
# Установка с явным указанием ядра
sign-craze --install --core sing-box   # по умолчанию, TPROXY-mode (default с v0.7.1; TUN — только при явном --inbound tun)
sign-craze --install --core xray       # PQ-VLESS, Vision, xhttp packet-up
sign-craze --install --core mihomo     # Hysteria2, TUIC, WireGuard

# Смена ядра в работающей системе
sign-craze --core xray --restart
sign-craze --core sing-box --restart

# Список ядер и статус установки
sign-craze --core-list
```

Если нужное ядро ещё не скачано — установить без переконфигурации:

```sh
sign-craze --core-install xray     # скачать xray в /opt/sbin/xray, не переключать
sign-craze --core-install mihomo   # скачать mihomo в /opt/sbin/mihomo
```

Ядро скачивается с GitHub Releases через тот же mirror chain, что и sign-craze (Fastly raw.githubusercontent.com как приоритетный канал). SHA256 проверяется автоматически.

### 6.2. Supervised peers: naiveproxy и mieru (v1.3.0+)

sign-craze поддерживает [naiveproxy](https://github.com/klzgrad/naiveproxy) и [mieru](https://github.com/enfein/mieru) как supervised peers — запускаются как daemon рядом с sing-box (process chain через socks5-outbound). Работают **только** с ядром sing-box; xray и mihomo такие конфиги отклоняют с подсказкой `--core sing-box`.

```sh
# Установка + активация naiveproxy
sign-craze --install --with-naive --proxy 'naive+https://user:pass@host:443'

# Обновить бинарь naiveproxy
sign-craze --update-naive
```

Поддерживаемые URL-схемы: `naive+https://user:pass@host:port`, `naive+quic://...`, `mieru://...`, `mierus://...`.

> [!NOTE]
> naiveproxy доступен только для **arm64, arm7, mipsle**. Роутеры big-endian MIPS (`mips`) не поддерживаются — klzgrad публикует только LE-сборки.

### 6.3. Интерактивная установка

```sh
sign-craze --install
```

Утилита спросит:

1. **URL прокси / outbound** — вставьте ваш `vless://...` / `vmess://...` / `ss://...` / `trojan://...` / `http://...` / `socks5://...`.
2. **Режим маршрутизации:**
   - `policy` — интеграция с Keenetic IP Policy через RCI (по умолчанию). Выбор устройств — через штатный web-UI Keenetic «Приоритеты подключений».
   - `full` — legacy: собственный fwmark 0x53 + ipset, маршрутизирует **весь** транзитный трафик через TUN.

Можно передать outbound сразу через флаг `--proxy`, минуя интерактивный режим:

```sh
sign-craze --install --proxy 'vless://...'
# или при переустановке:
sign-craze --reinstall --proxy 'vless://...'
sign-craze --restart
```

Флаг `--proxy` принимает тот же URL-формат, что и при интерактивной установке. При `--reinstall --proxy` завершение подсказывает `--restart` (не `--start`), так как ядро продолжает работать.

Флаг `--preset <name>` (v1.5.0+) применяет routing-пресет прямо при установке (режим replace):

```sh
sign-craze --install --proxy 'vless://...' --preset ru-direct
```

Доступные пресеты (8 шт.): `sign-craze-default`, `block-ads`, `ru-direct`, `ru-direct-rest-vpn`, `blocked-vpn`, `discord-vpn`, `torrents-direct`, `block-bogon-udp`. Посмотреть список: `sign-craze --preset-list`. Без `--preset` routing настраивается позже через Web UI на порту 9092.

После завершения:

- Скачается `sing-box` с GitHub Releases SagerNet → `/opt/sbin/sing-box`
- Сгенерируется `/opt/etc/sign-craze/config.json` (TUN inbound `signbox-tun`, fwmark `0x53` для loop-prevention)
- В режиме `policy` — будет создана IP Policy `sign-craze` через RCI Keenetic
- При включении DPI (`--dpi on`) — скачается `nfqws2` и сгенерируется `/opt/etc/sign-craze/nfqws2.conf`; DPI chain перенесена в FORWARD (signcraze_dpi_fwd) с v1.1.0 для применения ко всем LAN-устройствам
- Создастся init.d shim `/opt/etc/init.d/S99signcraze` (автозапуск sing-box + firewall watchdog)
- Создастся NDM netfilter.d hook `/opt/etc/ndm/netfilter.d/50-sign-craze` (реапплай правил после rebuild)

> **Защита от SSH-lockout и LAN-трафик**: SSH-порты роутера (`22` Entware/dropbear, `222` Keenetic admin) и локальные сети (RFC1918 + loopback + multicast + link-local) автоматически исключены из проксирования через [ipset](https://ipset.netfilter.org/) `signcraze_excludes` и RETURN-правила в mangle. Дополнительной настройки не требуется. Для кастомизации (другие порты, bypass публичных IP) править поля `admin_ports` и `admin_ips` в `/opt/etc/sign-craze/state.json` и перезапустить sign-craze.

### 6.4. Без вопросов

```sh
sign-craze --install-auto
```

Использует параметры по умолчанию: режим `policy`, outbound — заглушка `direct` (для реального outbound передай `--proxy <URL>` отдельной командой).

### 6.5. Из локального бинаря (offline)

```sh
sign-craze --install-offline /tmp/sing-box-1.10.0-linux-mipsle.tar.gz
```

### 6.6. С DPI-обходом из коробки (`--with-dpi`)

```sh
sign-craze --install --with-dpi
sign-craze --start
```

Что произойдёт:

1. Стандартный `--install` (sing-box, init.d shim, netfilter.d hook).
2. Скачивается `nfqws2-keenetic` v1.2.3 (`.ipk` для текущей arch) с GitHub.
3. Распаковывается бинарь `nfqws2` → `/opt/sbin/nfqws2`.
4. Распаковываются blob-payload'ы (`quic_initial.bin`, `tls_clienthello.bin`)
   → `/opt/etc/sign-craze/blobs/`.
5. `state.DPIEnabled=true`, `state.DPITargets` = preset `discord-youtube`
   (17 доменов: discord.com, discord.gg, …, youtube.com, googlevideo.com, …).
6. Генерируется `/opt/etc/sign-craze/nfqws2.conf` со стратегиями upstream
   (TLS+QUIC для YouTube, UDP-stun/voice для Discord).
7. После `--start` nfqws2 поднимается со связкой:

   ```
   nfqws2 --user=nobody --qnum=300 --lua-init --blob-dir=…
          <UDP discord/stun стратегия> --new
          <QUIC YouTube стратегия> --new
          <TLS YouTube/general> --hostlist=/opt/etc/sign-craze/dpi-hostlist.txt
   ```

Для другого набора доменов: `sign-craze --dpi-targets <список>` (через запятую).
Для отключения: `sign-craze --dpi off && sign-craze --restart`.

Источник стратегий: github.com/nfqws/nfqws2-keenetic (MIT).
Все стратегии встроены в sign-craze и могут быть переопределены через
`--dpi-strategy "<NFQWS_ARGS строка>"` (override TCP/TLS блока).

---

## 7. Запуск

```sh
sign-craze --start
```

Применит [iptables](https://www.netfilter.org/projects/iptables/index.html)/ipset правила и запустит sing-box (и [nfqws2](https://github.com/nfqws/nfqws2-keenetic) в режимах `dpi`/`hybrid`).

> **DPI отключён по умолчанию.** После `sign-craze --install` (без `--with-dpi`) режим — `policy`, `nfqws2` не скачан и не запускается, NFQUEUE-правила не добавляются. Для активации DPI:
>
> - **Из коробки**: `sign-craze --install --with-dpi` (см. [6.6](#66-с-dpi-обходом-из-коробки---with-dpi)).
> - **На существующей установке**: `sign-craze --dpi on && sign-craze --restart` (см. раздел [9a](#9a-selective-dpi-bypass-опционально)).

Проверка статуса:

```sh
sign-craze --status
```

Ожидаемый вывод:

```
sing-box: running (PID 12345)
nfqws2: running (PID 12346)
режим: policy
версия: v1.6.3
```

---

## 8. Web UI (опционально)

```sh
sign-craze --ui on
```

Откроются три HTTP-сервиса (только из LAN):

- `http://<router-ip>:9090/` — **[Zashboard](https://github.com/Zephyruso/zashboard)** (Clash-совместимый dashboard). Показывает реальное дерево прокси, живые счётчики трафика, активные соединения и логи в реальном времени — стриминговые эндпоинты (`/traffic`, `/logs`, `/connections`) передаются без буферизации. Данные приходят через Clash API реверс-прокси на sing-box (порт `9094`, внутренний). Выбор активного прокси сохраняется после рестарта sing-box.
- `http://<router-ip>:9091/api/status` — **admin REST API** (статус, конфиг, порты, исключения, DPI targets).
- `http://<router-ip>:9092` — **Routing Editor SPA** (визуальный редактор inbounds/outbounds/rules, пресеты). При первом запуске автоматически инициализируется из текущих `state.outbounds` — сконфигурированный прокси появляется сразу. Outbound в таблице отображается с адресом сервера и портом.

Все порты работают без аутентификации. Доступ извне LAN заблокирован iptables-правилами.

---

## 9. Гео-фильтрация (опционально)

```sh
sign-craze --update-geo
```

Скачивает [SRS rule-set](https://sing-box.sagernet.org/configuration/rule-set/) файлы с GitHub (manifest-driven, выборочная загрузка по SHA256). После обновления:

```sh
sign-craze --restart
```

---

## 9a. Selective DPI bypass (опционально)

По умолчанию `--dpi on` запускает nfqws2 на **весь** TCP/UDP трафик (через [NFQUEUE](https://www.netfilter.org/projects/libnetfilter_queue/)). На MIPS-роутерах это даёт packet-copy overhead до ~30% CPU при 40 Mbps. Selective режим ограничивает desync только выбранными доменами через `nfqws2 --hostlist`.

```sh
# Включить DPI
sign-craze --dpi on

# Установить список целевых доменов (Discord + YouTube)
sign-craze --dpi-targets discord.com,discordapp.com,youtube.com,googlevideo.com,ytimg.com

# Применить
sign-craze --restart

# Проверить активный список
sign-craze --dpi-targets-list

# Сбросить (вернуться к "desync для всего")
sign-craze --dpi-targets clear
```

Через Web UI: `PUT /api/dpi/targets` (порт 9091) или встроенные пресеты:

```sh
curl -X POST http://<router>:9091/api/dpi/presets/discord-youtube/apply
```

Доступные пресеты: `discord`, `youtube`, `discord-youtube`. Список — `GET /api/dpi/presets`.

При непустом `dpi_targets` файл `/opt/etc/sign-craze/dpi-hostlist.txt` пишется автоматически (один домен на строку).

---

## 10. Проверка прокси с клиента

С устройства, подключённого к роутеру (телефон/ноутбук):

```sh
# Внешний IP должен совпадать с IP outbound-сервера
curl https://api.ipify.org
curl https://ifconfig.me

# Проверка задержки (должна быть выше прямой — трафик идёт через прокси)
ping -c 5 1.1.1.1
```

Ресурсы для проверки утечек DNS / WebRTC / IP:

- https://browserleaks.com/ip
- https://ipleak.net
- https://dnsleaktest.com

Если внешний IP **совпадает** с outbound — прокси работает.

---

## 11. Автозапуск после ребута

Уже настроен через `/opt/etc/init.d/S99signcraze` (создан в [шаге 6](#6-конфигурация-sign-craze)). Init.d shim делает на boot:

1. `sign-craze --service-start` — поднимает sing-box + nfqws2 + применяет firewall.
2. `sign-craze --service-watchdog &` — standalone watchdog процесс в фоне (PID в `/opt/var/run/sign-craze-watchdog.pid`). Каждые 30 сек проверяет критичные правила (`-i/o signbox-tun -j ACCEPT` и PREROUTING jump на `signcraze_policy`/`signcraze`). Если ndm их стёр — реапплаит за ~100ms. Это закрывает сценарий «через несколько часов sign-craze перестаёт проксировать».

Проверка:

```sh
reboot
# Подождать 1–2 минуты пока роутер поднимется

ssh admin@192.168.1.1
exec sh
sign-craze --status

# Watchdog должен быть запущен
ps | grep service-watchdog
cat /opt/var/run/sign-craze-watchdog.pid
```

Если status показывает `running` и в `ps` есть `sign-craze --service-watchdog` — автозапуск работает.

---

## 12. Диагностика при проблемах

### 12.1. Команда диагностики

```sh
sign-craze --diag

# Machine-parseable JSON для скриптов и мониторинга (v1.4.0+)
sign-craze --diag --json
```

Выведет PASS/WARN/FAIL по пунктам:

- наличие бинарей (`sign-craze`, `sing-box`, `nfqws2`)
- валидность конфигов
- состояние init.d скриптов
- iptables / ipset правила
- маршруты и fwmark
- DNS-резолвинг
- свободное место на `/opt`
- статус swap

### 12.2. Логи

```sh
tail -f /opt/var/log/sign-craze/sign-craze.log
tail -f /opt/var/log/sign-craze/sing-box.log
```

### 12.3. Проверка процессов

```sh
ps | grep -E 'sing-box|nfqws2|sign-craze'
cat /opt/var/run/sign-craze-singbox.pid
```

### 12.4. Сброс состояния (если совсем плохо)

```sh
sign-craze --stop
sign-craze --uninstall    # полное удаление
sign-craze --install      # переустановка
sign-craze --start
```

### 12.5. Куда обращаться

- **Issues:** https://github.com/kittylabassistant/sign-craze/issues
- **BEHAVIOR_SPEC.md** — полное описание всех команд и инвариантов: https://github.com/kittylabassistant/sign-craze/blob/main/BEHAVIOR_SPEC.md

При создании issue приложите вывод `sign-craze --diag`, версию роутера и прошивки Keenetic.

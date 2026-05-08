# sign-craze

[![GitHub Release](https://img.shields.io/github/v/release/kittylabassistant/sign-craze)](https://github.com/kittylabassistant/sign-craze/releases) [![GitHub stars](https://img.shields.io/github/stars/kittylabassistant/sign-craze)](https://github.com/kittylabassistant/sign-craze/stargazers) [![GitHub License](https://img.shields.io/github/license/kittylabassistant/sign-craze)](LICENSE) [![GitHub Wiki](https://img.shields.io/badge/wiki-docs-blue)](https://github.com/kittylabassistant/sign-craze/wiki)

Go-утилита для управления межсетевым экраном на роутерах Keenetic.

Использует [sing-box](https://github.com/SagerNet/sing-box) как прокси-ядро и опционально [nfqws2.](https://github.com/nfqws/nfqws2-keenetic)

> [!WARNING]
> Данный материал подготовлен в научно‑технических целях. Sign-Craze предназначен для управления межсетевым экраном роутера Keenetic, защищающим домашнюю сеть. Разработчик не несёт ответственности за иное использование утилиты. Перед применением убедитесь, что ваши действия соответствуют законодательству вашей страны.

---

> [!IMPORTANT]
> **Юридические условия использования:**
>
> - **Лицензия.** Проект распространяется на условиях [BSD 3-Clause](LICENSE). Юридически обязывающим является только англоязычный текст в файле `LICENSE`; русский перевод предоставлен исключительно для удобства ознакомления.
> - **Отказ от гарантий.** Программное обеспечение предоставляется «КАК ЕСТЬ» (AS IS), без каких‑либо явных или подразумеваемых гарантий.
> - **Ограничение ответственности.** Автор и участники не несут ответственности за любой прямой, косвенный, случайный или последующий ущерб, включая нарушение работы сети, потерю данных или утрату доступа к оборудованию.
> - **Зона ответственности пользователя.** Применять утилиту допустимо только на оборудовании и в сетях, которыми вы владеете либо имеете явное письменное разрешение владельца.
> - **Соответствие законодательству.** Пользователь самостоятельно отвечает за соответствие своих действий законодательству страны пребывания.
> - **Согласие.** Загружая, устанавливая или используя Sign-Craze, вы подтверждаете, что ознакомились с указанными условиями и принимаете их в полном объёме.

---

![sign-craze](img/banner_release.jpg)

## Возможности

- Управление sing-box: установка, запуск, остановка, обновление, откат
- Два режима маршрутизации: `policy` — выборочная маркировка (default), `full` — весь LAN-трафик через прокси. DPI работает в обоих режимах через nfqws2 + NFQUEUE.
- Атомарное применение правил iptables/ipset с гарантированным откатом
- Гео-фильтрация через SRS rule-set (выборочная загрузка по SHA256)
- Встроенный Web UI (только из LAN): Zashboard `:9090`, admin API `:9091`, Routing Editor `:9092` (vanilla Preact + htm SPA)
- Selective DPI: desync только для выбранных доменов/SNI через `--dpi-targets`
- Firewall watchdog: автовосстановление iptables-правил каждые 30 с при работающем `--ui on`
- Управление портами и исключениями без перезапуска
- Резервное копирование и восстановление конфигурации
- Диагностический режим (`--diag`)

## Поддерживаемые архитектуры

| Платформа | GOARCH | Примечание |
| ----------- | -------- | ----------- |
| Keenetic (MIPS LE) | `mipsle` | GOMIPS=softfloat |
| Keenetic (MIPS BE) | `mips` | GOMIPS=softfloat |
| Keenetic / RPi (ARM 32) | `arm` | GOARM=7 |
| Keenetic Ultra / RPi 4 | `arm64` | |

## Требования

- Роутер Keenetic с установленным [Entware](https://help.keenetic.com/hc/ru/articles/360021214160)
- Доступ к интернету с роутера (для загрузки sing-box при установке)
- Свободное место на `/opt`: минимум 30 МБ

## Установка

> BusyBox `wget` на Keenetic без SSL — нужен `curl` (или `wget-ssl` из Entware): `opkg install curl`.

```sh
# Определить архитектуру автоматически и установить последний релиз
curl -fsSL https://github.com/kittylabassistant/sign-craze/releases/latest/download/install.sh | sh

# Альтернатива: скрипт напрямую с raw.githubusercontent.com (другой CDN —
# может пройти, если releases-домен заблокирован у провайдера или DNS).
curl -fsSL https://raw.githubusercontent.com/kittylabassistant/sign-craze/refs/heads/main/scripts/install.sh | sh

# Запустить установку (интерактивно: запросит URL прокси / outbound)
sign-craze --install

# Или без вопросов — автоопределение из конфигурации роутера
sign-craze --install-auto

# Запустить
sign-craze --start
```

> [!NOTE]
> `sing-box` загружается с GitHub Releases во время `--install` и **не входит** в sign-craze.
> `nfqws2` загружается **только при первом `sign-craze --dpi on`** или при `sign-craze --install --with-dpi` — DPI-обход отключён по умолчанию (opt-in). Флаг `--with-dpi` устанавливает nfqws2 + blob-файлы и сразу включает DPI с preset `discord-youtube` (out-of-box работа YouTube + Discord). См. [wiki/FAQ → «Работает ли DPI/nfqws2 из коробки»](https://github.com/kittylabassistant/sign-craze/wiki/FAQ#работает-ли-dpinfqws2-из-коробки).

### Offline-установка (роутер без доступа к GitHub)

Если на роутере нет интернета — скачать bundle на машине с интернетом, перенести по `scp` и запустить локально.

На машине с интернетом (укажите arch — `arm64`, `arm7`, `mipsle` или `mips`):

```sh
ARCH=mipsle
wget https://github.com/kittylabassistant/sign-craze/releases/latest/download/signcraze-${ARCH}.tar.gz
scp signcraze-${ARCH}.tar.gz root@192.168.1.1:/tmp/
```

На роутере:

```sh
cd /tmp && tar xzf signcraze-mipsle.tar.gz
cd signcraze-mipsle-bundle && ./install-offline.sh
```

> Только сам бинарь sign-craze ставится offline. `sign-craze --install` всё равно тянет sing-box с GitHub. Для полностью изолированной установки — скачайте sing-box tarball отдельно и используйте `sign-craze --install-offline /tmp/sing-box-*.tar.gz`.

## Quick Routing

sign-craze управляет маршрутизацией через файл `/opt/etc/sign-craze/routing.json`. Редактирование — через встроенный Web UI на порту 9092 либо через REST API.

### Порты управления

| Порт | Что | Когда использовать |
| ----------- | ----------- | ------------------- |
| `:9090` | Zashboard (мониторинг) | Смотреть live трафик, переключать proxy-selectors |
| `:9091` | Admin REST | Скрипты управления state, excludes, DPI |
| `:9092` | **Routing Editor** | **Редактировать правила маршрутизации** |

### Поднять Routing UI

```bash
sign-craze --ui on
# Открыть http://<router-ip>:9092
```

### Готовые пресеты

В UI на вкладке **Routing → "Пресеты ▾"**: `block-ads`, `ru-direct`, `blocked-vpn`, `discord-vpn`, `torrents-direct`, `block-bogon-udp`, `sign-craze-default`.

После любых правок — нажать **Apply**, затем `sign-craze --restart`.

### Документация

- [wiki/Routing.md](wiki/Routing.md) — обзор routing pipeline
- [wiki/Routing-Reference.md](wiki/Routing-Reference.md) — полная инструкция по routing.json и API
- [wiki/Recipe-RU-Direct.md](wiki/Recipe-RU-Direct.md) — рецепт "РФ direct, остальное VPN"
- [wiki/Recipes.md](wiki/Recipes.md) — индекс всех рецептов

## Web UI

**Web UI** (только из LAN):

- **9090** — Zashboard (управление прокси, мониторинг трафика, Clash-совместимый API). Откройте `http://<ROUTER_LAN_IP>:9090/` в браузере.
- **9091** — admin REST API sign-craze (статус, конфиг, порты, исключения, DPI targets).
- **9092** — Routing Editor SPA (визуальный редактор правил маршрутизации).

Порты 9090/9091/9092 слушают на `0.0.0.0`; правила в `filter/INPUT` (owner-comment + префикс) для блокировки Zashboard :9090 с WAN-интерфейса. Из локальной сети доступ открыт без аутентификации.

Запуск: `sign-craze --ui on`.

## Команды

```bash
sign-craze --install            Установить sing-box + правила iptables
sign-craze --install-auto       Установить без интерактивных подсказок
sign-craze --install-offline <путь>  Установить из локального бинаря
sign-craze --install --with-dpi      Установить + включить nfqws2 с preset discord-youtube

sign-craze --start              Применить правила + запустить sing-box
sign-craze --stop               Остановить + убрать правила iptables
sign-craze --restart / -r       Перезапуск (stop + start)
sign-craze --status  / -s       Показать состояние сервисов

sign-craze --update  / -u       Обновить sign-craze
sign-craze --update-geo / -g    Обновить гео-файлы (SRS rule-set)
sign-craze --update-core        Обновить бинарь sing-box
sign-craze --dpi-update         Переустановить актуальную версию nfqws2
sign-craze --reinstall          Переустановить sign-craze поверх существующей (сохраняет state.json)

sign-craze --core-list          Список зарегистрированных ядер (sing-box/xray/mihomo)
sign-craze --core <name>        Переключить активное ядро (требует --restart)
sign-craze --core-install <name> Скачать и установить указанное ядро

sign-craze --config-backup      Создать архив state.json в /opt/var/lib/sign-craze
sign-craze --config-restore <путь> Восстановить конфиг из архива

sign-craze --mode policy|full        Переключить режим маршрутизации
                                     (Legacy-имена `proxy`/`dpi`/`hybrid` мигрируются в `policy` с предупреждением.)
sign-craze --dpi on|off              Включить / выключить DPI-обход (по умолчанию off; первый `on` качает nfqws2)
sign-craze --dpi-strategy <пресет>   Установить стратегию DPI
sign-craze --dpi-targets <домены>    Selective DPI: desync только для указанных SNI (через запятую; clear — сбросить)
sign-craze --dpi-targets-list        Показать текущий список DPI-целей

sign-craze --port-add <порт>    Добавить порт в проксируемый набор
sign-craze --port-del <порт>    Удалить порт
sign-craze --port-list          Показать список портов

sign-craze --exclude-add <ip>   Добавить IP/CIDR в исключения
sign-craze --exclude-del <ip>   Удалить из исключений
sign-craze --exclude-list       Показать исключения

sign-craze --ui on|off          Включить / выключить Web UI (порты 9090/9091/9092)
                                Watchdog firewall активен, пока процесс `--ui on` работает
sign-craze --backup  / -b       Создать резервную копию конфигурации
sign-craze --restore <путь>     Восстановить из резервной копии
sign-craze --diag    / -D       Диагностика (PASS/WARN/FAIL по каждому пункту)
sign-craze --uninstall          Полное удаление: sing-box, конфиги, логи, бинарь sign-craze
sign-craze --version / -v       Показать версии sign-craze и sing-box
```

Полное описание каждой команды, инварианты iptables и форматы конфигов — в [`BEHAVIOR_SPEC.md`](BEHAVIOR_SPEC.md).

## Сборка из исходников

Go не требуется на хосте — сборка выполняется в контейнере.

```sh
# Все архитектуры + UPX
podman run --rm \
  -v $(pwd):/workspace:z -w /workspace \
  golang:1.25 make build

# Конкретная архитектура
podman run --rm \
  -v $(pwd):/workspace:z -w /workspace \
  -e GOOS=linux -e GOARCH=mipsle -e GOMIPS=softfloat -e CGO_ENABLED=0 \
  golang:1.25 go build -ldflags="-s -w" -o dist/sign-craze-mipsle ./cmd/sign-craze

# Тесты
podman run --rm -v $(pwd):/workspace:z -w /workspace golang:1.25 go test ./...

# Линтер
podman run --rm -v $(pwd):/workspace:z -w /workspace \
  golangci/golangci-lint:latest golangci-lint run ./...
```

Целевой размер бинаря после `upx --lzma`: ≤ 4 МБ.

## Архитектура

```plain
cmd/sign-craze/main.go
        │
internal/cli  (диспетчер команд)
   ├── internal/singbox   (загрузка, установка, конфиг sing-box)
   ├── internal/dpi       (nfqws2: загрузка, конфиг, NFQUEUE)
   ├── internal/firewall  (iptables/ipset: tproxy / redirect / hybrid)
   ├── internal/service   (init.d shim, lifecycle, PID-файлы)
   ├── internal/geo       (SRS rule-set, ipset-конвертация)
   └── internal/web       (HTTP: admin API + Routing Editor)
```

Подробная диаграмма потоков данных — в [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Структура файлов на роутере

```plain
/opt/sbin/sing-box                        — бинарь sing-box
/opt/sbin/sign-craze                      — этот бинарь
/opt/etc/sign-craze/config.json           — конфигурация sing-box
/opt/etc/sign-craze/nfqws2.conf           — конфигурация nfqws2 (если DPI включён)
/opt/etc/sign-craze/dpi-hostlist.txt      — список SNI-целей для Selective DPI (если задан)
/opt/etc/init.d/S99signcraze              — init.d shim (автозапуск)
/opt/var/lib/sign-craze/                  — состояние (гео-файлы, бэкапы)
/opt/var/log/sign-craze/                  — логи с ротацией
/opt/var/run/sign-craze-singbox.pid       — PID sing-box
/opt/var/run/sign-craze-nfqws2.pid        — PID nfqws2
/opt/var/lock/sign-craze.lock             — эксклюзивная блокировка
```

## Контакты

- E-Mail: [kittylabassistant@protonmail.com](mailto:kittylabassistant@protonmail.com)

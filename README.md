# sign-craze

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

## Возможности

- Управление sing-box: установка, запуск, остановка, обновление, откат
- Три режима маршрутизации: `proxy` (tproxy), `dpi` (nfqws2 + NFQUEUE), `hybrid`
- Атомарное применение правил iptables/ipset с гарантированным откатом
- Гео-фильтрация через SRS rule-set (выборочная загрузка по SHA256)
- Встроенный Web UI: Zashboard + admin API на портах `9090`/`9091`
- Управление портами и исключениями без перезапуска
- Резервное копирование и восстановление конфигурации
- Диагностический режим (`--diag`)

## Поддерживаемые архитектуры

| Платформа | GOARCH | Примечание |
|-----------|--------|-----------|
| Keenetic (MIPS LE) | `mipsle` | GOMIPS=softfloat |
| Keenetic (MIPS BE) | `mips` | GOMIPS=softfloat |
| Keenetic / RPi (ARM 32) | `arm` | GOARM=7 |
| Keenetic Ultra / RPi 4 | `arm64` | |

## Требования

- Роутер Keenetic с установленным [Entware](https://help.keenetic.com/hc/ru/articles/360021214160)
- Доступ к интернету с роутера (для загрузки sing-box при установке)
- Свободное место на `/opt`: минимум 30 МБ

## Установка

```sh
# Определить архитектуру автоматически и установить последний релиз
wget -O - https://github.com/kittylabassistant/sign-craze/releases/latest/download/install.sh | sh

# Запустить установку (интерактивно: запросит URL прокси / outbound)
sign-craze --install

# Или без вопросов — автоопределение из конфигурации роутера
sign-craze --install-auto

# Запустить
sign-craze --start
```

> [!NOTE]
> sing-box и nfqws2 загружаются во время установки с GitHub Releases и **не входят** в sign-craze.

## Команды

```bash
sign-craze --install            Установить sing-box + правила iptables
sign-craze --install-auto       Установить без интерактивных подсказок
sign-craze --install-offline <путь>  Установить из локального бинаря

sign-craze --start              Применить правила + запустить sing-box
sign-craze --stop               Остановить + убрать правила iptables
sign-craze --restart / -r       Перезапуск (stop + start)
sign-craze --status  / -s       Показать состояние сервисов

sign-craze --update  / -u       Обновить sign-craze
sign-craze --update-geo / -g    Обновить гео-файлы (SRS rule-set)
sign-craze --update-core        Обновить бинарь sing-box

sign-craze --mode proxy|dpi|hybrid   Переключить режим маршрутизации
sign-craze --dpi on|off              Включить / выключить DPI-обход
sign-craze --dpi-strategy <пресет>   Установить стратегию DPI

sign-craze --port-add <порт>    Добавить порт в проксируемый набор
sign-craze --port-del <порт>    Удалить порт
sign-craze --port-list          Показать список портов

sign-craze --exclude-add <ip>   Добавить IP/CIDR в исключения
sign-craze --exclude-del <ip>   Удалить из исключений
sign-craze --exclude-list       Показать исключения

sign-craze --ui on|off          Включить / выключить Web UI (порты 9090/9091)
sign-craze --backup  / -b       Создать резервную копию конфигурации
sign-craze --restore <путь>     Восстановить из резервной копии
sign-craze --diag    / -D       Диагностика (PASS/WARN/FAIL по каждому пункту)
sign-craze --uninstall          Удалить sing-box и правила, сохранить конфиги
sign-craze --purge              Полное удаление включая конфиги и логи
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
   └── internal/web       (HTTP: Zashboard + admin API)
```

Подробная диаграмма потоков данных — в [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md).

## Состояние разработки

| Фаза | Статус | Содержание |
|------|--------|-----------|
| Phase 0 — подготовка | ✅ | go.mod, Makefile, CI, BEHAVIOR_SPEC.md, docs |
| Phase 1 — scaffold | ✅ | cli, log, locks, exectx, errors, atomicfs, version, types |
| Phase 2 — sing-box | ✅ | download, install, config template, service shim, lifecycle |
| Phase 3 — firewall | ✅ | iptables/ipset, режимы tproxy/redirect/hybrid, Docker тесты |
| Phase 4 — DPI/nfqws2 | ✅ | download, config, NFQUEUE lifecycle |
| Phase 5 — гео-файлы | ✅ | SRS manifest, выборочная загрузка, ipset |
| Phase 6 — Web UI | 🔄 | HTTP-сервер, REST API, Zashboard embed |
| Phase 7 — release | ⏳ | GitHub Actions pipeline, install.sh |

## Структура файлов на роутере

```plain
/opt/sbin/sing-box                        — бинарь sing-box
/opt/sbin/sign-craze                      — этот бинарь
/opt/etc/sign-craze/config.json           — конфигурация sing-box
/opt/etc/sign-craze/nfqws2.conf           — конфигурация nfqws2 (если DPI включён)
/opt/etc/init.d/S05signcraze              — init.d shim (автозапуск)
/opt/var/lib/sign-craze/                  — состояние (гео-файлы, бэкапы)
/opt/var/log/sign-craze/                  — логи с ротацией
/opt/var/run/sign-craze-singbox.pid       — PID sing-box
/opt/var/run/sign-craze-nfqws2.pid        — PID nfqws2
/opt/var/lock/sign-craze.lock             — эксклюзивная блокировка
```

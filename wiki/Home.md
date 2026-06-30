![sign-craze](https://raw.githubusercontent.com/kittylabassistant/sign-craze/main/img/banner_release.jpg)

# sign-craze

Go-утилита для управления межсетевым экраном на роутерах [Keenetic](https://help.keenetic.com/hc/ru). Поддерживает три прокси-ядра с единым управлением через Web UI: [sing-box](https://sing-box.sagernet.org/) (умолчание), [xray](https://xtls.github.io/) и [mihomo](https://wiki.metacubex.one/); дополнительно — supervised peers **[naiveproxy](https://github.com/klzgrad/naiveproxy)** и **[mieru](https://github.com/enfein/mieru)** (process chain через socks5-outbound sing-box). Опционально [nfqws2](https://github.com/nfqws/nfqws2-keenetic) для DPI-обхода. Чистая реализация (clean-room) без переиспользования кода XKeen.

> [!WARNING]
> Материал подготовлен в научно-технических целях. Sign-Craze предназначен для управления межсетевым экраном роутера Keenetic в домашней сети. Разработчик не несёт ответственности за иное использование. Перед применением убедитесь, что ваши действия соответствуют законодательству вашей страны.

> [!IMPORTANT]
> **Юридические условия использования:** проект распространяется на условиях [BSD 3-Clause](https://github.com/kittylabassistant/sign-craze/blob/main/LICENSE). ПО предоставляется «КАК ЕСТЬ» (AS IS), без гарантий. Применять допустимо только на оборудовании и в сетях, которыми вы владеете либо имеете явное письменное разрешение владельца. Полный текст — в [LICENSE](https://github.com/kittylabassistant/sign-craze/blob/main/LICENSE) и [LICENSE.ru.md](https://github.com/kittylabassistant/sign-craze/blob/main/docs/LICENSE.ru.md).

---

## Навигация по wiki

| Страница | Содержание |
| ---------- | ----------- |
| **[Installation](Installation)** | Пошаговая инструкция: от форматирования флешки и [opkg](https://openwrt.org/docs/guide-user/additional-software/opkg)/IPK до запуска прокси |
| **[Install за DPI](Install-Behind-DPI)** | Установка, когда GitHub/CDN недоступны напрямую (offline-метод) |
| **[Routing](Routing)** | Как работает маршрутизация, порты, пресеты |
| **[Routing Reference](Routing-Reference)** | Справочник: схема `routing.json`, теги, rule-sets |
| **[Recipes](Recipes)** | Готовые сценарии конфигурации (например «РФ direct, остальное VPN») |
| **[FAQ](FAQ)** | Частые вопросы |

## Возможности

- Управление прокси-ядром: установка, запуск, остановка, обновление, откат. Поддерживается три ядра: **sing-box** (умолчание), **xray** (PQ-VLESS/Vision UDP443, xhttp packet-up), **mihomo** ([Hysteria2](https://v2.hysteria.network/), [TUIC](https://github.com/EAimTY/tuic), [WireGuard](https://www.wireguard.com/))
- **Мульти-ядерность (v1.0.0+)**: Web UI `:9092` полностью унифицирован — routing, inbound, outbound и пресеты работают одинаково для всех трёх ядер. Несовместимые конструкции (например `.srs` URL на mihomo или TUN inbound на xray) отображаются как предупреждения, не блокируя Apply
- **Supervised peers (v1.3.0)**: дополнительные протоколы **naiveproxy** и **mieru** запускаются как локальный daemon (`127.0.0.1`), sing-box заворачивает их в socks5-outbound (process chain). URL: `naive+https://…`, `mierus://…`. CLI: `--with-naive`, `--update-naive`. Поддерживается только для ядра sing-box
- Два режима маршрутизации: `policy` (Keenetic IP Policy через RCI, default) и `full` (legacy с собственным fwmark/ipset)
- Атомарное применение правил [iptables](https://www.netfilter.org/projects/iptables/index.html)/[ipset](https://ipset.netfilter.org/) с гарантированным откатом
- DPI bypass через `nfqws2` (opt-in, **off by default**) — включается `sign-craze --dpi on`; selective режим `--dpi-targets` ограничивает desync выбранными доменами (Discord, YouTube) через `nfqws2 --hostlist`
- Firewall watchdog: автоматическое восстановление правил после ndm reconciliation (защита от "перестаёт проксировать через несколько часов")
- Гео-фильтрация через [SRS rule-set](https://sing-box.sagernet.org/configuration/rule-set/) (выборочная загрузка по SHA256)
- Встроенный Web UI (только из LAN): [Zashboard](https://github.com/Zephyruso/zashboard) на порту `9090` (реальное дерево прокси и счётчики трафика через Clash API реверс-прокси на sing-box), admin REST API на `9091`, Routing Editor SPA на `9092`
- Routing Editor автоматически инициализируется из `state.outbounds` при первом запуске `--ui` (routing.json bootstrap)
- Встроенные пресеты роутинга (8 шт.): `sign-craze-default`, `block-ads`, `ru-direct`, `ru-direct-rest-vpn`, `blocked-vpn`, `discord-vpn`, `torrents-direct`, `block-bogon-udp`. Применяются из Web UI `:9092` либо из CLI прямо при установке — `--preset <name>` (список — `--preset-list`, v1.5.0). Отдельно DPI-пресеты `discord`/`youtube`/`discord-youtube`
- Управление портами и исключениями без перезапуска
- Резервное копирование и восстановление конфигурации
- Диагностический режим: `--diag` с PASS/WARN/FAIL по каждому пункту; `--diag --json` — машиночитаемый вывод для скриптов и мониторинга (v1.4.0)
- ANSI-цветной вывод CLI с opt-out `--no-color` / `NO_COLOR` / `FORCE_COLOR` (v1.2.0)
- Два канала установки: бинарь из GitHub Releases (`install.sh`) или **opkg/IPK-пакет для [Entware](https://entware.net/)** (`opkg install sign-craze`, v1.6.0)

## Поддерживаемые архитектуры

| Платформа | GOARCH | Примечание |
| ----------- | -------- | ----------- |
| Keenetic (MIPS LE) | `mipsle` | GOMIPS=softfloat |
| Keenetic (MIPS BE) | `mips` | GOMIPS=softfloat |
| Keenetic / RPi (ARM 32) | `arm` | GOARM=7 |
| Keenetic Ultra / RPi 4 | `arm64` | |

Целевой размер бинаря после `upx --lzma`: ≤ 4 МБ. RAM-бюджет: 128 МБ (общий RSS sing-box + sign-craze + nfqws2).

## История версий (ключевое)

Текущая версия — **v1.6.3**. Полный журнал изменений — в [CHANGELOG.md](https://github.com/kittylabassistant/sign-craze/blob/main/CHANGELOG.md).

| Версия | Ключевое |
| ------ | -------- |
| v1.0.0 | Унифицированная мульти-ядерность (sing-box / xray / mihomo), единый Routing Editor |
| v1.1.x | Routing UI: пресеты AS-IS/TO-BE, клиентский буфер Save/Cancel, светлая тема ☀/☾ |
| v1.2.0 | ANSI-цветной CLI + opt-out `--no-color` / `NO_COLOR` / `FORCE_COLOR` |
| v1.3.0 | Supervised peers: naiveproxy + mieru (process chain через sing-box) |
| v1.4.x | Пост-аудитный hardening: firewall batching, `--diag --json`, cosign/SLSA provenance, reproducible builds |
| v1.5.0 | CLI `--preset <name>` / `--preset-list` — пресеты роутинга прямо при установке |
| v1.6.x | opkg/IPK канал дистрибуции для Entware; удаление мёртвого кода и bcrypt auth-каркаса |

## Структура файлов на роутере

```plain
/opt/sbin/sign-craze                       — основной бинарь
/opt/sbin/sing-box                         — прокси-ядро (sing-box, умолчание)
/opt/sbin/xray                             — прокси-ядро (xray, если установлено через --core xray)
/opt/sbin/mihomo                           — прокси-ядро (mihomo, если установлено через --core mihomo)
/opt/sbin/naive                            — supervised peer naiveproxy (если установлен через --with-naive)
/opt/sbin/mieru                            — supervised peer mieru (если установлен)
/opt/sbin/nfqws2                           — DPI desync (если включён)
/opt/etc/sign-craze/config.json            — конфиг sing-box (TUN/TProxy, fwmark 0x53)
/opt/etc/sign-craze/xray/config.json       — конфиг xray (TProxy, dokodemo-door)
/opt/etc/sign-craze/mihomo/config.yaml     — конфиг mihomo (TProxy)
/opt/etc/sign-craze/state.json             — состояние sign-craze (core, mode, outbounds, ports, dpi_targets, ...)
/opt/etc/sign-craze/routing.json           — пользовательские routing-правила (Web UI, core-agnostic)
/opt/etc/sign-craze/nfqws2.conf            — конфиг nfqws2 (если DPI включён)
/opt/etc/sign-craze/dpi-hostlist.txt       — список доменов для selective DPI desync
/opt/etc/init.d/S99signcraze               — init.d shim (автозапуск sing-box + watchdog)
/opt/etc/ndm/netfilter.d/50-sign-craze     — NDM hook: реапплай правил после rebuild
/opt/var/lib/sign-craze/geo/               — гео-файлы (*.srs)
/opt/var/lib/sign-craze/backups/           — снимки tar.gz
/opt/var/log/sign-craze/sign-craze.log     — структурированные логи (slog JSON, ротация)
/opt/var/log/sign-craze/sing-box.log       — лог sing-box
/opt/var/log/sign-craze/boot.log           — stderr init.d shim
/opt/var/run/sign-craze-singbox.pid        — PID sing-box
/opt/var/run/sign-craze-naive.pid          — PID naiveproxy (если включён)
/opt/var/run/sign-craze-nfqws2.pid         — PID nfqws2
/opt/var/run/sign-craze-watchdog.pid       — PID firewall watchdog
/opt/var/lock/sign-craze.lock              — эксклюзивная блокировка операций
```

## Полезные ссылки

- **Репозиторий:** <https://github.com/kittylabassistant/sign-craze>
- **Releases:** <https://github.com/kittylabassistant/sign-craze/releases>
- **Issues:** <https://github.com/kittylabassistant/sign-craze/issues>
- **README:** [README.md](https://github.com/kittylabassistant/sign-craze/blob/main/README.md)
- **Поведенческая спецификация:** [BEHAVIOR_SPEC.md](https://github.com/kittylabassistant/sign-craze/blob/main/docs/BEHAVIOR_SPEC.md)
- **Архитектура:** [docs/ARCHITECTURE.md](https://github.com/kittylabassistant/sign-craze/blob/main/docs/ARCHITECTURE.md)
- **Лицензия:** [LICENSE](https://github.com/kittylabassistant/sign-craze/blob/main/LICENSE) (BSD 3-Clause)

## Контакты

- E-Mail: [kittylabassistant@protonmail.com](mailto:kittylabassistant@protonmail.com)

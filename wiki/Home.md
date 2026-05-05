![sign-craze](https://raw.githubusercontent.com/kittylabassistant/sign-craze/main/img/banner_release.jpg)

# sign-craze

Go-утилита для управления межсетевым экраном на роутерах Keenetic. Использует [sing-box](https://github.com/SagerNet/sing-box) как прокси-ядро и опционально [nfqws2](https://github.com/nfqws/nfqws2-keenetic) для DPI-обхода. Чистая реализация (clean-room) без переиспользования кода XKeen.

> [!WARNING]
> Материал подготовлен в научно-технических целях. Sign-Craze предназначен для управления межсетевым экраном роутера Keenetic в домашней сети. Разработчик не несёт ответственности за иное использование. Перед применением убедитесь, что ваши действия соответствуют законодательству вашей страны.

> [!IMPORTANT]
> **Юридические условия использования:** проект распространяется на условиях [BSD 3-Clause](https://github.com/kittylabassistant/sign-craze/blob/main/LICENSE). ПО предоставляется «КАК ЕСТЬ» (AS IS), без гарантий. Применять допустимо только на оборудовании и в сетях, которыми вы владеете либо имеете явное письменное разрешение владельца. Полный текст — в [LICENSE](https://github.com/kittylabassistant/sign-craze/blob/main/LICENSE) и [LICENSE.ru.md](https://github.com/kittylabassistant/sign-craze/blob/main/docs/LICENSE.ru.md).

---

## Навигация по wiki

| Страница | Содержание |
| ---------- | ----------- |
| **[Installation](Installation)** | Пошаговая инструкция: от форматирования флешки до запуска прокси |
| **[FAQ](FAQ)** | Частые вопросы (в разработке) |

## Возможности

- Управление sing-box: установка, запуск, остановка, обновление, откат
- Два режима маршрутизации: `policy` (Keenetic IP Policy через RCI, default) и `full` (legacy с собственным fwmark/ipset)
- Атомарное применение правил iptables/ipset с гарантированным откатом
- DPI bypass через `nfqws2` (opt-in, **off by default**) — включается `sign-craze --dpi on`; selective режим `--dpi-targets` ограничивает desync выбранными доменами (Discord, YouTube) через `nfqws2 --hostlist`
- Firewall watchdog: автоматическое восстановление правил после ndm reconciliation (защита от "перестаёт проксировать через несколько часов")
- Гео-фильтрация через SRS rule-set (выборочная загрузка по SHA256)
- Встроенный Web UI: admin REST API на порту `9091` + Routing Editor SPA на `9092`
- Встроенные пресеты роутинга: `block-ads`, `ru-direct`, `blocked-vpn`, `discord-vpn`, `torrents-direct` и DPI-пресеты `discord`/`youtube`/`discord-youtube`
- Управление портами и исключениями без перезапуска
- Резервное копирование и восстановление конфигурации
- Диагностический режим (`--diag`) с PASS/WARN/FAIL по каждому пункту

## Поддерживаемые архитектуры

| Платформа | GOARCH | Примечание |
| ----------- | -------- | ----------- |
| Keenetic (MIPS LE) | `mipsle` | GOMIPS=softfloat |
| Keenetic (MIPS BE) | `mips` | GOMIPS=softfloat |
| Keenetic / RPi (ARM 32) | `arm` | GOARM=7 |
| Keenetic Ultra / RPi 4 | `arm64` | |

Целевой размер бинаря после `upx --lzma`: ≤ 4 МБ. RAM-бюджет: 128 МБ (общий RSS sing-box + sign-craze + nfqws2).

## Состояние разработки

| Фаза | Статус | Содержание |
| ------ | -------- | ----------- |
| Phase 0 — подготовка | ✅ | go.mod, Makefile, CI, BEHAVIOR_SPEC.md, docs |
| Phase 1 — scaffold | ✅ | cli, log, locks, exectx, errors, atomicfs, version, types |
| Phase 2 — sing-box | ✅ | download, install, config template, service shim, lifecycle |
| Phase 3 — firewall | ✅ | iptables/ipset, режимы tproxy/redirect/hybrid, Docker-тесты |
| Phase 4 — DPI/nfqws2 | ✅ | download, config, NFQUEUE lifecycle |
| Phase 5 — гео-файлы | ✅ | SRS manifest, выборочная загрузка, ipset |
| Phase 6 — Web UI | ✅ | HTTP-сервер, REST API, Routing Editor |
| Phase 7 — release | ✅ | GitHub Actions pipeline, install.sh, multi-arch UPX |
| Phase 8 — CLI команды | ✅ | install/start/stop/status/diag/update/uninstall/dpi/mode/ports/excludes/backup |
| Phase 9 — `--mode policy` | ✅ | Keenetic IP Policy через RCI (TUN-mode), legacy режимы → `full` |
| Phase 10 — Selective DPI | ✅ | `nfqws2 --hostlist`, `--dpi-targets`, web-пресеты Discord/YouTube |
| Phase 11 — Firewall watchdog | ✅ | Standalone `--service-watchdog` daemon, переживает ребут через init.d shim |

## Структура файлов на роутере

```plain
/opt/sbin/sign-craze                       — основной бинарь
/opt/sbin/sing-box                         — прокси-ядро
/opt/sbin/nfqws2                           — DPI desync (если включён)
/opt/etc/sign-craze/config.json            — конфиг sing-box (TUN, fwmark 0x53)
/opt/etc/sign-craze/state.json             — состояние sign-craze (mode, outbounds, ports, dpi_targets, ...)
/opt/etc/sign-craze/routing.json           — пользовательские routing-правила (Web UI)
/opt/etc/sign-craze/nfqws2.conf            — конфиг nfqws2 (если DPI включён)
/opt/etc/sign-craze/dpi-hostlist.txt       — список доменов для selective DPI desync
/opt/etc/sign-craze/admin.creds            — bcrypt-хэш для Web UI
/opt/etc/init.d/S05signcraze               — init.d shim (автозапуск sing-box + watchdog)
/opt/etc/ndm/netfilter.d/50-sign-craze     — NDM hook: реапплай правил после rebuild
/opt/var/lib/sign-craze/geo/               — гео-файлы (*.srs)
/opt/var/lib/sign-craze/backups/           — снимки tar.gz
/opt/var/log/sign-craze/sign-craze.log     — структурированные логи (slog JSON, ротация)
/opt/var/log/sign-craze/sing-box.log       — лог sing-box
/opt/var/log/sign-craze/boot.log           — stderr init.d shim
/opt/var/run/sign-craze-singbox.pid        — PID sing-box
/opt/var/run/sign-craze-nfqws2.pid         — PID nfqws2
/opt/var/run/sign-craze-watchdog.pid       — PID firewall watchdog
/opt/var/lock/sign-craze.lock              — эксклюзивная блокировка операций
```

## Полезные ссылки

- **Репозиторий:** <https://github.com/kittylabassistant/sign-craze>
- **Releases:** <https://github.com/kittylabassistant/sign-craze/releases>
- **Issues:** <https://github.com/kittylabassistant/sign-craze/issues>
- **README:** [README.md](https://github.com/kittylabassistant/sign-craze/blob/main/README.md)
- **Поведенческая спецификация:** [BEHAVIOR_SPEC.md](https://github.com/kittylabassistant/sign-craze/blob/main/BEHAVIOR_SPEC.md)
- **Архитектура:** [docs/ARCHITECTURE.md](https://github.com/kittylabassistant/sign-craze/blob/main/docs/ARCHITECTURE.md)
- **Лицензия:** [LICENSE](https://github.com/kittylabassistant/sign-craze/blob/main/LICENSE) (BSD 3-Clause)

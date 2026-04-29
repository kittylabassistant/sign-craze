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
- Три режима маршрутизации: `proxy` (TPROXY), `dpi` (NFQUEUE + nfqws2), `hybrid`
- Атомарное применение правил iptables/ipset с гарантированным откатом
- Гео-фильтрация через SRS rule-set (выборочная загрузка по SHA256)
- Встроенный Web UI: Zashboard + admin REST API на портах `9090`/`9091`
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
| Phase 6 — Web UI | ✅ | HTTP-сервер, REST API, Zashboard embed |
| Phase 7 — release | ✅ | GitHub Actions pipeline, install.sh, multi-arch UPX |
| Phase 8 — on-device | 🔄 | Тестирование на реальном Keenetic |

## Структура файлов на роутере

```plain
/opt/sbin/sign-craze                  — основной бинарь
/opt/sbin/sing-box                    — прокси-ядро
/opt/etc/sign-craze/config.json       — конфиг sing-box (TPROXY, fwmark 0x53)
/opt/etc/sign-craze/nfqws2.conf       — конфиг nfqws2 (если DPI включён)
/opt/etc/sign-craze/admin.creds       — bcrypt-хэш для Web UI
/opt/etc/init.d/S05signcraze          — init.d shim (автозапуск)
/opt/var/lib/sign-craze/geo/          — гео-файлы (*.srs)
/opt/var/lib/sign-craze/backups/      — снимки tar.gz
/opt/var/log/sign-craze/              — логи с ротацией
/opt/var/run/sign-craze-singbox.pid   — PID sing-box
/opt/var/run/sign-craze-nfqws2.pid    — PID nfqws2
/opt/var/lock/sign-craze.lock         — эксклюзивная блокировка операций
```

## Полезные ссылки

- **Репозиторий:** <https://github.com/kittylabassistant/sign-craze>
- **Releases:** <https://github.com/kittylabassistant/sign-craze/releases>
- **Issues:** <https://github.com/kittylabassistant/sign-craze/issues>
- **README:** [README.md](https://github.com/kittylabassistant/sign-craze/blob/main/README.md)
- **Поведенческая спецификация:** [BEHAVIOR_SPEC.md](https://github.com/kittylabassistant/sign-craze/blob/main/BEHAVIOR_SPEC.md)
- **Архитектура:** [docs/ARCHITECTURE.md](https://github.com/kittylabassistant/sign-craze/blob/main/docs/ARCHITECTURE.md)
- **Лицензия:** [LICENSE](https://github.com/kittylabassistant/sign-craze/blob/main/LICENSE) (BSD 3-Clause)

# E2E-тесты для Keenetic

Набор скриптов для прогона на живом роутере Keenetic (KN-1410, KN-Giga и др.).
Покрывает Phase 8.5 (install/start/status), 9.8 (policy mode), 10 (selective DPI hostlist).

## Предварительные условия

- SSH-доступ к роутеру: пара ключей в `~/.ssh/keenetic` (или указанный через `KEENETIC_KEY`).
- Entware установлен на роутере (`/opt/sbin/` существует).
- На хосте разработчика: `make`, `go 1.25+`, опционально `upx`.
- Роутер: Keenetic OS 5.x, RCI API доступен на `127.0.0.1:79`.

Рекомендуется добавить запись в `~/.ssh/config`:

```
Host keenetic
    HostName 192.168.1.1
    User root
    IdentityFile ~/.ssh/keenetic
    StrictHostKeyChecking accept-new
```

## Переменные окружения

| Переменная      | Обязательная | По умолчанию         | Описание                        |
|-----------------|:------------:|----------------------|---------------------------------|
| `KEENETIC_HOST` | да           | —                    | IP-адрес роутера                |
| `KEENETIC_USER` | нет          | `root`               | Пользователь SSH                |
| `KEENETIC_KEY`  | нет          | `~/.ssh/keenetic`    | Путь к приватному SSH-ключу     |
| `DIST`          | нет          | `dist`               | Директория с артефактами сборки |
| `DPI_TARGETS`   | нет          | `discord.com,youtube.com,googlevideo.com` | Домены для DPI-hostlist (шаг 03) |

## Порядок запуска

```sh
# Полный прогон (сборка + установка + все проверки)
KEENETIC_HOST=192.168.1.1 ./scripts/e2e/run.sh

# Пропустить сборку (бинарь уже есть в dist/)
KEENETIC_HOST=192.168.1.1 ./scripts/e2e/run.sh --skip-build

# Начать с конкретного шага (при отладке)
KEENETIC_HOST=192.168.1.1 ./scripts/e2e/run.sh --from 02

# Запуск отдельного шага напрямую
KEENETIC_HOST=192.168.1.1 ./scripts/e2e/03-dpi-hostlist.sh
```

## Описание шагов

| Скрипт              | Что проверяет                                          |
|---------------------|--------------------------------------------------------|
| `00-build.sh`       | Сборка mipsle-бинаря, проверка размера (8MB/4MB UPX)  |
| `01-install.sh`     | SCP + `--install-auto`, `--status`, `--diag`, init.d  |
| `02-policy.sh`      | `--mode policy`, RCI policy=sign-craze, iptables MARK  |
| `03-dpi-hostlist.sh`| `--dpi on`, `--dpi-targets`, hostlist, nfqws2 PID     |
| `04-reboot.sh`      | Перезагрузка, автостарт через S99signcraze             |
| `99-uninstall.sh`   | `--uninstall`, проверка полной очистки файлов и RCI   |

## Ожидаемый вывод при успехе

```
============================================
  Итоговый отчёт
============================================
  Шаг 00    : PASS
  Шаг 01    : PASS
  Шаг 02    : PASS
  Шаг 03    : PASS
  Шаг 04    : PASS
  Шаг 99    : PASS
--------------------------------------------
  Итог: PASS
============================================
```

При любом FAIL оркестратор завершается с `exit 1` и прерывает дальнейший прогон.

## Безопасность SSH

Скрипты используют `StrictHostKeyChecking=accept-new`:
- Первое подключение к роутеру автоматически добавляет fingerprint в `~/.ssh/known_hosts`.
- Последующие подключения проверяют, что fingerprint совпадает — защита от MITM.
- Если роутер пересоздаёт SSH-ключ (factory reset, firmware upgrade), нужно очистить запись:
  `ssh-keygen -R "$ROUTER_HOST"` или `ssh-keygen -R "[$ROUTER_HOST]:222"`

## Прерывание

`run.sh` устанавливает trap на INT/TERM. При Ctrl-C автоматически вызывает `sign-craze --stop` на роутере, чтобы не оставлять частично настроенный firewall.

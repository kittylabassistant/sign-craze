# TROUBLESHOOTING

> Symptom → diagnosis → fix.
> Это не install-guide — он в `wiki/Installation.md`.
> Здесь только «вижу X, делаю Y, получаю Z».

---

## Как использовать

**Быстрый flow:**

```bash
sign-craze --status          # 1. что сейчас живо
sign-craze --diag            # 2. PASS/WARN/FAIL по 12 проверкам
```

Если `--diag` нашёл FAIL — найди симптом в таблице ниже и следуй fix. Если симптома нет — [собери support bundle](#сбор-support-bundle) и открой issue.

---

## Команды диагностики

| Команда | Что показывает | Когда применять |
| --------- | ---------------- | ----------------- |
| `sign-craze --status` | sing-box/nfqws2 запущены, режим, версии | Первая проверка |
| `sign-craze --diag` | PASS/WARN/FAIL по 12 проверкам: бинари, конфиг, маршруты, ipset, geo, RCI-policy | Полная самодиагностика |
| `sign-craze --version` | Версия sign-craze + sing-box | Нужна при создании issue |
| `tail -f /opt/var/log/sign-craze/sign-craze.log` | Структурированные события (slog) | Ошибки старта, reapply, firewall |
| `tail -f /opt/var/log/sign-craze/sing-box.log` | Лог sing-box | sing-box упал или не соединяется |
| `cat /opt/var/log/sign-craze/sing-box.stderr.log` | Stderr sing-box при старте | sing-box умирает сразу |
| `cat /opt/var/log/sign-craze/boot.log` | Результат `--service-start` при последнем ребуте | Автостарт не сработал |
| `iptables -t mangle -nvL` | Цепочки `signcraze_policy`, `signcraze_full` | Трафик не маркируется |
| `ipset list signcraze_ipv4 \| head` | Содержимое ipset (пустой = full-режим не матчит) | Прокси в `full` не маркирует |
| `ip rule show` | Правила маршрутизации, fwmark 0x53 lookup 83 | fwmark конфликт; loop-prevention |
| `ip route show table 83` | Default через `signbox-tun` | TUN не прикреплён, маршрут пуст |
| `ip link show signbox-tun` | Состояние TUN-интерфейса | TUN не появился или завис |

---

## Симптомы и решения

### Установка

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| `opkg install` зависает или падает | Недостаточно места на `/opt` | `df -h /opt` | Удалить лишнее или подключить USB большего объёма; ≥ 30 МБ свободно |
| `--install` выдаёт «permission denied» | SSH не от admin/root | `id` — uid=0 | `ssh admin@192.168.1.1`, затем `exec sh` |
| `sing-box` не скачивается, ошибка 429 / `rate limit` | GitHub API rate-limit (>60 req/час с IP) | `curl -s https://api.github.com/rate_limit` | Подождать час или `sign-craze --install-offline /tmp/sing-box-*.tar.gz` |
| `wget: not an http or ftp url` | BusyBox wget без SSL | — | `opkg install curl` или `opkg install wget-ssl` |
| `Недостаточно места в /opt` | `/opt` почти полон | `df -h /opt` | Очистить `/opt/var/cache/`, удалить неиспользуемые пакеты Entware |

### Запуск и режим

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| `--start: ndm: ... connection refused` или `RCI недоступен` | Режим `policy` (default) требует Keenetic RCI на `127.0.0.1:79`. KeeneticOS < 5.0 не имеет endpoint | `curl -s http://127.0.0.1:79/rci/show/ip/policy` | `sign-craze --mode full && sign-craze --start` |
| `--start: fwmark conflict` или `ErrFWMarkConflict` | fwmark `0x53` занят XKeen или скриптом с другой таблицей | `ip rule show \| grep 0x53` | Остановить конкурирующий сервис или поменять его fwmark. XKeen и sign-craze не должны работать в одном режиме |
| sing-box процесс умирает сразу | Ошибка конфига, занятый TUN, нехватка памяти | `cat /opt/var/log/sign-craze/sing-box.stderr.log` | `sign-craze --reinstall` пересоздаст конфиг; зависший TUN: `ip tuntap del mode tun name signbox-tun`, затем `sign-craze --start` |
| `--start: TUN attach: timeout` | `signbox-tun` не появился за timeout | `ip link show signbox-tun` | sing-box не поднялся — `sing-box.stderr.log`; занятый netdev: `ip tuntap del mode tun name signbox-tun` |
| `--start` сообщает «уже запущен (pid X)», но ничего не работает | Зависший PID-файл от предыдущего краша | `cat /opt/var/run/sign-craze-singbox.pid` → `ps \| grep sing-box` | Если процесса нет: `sign-craze --stop && sign-craze --start` |
| `--start` в режиме `policy`: «policy not found» или mark = 0 | Policy удалена вручную через UI Keenetic | `sign-craze --diag` → `keenetic-policy` | `sign-craze --start` пересоздаст policy через RCI |

### Трафик не проксируется

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| Внешний IP не сменился после `--start` | sing-box запущен, маршрутизация не работает | `sign-craze --diag`; `ip route show table 83` | Если table 83 пуста — TUN не прикреплён; `sign-craze --restart` |
| В режиме `full`: ipset пустой | Гео не загружено / дамп не восстановлен при старте | `ipset list signcraze_ipv4 \| head` | `sign-craze --update-geo && sign-craze --restart`. После `--update-geo` дамп пишется в `/opt/share/sign-craze/ipset.dump` |
| Loop трафика / бесконечная рекурсия в TUN | fwmark 0x53 не выставляется sing-box (некорректный `tproxy-mark` в конфиге) | `iptables -t mangle -nvL \| grep 0x53`; `ip rule show \| grep 0x53` | Проверить `tproxy-mark: "0x53"` в `/opt/etc/sign-craze/config.json`; пересоздать через `sign-craze --install` |
| Устройство не проксируется в `policy` | Не привязано к policy «sign-craze» в Keenetic UI | Keenetic UI → Приоритеты подключений | Привязать устройство; сохранить конфигурацию |
| Кратковременная потеря трафика у устройств в policy | NDM-rebuild сбросил `signcraze_policy`; hook не успел восстановить (100–500 ms на MIPS) | `iptables -t mangle -L \| grep signcraze_policy` | Нормально при изменениях Keenetic. Если chain пустой > минуты: `sign-craze --reapply` |

### Прокси перестаёт работать через несколько часов

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| Соединения таймаутят, `curl` через прокси зависает, но sing-box запущен | NDM пересобрал iptables; hook сработал, но watchdog не запущен (`--ui on` не активен) | `iptables -t mangle -L signcraze_policy -n 2>&1` — цепочка пустая или отсутствует | `sign-craze --reapply` восстановит немедленно. Для постоянной защиты держать `sign-craze --ui on` в качестве сервиса (watchdog проверяет правила каждые 30 с). |
| Периодические кратковременные обрывы, восстанавливаются сами через ~30 с | Watchdog (`--ui on`) активен, NDM вызывает rebuild без события hook'а | `tail -n 50 /opt/var/log/sign-craze/sign-craze.log \| grep watchdog` | Нормальное поведение watchdog при активном `--ui on`. Если восстановление не происходит — `sign-craze --reapply`. |

### NDM hook и persistence

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| После изменения конфигурации Keenetic правила исчезают и не восстанавливаются | NDM hook не вызывается или падает | `ls -la /opt/etc/ndm/netfilter.d/50-sign-craze` — должен быть исполняемым | `chmod +x /opt/etc/ndm/netfilter.d/50-sign-craze`. Убедиться что hook заканчивается `exit 0` |
| Правила не восстанавливаются автоматически | Keenetic не генерирует event для типа изменения | `sign-craze --reapply` вручную | Применить вручную. Если помогает — hook пропускает events. Открыть issue с описанием действия |
| Policy «sign-craze» исчезла из UI Keenetic | Пользователь удалил policy вручную | `curl -s http://127.0.0.1:79/rci/show/ip/policy \| grep sign-craze` | `sign-craze --reapply` или `sign-craze --restart` воссоздадут. |

### Web UI

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| Web UI не отвечает на 9090/9091 или 9092 | UI не включён | `sign-craze --status` | `sign-craze --ui on` |
| Большой POST завешивает роутер | **Исправлено** (safety-fixes #7 — `MaxBytesReader 1MB`). Если воспроизводится — версия устарела | `sign-craze --version` | `sign-craze --update` |
| Zashboard `:9090`: `/traffic` или `/logs` зависают, нет обновлений | **Исправлено** (2026-05-06): reverse proxy буферизовал ответы; теперь `FlushInterval=-1` и `WriteTimeout=0` для стримов | — | Обновить до актуальной версии |
| Routing Editor `:9092`: Validate возвращает ошибку или 500 | **Исправлено** (2026-05-06): Validate слал пустой `{}`, Apply падал с 500 при `version=0` в routing.json; теперь routing.json читается с диска, `version=0` мигрирует в `SchemaVersion=1` | — | Обновить до актуальной версии |
| Routing Editor `:9092`: outbounds в таблице/форме не показывают адрес сервера | **Исправлено** (2026-05-06): фронтенд читал поле `port` вместо `server_port` | — | Обновить до актуальной версии |
| Routing Editor `:9092`: dropdown пресетов закрывается до клика | **Исправлено** (2026-05-06): dropdown закрывался по `mouseLeave`; теперь закрывается по клику вне элемента | — | Обновить до актуальной версии |

### Гео-фильтрация

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| `--update-geo` падает с OOM или роутер ребутится | **Исправлено в v0.4.x** (safety-fixes #14 — streaming write). Старые версии читали .srs (10–30 МБ) в RAM | `sign-craze --version` | Обновить до `>=v0.4.x`. Временно: ночью; убедиться что swap активен: `free -m` |
| `--diag` → `geo-files WARN` | `--update-geo` не запускался > 7 дней | `ls -la /opt/var/lib/sign-craze/geo/*.srs` | `sign-craze --update-geo && sign-craze --restart` |
| `--update-geo` падает на скачивании | GitHub недоступен или rate-limit | `curl -s -o /dev/null -w "%{http_code}" https://github.com` | Retry; или скопировать .srs вручную в `/opt/var/lib/sign-craze/geo/` |

### Автостарт после ребута

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| После ребута sign-craze не запустился | init.d shim упал при ожидании маршрута (timeout) | `cat /opt/var/log/sign-craze/boot.log` | Если «timeout waiting default route»: увеличить `state.BootTimeoutSec` до `120` в `/opt/etc/sign-craze/state.json`, перезагрузить |
| После ребута не запустился, boot.log отсутствует | USB примонтирован ПОСЛЕ запуска init.d (асинхронный mount Entware) | `ls /opt/etc/init.d/S99signcraze` | Известное ограничение (safety-fixes #12 PARTIAL). Проверить mount-opt Entware; `sign-craze --start` вручную |
| Шим есть, но не исполняемый | Права сбились | `ls -la /opt/etc/init.d/S99signcraze` | `chmod +x /opt/etc/init.d/S99signcraze` |
| `state.json` повреждён — `--service-start` падает | Некорректная запись при сбое питания | `cat /opt/etc/sign-craze/state.json` | `sign-craze --uninstall && sign-craze --install && sign-craze --start` |

### Persistence (Keenetic policy)

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| После ребута policy в UI есть, но трафик не проксируется | `SaveConfig` не выполнился до ребута → mark изменился | `--diag` → `keenetic-policy WARN: mark в RCI не совпадает с state` | `sign-craze --start` обновит mark; или `--stop && --start` |
| Policy не переживает ребут — исчезает из UI | `ndm.SaveConfig` не вызывался при последнем старте | `grep -i saveconfig /opt/var/log/sign-craze/sign-craze.log` | `sign-craze --restart` — SaveConfig вызывается при каждом `--start` в `policy` |

### DPI и nfqws2

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| YouTube не открывается на TV/STB/PC — SNI блокируется ISP | Устройство не получает mark `0x53`: не входит в Keenetic policy «sign-craze», nfqws2 не обрабатывает его трафик | `iptables -t mangle -nvL signcraze_policy` — строка с IP устройства отсутствует | Добавить устройство в policy «sign-craze» в Keenetic UI (Приоритеты подключений). Альтернатива: расширить список обхода через `sign-craze --dpi-update-urls <url> --restart` |
| Reality VPN handshake ломается у downstream-устройства (Beelink/RPi4) с собственным VPN-клиентом к тому же эндпоинту | nfqws2 в POSTROUTING десинкает TLS ClientHello к VPN-серверу; пакеты к VPN-IP попадают в NFQUEUE и модифицируются | `iptables -t mangle -nvL \| grep NFQUEUE` — счётчик растёт на трафике к VPN-IP | `sign-craze --dpi-exclude-ips <vpn_ip> --restart` — добавляет RETURN-правило перед NFQUEUE для указанного IP. Автоматически: IP первого `outbound.server` из конфига резолвится best-effort и исключается при старте |
| NFQUEUE счётчик UDP/443 = 0 (в выводе `iptables -t mangle -nvL`) | В v0.8.0 jump имеет `-o $WAN_IFACE`: QUIC-трафик устройств в policy уходит через TPROXY до POSTROUTING, минуя цепочку на `eth3` | `iptables -t mangle -nvL \| grep NFQUEUE` | Нормальное поведение v0.8.0. Если QUIC-клиентов нет в policy и счётчик всё равно 0 — проверить `sign-craze --diag` |

### Логи и служебные события

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| Лог заспамлен `--reapply: правила восстановлены` (200+ INFO/час) | NDM netfilter.d hook вызывает `--reapply` пачкой 5–10 раз/сек при каждом NDM-событии | `tail -f /opt/var/log/sign-craze/sign-craze.log \| grep reapply` | В v0.8.0 реализован throttle 5 с через mtime-маркер `/opt/var/run/sign-craze-reapply.last` — обновить до актуальной версии |

### Hostlist и DPI-обновления

| Симптом | Вероятная причина | Команда проверки | Fix |
| --------- | ------------------- | ----------------- | ----- |
| Hostlist auto-update не срабатывает | `dpi_update_urls` пустой или `dpi_update_interval_hours = 0`; watchdog goroutine стартует с задержкой 5 минут после `--service-watchdog` (прогрев DNS/NTP) | `grep "dpi updater" /opt/var/log/sign-craze/sign-craze.log` — нет записей | Убедиться что в state заданы непустой `dpi_update_urls` и `dpi_update_interval_hours > 0` (рекомендуется 24). Принудительное обновление: `sign-craze --dpi-update-now` |

---

## Сбор support bundle

```sh
sign-craze --diag 2>&1 | tee /tmp/diag.txt
sign-craze --version >> /tmp/diag.txt
ip rule show >> /tmp/diag.txt
ip route show table 83 >> /tmp/diag.txt
iptables -t mangle -nvL >> /tmp/diag.txt
ipset list 2>&1 | head -50 >> /tmp/diag.txt
```

Что **автоматически** включается через `--diag`:

- наличие и права бинарей
- валидность конфига (`sing-box check`)
- статус процессов
- маршрут по умолчанию
- свежесть гео-файлов
- свободность lock-файла
- состояние Keenetic policy в RCI

**Redaction**: `--diag` не собирает содержимое конфига (URL прокси), пароли, данные трафика. Безопасно прикладывать к публичному issue.

**Куда отправить**: <https://github.com/kittylabassistant/sign-craze/issues>

---

## Известные ограничения

- **Утечка трафика при WAN-fallback**: если оператор вручную назначит policy «sign-craze» WAN-fallback через Keenetic UI (`permit` с активным интерфейсом), table 4098 получит default через провайдера, и в окне reapply (~100–500 ms) возможен leak. Не делайте этого. Sign-craze создаёт policy без `permit`-канала именно для предотвращения этого. (`BEHAVIOR_SPEC.md:467-471`)

- **Watchdog для policy**: если policy «sign-craze» удалена вручную, sign-craze узнает только при следующем `--start` или `--reapply`. Восстановление iptables-правил работает через firewall watchdog (активен при `--ui on`), однако RCI policy watchdog не реализован.

- **Автостарт при медленном USB**: если флешка монтируется после init.d, шим автостарта недоступен. Зависит от Entware. Обход: `sign-craze --start` вручную. (`tasks/safety-fixes.md:107-114`)

- **Кратковременный дроп при NDM rebuild**: при изменениях Keenetic (~100–500 ms на MIPS) цепочки signcraze отсутствуют. В режиме `policy` blackhole (не утечка), но connectivity теряется до завершения reapply.

- **sing-box не watchdog'ится**: если sing-box упал, sign-craze не перезапустит автоматически. Используйте `--restart` или cron. (`BEHAVIOR_SPEC.md §4`)

- **tmp-сеты ipset при OOM**: при крайне малом RAM `ipset swap` может оставить `signcraze_*_tmp*`. Очистка: `ipset list \| grep tmp` → `ipset destroy signcraze_*_tmp*`. (`tasks/safety-fixes.md #13`)

---

### Web UI 9090 недоступен снаружи LAN

Это by design. Sign-craze добавляет правила в `filter/INPUT` с owner-комментарием для WAN-интерфейса на портах 9090/9091/9092. Локальная сеть имеет доступ без аутентификации.

Если из LAN тоже не открывается — выполните следующее:

1. Убедитесь что Web UI запущен: `sign-craze --status` должен показывать `ui: on`.
2. Проверьте iptables: `iptables -nvL INPUT` — убедитесь что ваш LAN-интерфейс не определён как WAN ошибочно.
3. Запустите диагностику: `sign-craze --diag` — проверьте пункт WAN-интерфейса.
4. При необходимости уточните определённый WAN: `sign-craze --diag` выводит `DetectISPInterface` — если результат неверный, сообщите в issue.

---

## Получение помощи

**GitHub Issues**: <https://github.com/kittylabassistant/sign-craze/issues>

К issue приложить:

1. Вывод `sign-craze --diag` (см. [сбор bundle](#сбор-support-bundle))
2. `sign-craze --version`
3. Модель роутера и KeeneticOS (`--diag` включает `ip route` и RCI-info)
4. Режим: `policy` или `full` (`--status`)
5. Краткое описание: что делали → что ожидали → что получили
6. Логи: `boot.log`, `sing-box.stderr.log`, последние строки `sign-craze.log`

Полезные ссылки:

- Спецификация: `docs/BEHAVIOR_SPEC.md`
- FAQ: `wiki/FAQ.md`
- Установка: `wiki/Installation.md`

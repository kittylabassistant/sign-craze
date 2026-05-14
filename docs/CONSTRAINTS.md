# CONSTRAINTS.md — Технические ограничения sign-craze

Single source of truth для инвариантов, нарушение которых недопустимо. Изменение любого пункта требует: ADR + правка этого файла + прохождение CI matrix.

---

## 1. Бюджеты ресурсов

| Ресурс | Лимит | Мотивация | Проверка |
|--------|-------|-----------|----------|
| Бинарь (сжатый, UPX --lzma) | ≤ 4 МБ | Flash на Entware ограничен; README:136, `CLAUDE.md:32` | `ls -lh dist/sign-craze-*` после `make upx` |
| Бинарь (несжатый mipsle) | ~9 МБ | Ориентир для отслеживания bloat; `test-roadmap.md:590` | `ls -lh dist/sign-craze-mipsle` |
| RSS всей системы (sing-box + sign-craze + nfqws2) | ≤ 128 МБ целевой; 256 МБ реально на KN-1810 | KN-1810 имеет 256 МБ RAM; `CLAUDE.md:34`, `test-roadmap.md:577,592` | `top \| grep -E 'sing-box\|sign-craze\|nfqws2'` |
| RSS sing-box | < 30 МБ | Предупреждение memory leak; `test-roadmap.md:592` | `/proc/<pid>/status`, VmRSS |
| RSS sign-craze | < 10 МБ | Управляющий процесс не должен вытеснять sing-box | `/proc/<pid>/status`, VmRSS |
| Flash (Entware-раздел) | ≥ 30 МБ свободно | 3 бинаря + geo; `BEHAVIOR_SPEC.md:49` | `df -h /opt` в `--install` до загрузки |
| HTTP-тело (admin API) | 1 МБ (`1 << 20`) | 50 МБ malformed JSON убивает 128 МБ роутер; safety-fixes #7 | `MaxBytesReader` в `apiConfigPost`, `apiPortsAdd`, `apiExcludesAdd` |
| Geo-файл (.srs) | ≤ 200 МБ hard cap; streaming, не в RAM | Потоковая запись через `WriteFileAtomicFromReader`; `internal/geo/srs.go:27` | 8 КБ буфер, `io.CopyBuffer` |

### naiveproxy daemon

При включении naive outbound добавляется отдельный процесс `naive` с RSS ~30–50 МБ
(Chrome network stack). На роутерах с 128 МБ RAM проверяйте свободную память перед
включением. Суммарный RSS sign-craze + sing-box + nfqws2 + naive может превысить 80 МБ.

---

## 2. Toolchain

| Компонент | Версия / значение | Мотивация |
|-----------|-------------------|-----------|
| Go | `1.25.9` | Стабильный MIPS-support, `log/slog`, `embed.FS`; `go.mod:3` |
| `CGO_ENABLED` | `0` | Статическая линковка, кросс-компиляция без C-toolchain; `CLAUDE.md:25`, `Makefile` |
| ldflags production | `-s -w` | Удаление debug-символов и DWARF; `Makefile`, `release.yml:54` |
| `-trimpath` | обязателен | Воспроизводимость сборки; `Makefile`, `ci.yml:97` |
| UPX | `--lzma` | Максимальное сжатие; `Makefile:47`, `release.yml:64` |
| UPX на MIPS big-endian | **пропускать** (`\|\| true`) | UPX не поддерживает mips BE; `release.yml:64` |
| Образ сборки | `golang:1.25` | Воспроизводимость; `Makefile:15` |

---

## 3. Поддерживаемые архитектуры

| GOARCH | Доп. флаг | Статус | Источник |
|--------|-----------|--------|----------|
| `arm64` | — | поддерживается | `Makefile`, `ci.yml`, `release.yml` |
| `arm` | `GOARM=7` | поддерживается | `Makefile`, `ci.yml`, `release.yml` |
| `mipsle` | `GOMIPS=softfloat` | поддерживается | `Makefile`, `ci.yml`, `release.yml` |
| `mips` | `GOMIPS=softfloat` | поддерживается | `Makefile`, `ci.yml`, `release.yml` |
| `mips`/`mipsle` с hardfloat | **ЗАПРЕЩЕНО** | KN-1810 без FPU → SIGILL; `test-roadmap.md:591` | `GOMIPS=softfloat` обязателен |
| `windows`, `darwin`, `386` | — | вне scope | — |

---

## 4. Run-time окружение

| Ограничение | Мотивация | Источник |
|-------------|-----------|----------|
| PATH обязан включать `/opt/sbin`, `/opt/bin`, `/opt/usr/sbin`, `/opt/usr/bin` | На Keenetic init.d PATH ограничен; `iptables`, `ipset`, `ip` живут в `/opt` | `internal/exectx/exec.go:19` |
| **Нет systemd** | Keenetic — собственный init; init.d shim — единственный механизм автостарта | `CLAUDE.md`, `internal/service/shim.go` |
| **Нет nftables** — только `iptables-legacy` | Стоковое ядро Keenetic (mipsel-3.4) без nf_tables | `test-roadmap.md:599` |
| **Нет `xt_TPROXY` kernel-зависимости** с v0.3 | Переход на TUN-mode; xt_TPROXY отсутствует на стоковом ядре | `BEHAVIOR_SPEC.md:7-12` |
| POSIX sh для всех shell-скриптов | bash может отсутствовать в Entware-минимуме | `CLAUDE.md:27`, shim/hook шаблоны |
| `/dev/net/tun` обязан существовать | sing-box tun inbound требует char device | `test-roadmap.md:593` |

---

## 5. CLI-инварианты

| Правило | Валидный пример | Запрещённый | Причина |
|---------|----------------|-------------|---------|
| Однобуквенные флаги — одиночный дефис `-X` | `-i`, `-v` | `-ia`, `-vv` | POSIX; `CLAUDE.md:27` |
| Длинные флаги — двойной дефис `--name` | `--install`, `--mode` | `-install` | POSIX |
| Многобуквенные короткие флаги — **запрещены** | `--install-auto` | `-ia` | Неоднозначность; `CLAUDE.md:27` |
| `--help` и `--version` — обязательны | `sign-craze --help` | отсутствие справки | Стандарт |
| Read-only команды (`--status`, `--diag`) — **никаких side-effects** | только чтение | `--status` с auto-repair | Безопасность диагностики |

---

## 6. Firewall-инварианты

Все значения жёстко зафиксированы. Изменение требует ADR + обновление этого файла.

| Параметр | Значение | Мотивация | Источник |
|----------|----------|-----------|----------|
| fwmark sign-craze | `0x53` (83) | Loop-prevention: пакеты от sing-box не попадают повторно в TPROXY | `internal/firewall/applier.go:69`, `BEHAVIOR_SPEC.md:281` |
| Routing table | `83` | Выделена для sign-craze; не пересекается с Keenetic | `internal/firewall/applier.go:70` |
| `ip rule` приоритет | `32765` | Ниже Keenetic-политик, выше `32766 unreachable` | `internal/firewall/applier.go:71`, `BEHAVIOR_SPEC.md:346` |
| sing-box TPROXY/TUN listen port | `7895` | Фиксирован в конфиге; `BEHAVIOR_SPEC.md:280` | `internal/firewall/applier.go:72` |
| sing-box loopback mark (SO_MARK) | `0x53` | Совпадает с fwmark; пакеты от прокси уходят в таблицу 83 | `BEHAVIOR_SPEC.md:281` |
| NFQUEUE num (DPI) | `200` | Фиксирован в конфиге nfqws2 и iptables-правилах | `internal/firewall/applier.go:73` |
| TUN-интерфейс sing-box | `signbox-tun` | Имя жёстко в конфиге sing-box и firewall-слое | `internal/firewall/applier.go:59`, `internal/singbox/config.go:23` |
| TUN-адреса | `172.19.0.1/30`, `fdfe:dcba:9876::1/126` | Не пересекаются с типичными Keenetic LAN | `internal/singbox/config.go:29` |
| Prefix цепочек iptables | `signcraze_*` | Namespace для idempotent cleanup | `internal/firewall/applier.go:284-309` |
| Prefix ipset-наборов | `signcraze_*` | Аналогично | `internal/firewall/applier.go:54`, `ipset.go` |
| `iptables -F` без chain | **ЗАПРЕЩЕНО** | Уничтожает Keenetic-правила; только `FlushAndDeleteChain` по owned | `CLAUDE.md` |

---

## 7. Filesystem-инварианты

Все пути жёстко зафиксированы в коде; изменение требует ADR.

```
/opt/etc/sign-craze/          — конфигурация (state.json, config.json, nfqws2.conf, dpi-hostlist.txt, admin.creds)
/opt/etc/init.d/S99signcraze  — init.d shim (автозапуск Entware)
/opt/etc/ndm/netfilter.d/50-sign-craze — NDM hook (persistence iptables)

/opt/sbin/sign-craze          — основной бинарь
/opt/sbin/sing-box            — скачанный бинарь sing-box
/opt/sbin/nfqws2              — скачанный бинарь nfqws2

/opt/var/lib/sign-craze/      — данные (geo/*.srs, backups/, cache/)
/opt/var/log/sign-craze/      — логи (ротируются)
/opt/var/run/sign-craze-singbox.pid  — PID sing-box
/opt/var/run/sign-craze-nfqws2.pid   — PID nfqws2
/opt/var/lock/sign-craze.lock — flock (эксклюзивный mutex)
/opt/share/sign-craze/ipset.dump     — персист ipset-дампа
```

Источники: `internal/singbox/install.go:23-26`, `internal/dpi/install.go:17-23` (`DefaultHostlistPath`), `internal/service/shim.go:16-18`, `internal/service/netfilter_hook.go:15-19`, `internal/geo/srs.go:21`, `internal/locks/file.go:16`, `internal/firewall/ipset_persist.go:16`, `internal/state/state.go:19` (`DPITargets`).

---

## 8. Code style-инварианты

- `cmd/` — только `main`-пакеты; вся логика в `internal/`.
- `pkg/types` — единственный публичный пакет; остальные `internal/`.
- `log/slog` — для структурированного логирования; прямой `fmt.Print*` в логах **запрещён**.
- `fmt.Errorf("context: %w", err)` — обязательное оборачивание ошибок с контекстом.
- `text/template` — для генерации shell-скриптов и shim; `encoding/json` — для sing-box конфигов.
- **Запрещено**: `bubbletea` и TUI-фреймворки; heavy runtime dependencies.
- **Запрещено**: чтение/копирование XKeen source code (clean-room; `BEHAVIOR_SPEC.md:1-6`).
- **Запрещено**: прямая запись файлов без `atomicfs` (temp + fsync + rename).
- Логин для web UI — жёстко `"admin"` (`internal/web/server.go:13`).

---

## 9. Process-инварианты

- **Atomic write**: все мутирующие записи — через `atomicfs.WriteFileAtomic` или `WriteFileAtomicFromReader` (temp → fsync → rename); `internal/atomicfs/atomicfs.go`.
- **PID-проверка**: перед операцией над процессом — `kill -0 PID` (`/proc/<pid>/status`), не доверять только PID-файлу; `internal/service/lifecycle.go`.
- **Setpgid**: daemon-процессы (sing-box, nfqws2) запускаются с `SysProcAttr{Setpgid: true}` — не умирают вместе с sign-craze; `internal/service/lifecycle.go:75`.
- **flock**: все mutating-операции захватывают `/opt/var/lock/sign-craze.lock` (LOCK_EX); `internal/locks/file.go:16`.
- **TryAcquire** для re-entrant (NDM hook `--reapply`): если заблокировано — exit 0, не ждать; `internal/locks/file.go:61`.
- **killall — запрещён**: завершать только процесс из owned PID-файла.
- **SIGTERM + grace 10 с + SIGKILL**: стандартный stop-flow; `internal/service/lifecycle.go:151`.
- **ForceDeleteTUNDevice**: вызывать до старта и после остановки sing-box, чтобы избежать `TUNSETIFF: device busy`; `internal/firewall/route.go:131`.
- **TUN-attach timeout**: 30 с (`TUNAttachTimeout`) — безопасный запас для cold-start на slow MIPS; `internal/firewall/applier.go:65`.

---

## 10. Абсолютные запреты

- `iptables -F` без указания цепочки — **никогда**; уничтожает Keenetic-правила.
- `killall` — **никогда**; только `kill <pid>` по owned PID-файлу.
- Чтение/копирование исходников XKeen — **запрещено**; clean-room; `BEHAVIOR_SPEC.md:4`.
- Bash-only конструкции (`[[`, `$(( ))`, `local`, `source`) в shell-скриптах — **запрещены**; только POSIX sh.
- Side-effects в read-only командах (`--status`, `--diag`, `--version`) — **запрещены**.
- hardfloat MIPS (`GOMIPS=hardfloat`) в любом бинаре — **запрещён**; SIGILL на KN-1810.
- Любые операции с geo-файлами с буферизацией всего файла в RAM — **запрещены**; использовать `WriteFileAtomicFromReader`.
- `http.MaxBytesReader` в admin API обязателен — убирать **запрещено**; safety-fixes #7.

---

## 11. Изменение ограничений

Любое изменение значений из секций 1–10 требует:

1. **ADR** в `docs/adr/` с мотивацией и альтернативами.
2. **Правка этого файла** (`docs/CONSTRAINTS.md`) с обновлением таблицы и источника.
3. **Прохождение CI matrix** (`ci.yml`): все 4 архитектуры — `arm64`, `arm7`, `mipsle`, `mips`.
4. Для firewall-инвариантов (§6): обновление `BEHAVIOR_SPEC.md` §3 + тест в `internal/firewall/`.
5. Для бюджетов RAM/размера (§1): валидация на реальном KN-1810 или qemu-mipsle.

# CONSTRAINTS.md — Технические ограничения sign-craze

> Версия: 2026-07-01.

Single source of truth для инвариантов, нарушение которых недопустимо. Изменение любого пункта требует: ADR + правка этого файла + прохождение CI matrix.

---

## 1. Бюджеты ресурсов

| Ресурс | Лимит | Мотивация | Проверка |
|--------|-------|-----------|----------|
| Бинарь (сжатый, [UPX](https://upx.github.io/) --lzma) | ≤ 4 МБ | Flash на [Entware](https://entware.net/) ограничен; README:136, `CLAUDE.md:32` | `ls -lh dist/sign-craze-*` после `make upx` |
| Бинарь (несжатый mipsle) | ~9 МБ | Ориентир для отслеживания bloat; `test-roadmap.md:590` | `ls -lh dist/sign-craze-mipsle` |
| RSS всей системы ([sing-box](https://sing-box.sagernet.org/) + sign-craze + [nfqws2](https://github.com/nfqws/nfqws2-keenetic)) | ≤ 128 МБ целевой; 256 МБ реально на KN-1810 | KN-1810 имеет 256 МБ RAM; `CLAUDE.md:34`, `test-roadmap.md:577,592` | `top \| grep -E 'sing-box\|sign-craze\|nfqws2'` |
| RSS sing-box | < 30 МБ | Предупреждение memory leak; `test-roadmap.md:592` | `/proc/<pid>/status`, VmRSS |
| RSS sign-craze | < 10 МБ | Управляющий процесс не должен вытеснять sing-box | `/proc/<pid>/status`, VmRSS |
| Flash (Entware-раздел) | ≥ 30 МБ свободно | 3 бинаря + geo; `BEHAVIOR_SPEC.md:49` | `df -h /opt` в `--install` до загрузки |
| HTTP-тело (admin API) | 1 МБ (`1 << 20`) | 50 МБ malformed JSON убивает 128 МБ роутер; safety-fixes #7 | `MaxBytesReader` в `apiConfigPost`, `apiPortsAdd`, `apiExcludesAdd` |
| Geo-файл (.srs) | ≤ 200 МБ hard cap; streaming, не в RAM | Потоковая запись через `WriteFileAtomicFromReader`; `internal/geo/srs.go:27` | 8 КБ буфер, `io.CopyBuffer` |

### [naiveproxy](https://github.com/klzgrad/naiveproxy) daemon

При включении naive outbound добавляется отдельный процесс `naive` с RSS ~30–50 МБ
(Chrome network stack). На роутерах с 128 МБ RAM проверяйте свободную память перед
включением. Суммарный RSS sign-craze + sing-box + nfqws2 + naive может превысить 80 МБ.

### mieru daemon

- [mieru](https://github.com/enfein/mieru): ~30 МБ RSS (отдельный процесс, supervised peer). Сумма naive+mieru не должна превышать 80 МБ для бюджета 128 МБ роутера.

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

Канонический список целевых платформ: [COMPATIBILITY_MATRIX.md](COMPATIBILITY_MATRIX.md) §1.

Инварианты:
- Поддерживаемые архитектуры: `arm64`, `arm` (GOARM=7), `mipsle`, `mips` — все с `GOMIPS=softfloat` для mips-вариантов.
- `GOMIPS=hardfloat` — **ЗАПРЕЩЕНО** на mips/mipsle: KN-1810 без FPU → SIGILL.
- `windows`, `darwin`, `386` — вне scope.

---

## 4. Run-time окружение

| Ограничение | Мотивация | Источник |
|-------------|-----------|----------|
| PATH обязан включать `/opt/sbin`, `/opt/bin`, `/opt/usr/sbin`, `/opt/usr/bin` | На [Keenetic](https://help.keenetic.com/hc/ru) init.d PATH ограничен; `iptables`, `ipset`, `ip` живут в `/opt` | `internal/exectx/exec.go:19` |
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
| `--no-color` — отключить ANSI-цвета в выводе CLI и slog handler (`internal/log/log.go`, v1.3.0+). Mandatory для скриптовой обработки. | `sign-craze --status --no-color` | отсутствие флага в скриптах | Автоматизация |

---

## 6. Firewall-инварианты

Все значения жёстко зафиксированы. Изменение требует ADR + обновление этого файла. Канонический реестр идентификаторов: [OWNERSHIP.md](OWNERSHIP.md) §5.

| Параметр | Значение | Мотивация | Источник |
|----------|----------|-----------|----------|
| [fwmark](https://www.man7.org/linux/man-pages/man7/socket.7.html) sign-craze | `0x53` (83) | Loop-prevention: пакеты от sing-box не попадают повторно в [TPROXY](https://www.kernel.org/doc/html/latest/networking/tproxy.html) | `internal/firewall/applier.go:69`, `BEHAVIOR_SPEC.md:281` |
| Routing table | `83` | Выделена для sign-craze; не пересекается с Keenetic | `internal/firewall/applier.go:70` |
| `ip rule` приоритет | `32765` | Ниже Keenetic-политик, выше `32766 unreachable` | `internal/firewall/applier.go:71`, `BEHAVIOR_SPEC.md:346` |
| sing-box TPROXY/TUN listen port | `7895` | Фиксирован в конфиге; `BEHAVIOR_SPEC.md:280` | `internal/firewall/applier.go:72` |
| sing-box loopback mark (SO_MARK) | `0x53` | Совпадает с fwmark; пакеты от прокси уходят в таблицу 83 | `BEHAVIOR_SPEC.md:281` |
| [NFQUEUE](https://www.netfilter.org/projects/libnetfilter_queue/) num (DPI) | `300` | Фиксирован в конфиге nfqws2 и [iptables](https://www.netfilter.org/projects/iptables/index.html)-правилах (совпадает с upstream nfqws2-keenetic) | `internal/firewall/applier.go:57,151` |
| TUN-интерфейс sing-box | `signbox-tun` | Имя жёстко в конфиге sing-box и firewall-слое | `internal/firewall/applier.go:59`, `internal/singbox/config.go:23` |
| TUN-адреса | `172.19.0.1/30`, `fdfe:dcba:9876::1/126` | Не пересекаются с типичными Keenetic LAN | `internal/singbox/config.go:29` |
| Prefix цепочек iptables | `signcraze_*` | Namespace для idempotent cleanup | `internal/firewall/applier.go:284-309` |
| Prefix [ipset](https://ipset.netfilter.org/)-наборов | `signcraze_*` | Аналогично | `internal/firewall/applier.go:54`, `ipset.go` |
| `iptables -F` без chain | **ЗАПРЕЩЕНО** | Уничтожает Keenetic-правила; только `FlushAndDeleteChain` по owned | `CLAUDE.md` |

---

## 7. Filesystem-инварианты

Все пути жёстко зафиксированы в коде; изменение требует ADR. Полный канонический список путей и их владельцев: [OWNERSHIP.md](OWNERSHIP.md) §6.

Инварианты:
- Все persistent-файлы живут в `/opt/etc/sign-craze/`, `/opt/var/lib/sign-craze/`, `/opt/var/log/sign-craze/`, `/opt/share/sign-craze/`.
- Volatile PID-файлы — в `/opt/var/run/`, lock — `/opt/var/lock/sign-craze.lock`.
- Все записи — атомарно (`atomicfs.WriteFileAtomic`); прямая запись без atomic-паттерна **запрещена**.
- При `--uninstall` удаляются только пути из §6 OWNERSHIP.md — никаких широких `rm -rf`.

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
- Web UI без HTTP-аутентификации (с v1.6.1); защита портов 9090/9091/9092 от WAN — только через NDM INPUT DROP.

---

## 9. Process-инварианты

- **Atomic write**: все мутирующие записи — через `atomicfs.WriteFileAtomic` или `WriteFileAtomicFromReader` (temp → fsync → rename); `internal/atomicfs/atomicfs.go`.
- **PID-проверка**: перед операцией над процессом — `kill -0 PID` (`/proc/<pid>/status`), не доверять только PID-файлу; `internal/service/lifecycle.go`.
- **Setpgid**: daemon-процессы (sing-box, nfqws2) запускаются с `SysProcAttr{Setpgid: true}` — не умирают вместе с sign-craze; `internal/service/lifecycle.go:75`.
- **[flock](https://man7.org/linux/man-pages/man2/flock.2.html)**: все mutating-операции захватывают `/opt/var/lock/sign-craze.lock` (LOCK_EX); `internal/locks/file.go:16`.
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

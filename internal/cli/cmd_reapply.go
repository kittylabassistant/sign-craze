package cli

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/locks"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

func init() {
	Register(Cmd{
		Long:    "--reapply",
		Help:    "(internal) переприменить firewall после NDM rebuild",
		Handler: handleReapply,
		Hidden:  true,
	})
}

// reapplyThrottlePeriod — минимальный интервал между двумя --reapply.
// NDM netfilter.d hook вызывает наш handler по каждому rebuild iptables
// (firewall reload, IPv6 add/del, ipset change). На активной системе
// rebuild'ы случаются пачкой по 5-10 за секунду — Apply на каждый
// триггер съедает CPU, конкурирует с самим NDM (race на цепочках) и
// генерирует ~263 INFO-записи в час в sign-craze.log.
//
// 5 секунд — компромисс: достаточно частый чтобы быстро восстановить
// правила после реального rebuild'а, и достаточно редкий чтобы
// амортизировать пачки NDM-событий.
const reapplyThrottlePeriod = 5 * time.Second

// reapplyMarkerPath — файл-маркер времени последнего успешного --reapply.
// mtime используется для throttle-проверки: если разница меньше
// reapplyThrottlePeriod — пропускаем тик. /opt/var/run переживает
// перезагрузку как tmpfs (для Entware: /opt/var/run на nvram), поэтому
// throttle-окно «живёт» ровно сессию sign-craze, что и нужно.
const reapplyMarkerPath = "/opt/var/run/sign-craze-reapply.last"

// handleReapply вызывается NDM netfilter.d hook после rebuild iptables.
// Восстанавливает mangle-цепочки sign-craze, не трогая sing-box и TUN —
// они переживают rebuild (NDM не управляет процессами и rtnetlink).
//
// Контракт hook'а:
//   - exit 0 ВСЕГДА (даже при внутренних ошибках), чтобы не ломать chain
//     обработки NDM events;
//   - non-blocking lock: если другой mutator (--start/--restart/...)
//     держит блокировку, он сам сделает Apply;
//   - throttle через mtime-маркер: NDM делает rebuild пачкой 5-10/сек,
//     обрабатывать каждый — лишняя нагрузка, race с самим NDM и
//     log spam (263 записи/час в production);
//   - early-exit если sing-box не запущен — без TUN reapply создал бы
//     правила, маршрутизирующие в несуществующий dev;
//   - использует Reconcile (idempotent re-apply через EnsureRule), не
//     Apply — последний делает auto-rollback при ошибке и pre-flights,
//     которые на reapply-пути избыточны.
func handleReapply(ctx context.Context, _ []string) error {
	if reapplyThrottled() {
		log.L().Debug("--reapply: throttled (< 5s с прошлого), пропуск")
		return nil
	}

	lock, err := locks.TryAcquire(locks.DefaultPath)
	if err != nil {
		if errors.Is(err, locks.ErrLocked) {
			log.L().Debug("--reapply: блокировка занята, пропуск")
			return nil
		}
		log.L().Warn("--reapply: захват блокировки", "err", err)
		return nil
	}
	defer func() {
		if relErr := lock.Release(); relErr != nil {
			log.L().Warn("--reapply: снятие блокировки", "err", relErr)
		}
	}()

	c := mustActiveCore()
	coreStat, err := c.NewLifecycle().Status(ctx)
	if err != nil || !coreStat.Running {
		log.L().Debug("--reapply: ядро не запущено, пропуск", "core", c.Name())
		return nil
	}

	st, err := loadState()
	if err != nil {
		log.L().Warn("--reapply: чтение state", "err", err)
		return nil
	}

	applier, err := newFirewallApplier(st)
	if err != nil {
		log.L().Warn("--reapply: инициализация applier", "err", err)
		return nil
	}

	if err := applier.Reconcile(ctx, st.Mode); err != nil {
		log.L().Warn("--reapply: повторное применение правил", "err", err)
		return nil
	}

	touchReapplyMarker()
	log.L().Info("--reapply: правила восстановлены после NDM rebuild", "mode", st.Mode)
	return nil
}

// reapplyThrottled возвращает true если последний reapply был меньше чем
// reapplyThrottlePeriod назад. Ошибки stat игнорируются (отсутствие файла =
// первый запуск = throttle-OK).
func reapplyThrottled() bool {
	info, err := os.Stat(reapplyMarkerPath)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < reapplyThrottlePeriod
}

// touchReapplyMarker обновляет mtime файла-маркера для throttle.
// Создаёт файл если отсутствует. Ошибки логируются как Debug — не критично,
// при следующем тике throttle будет пропущен и повторим Apply.
func touchReapplyMarker() {
	now := time.Now()
	if err := os.Chtimes(reapplyMarkerPath, now, now); err == nil {
		return
	}
	// Файл не существует — создаём.
	f, err := os.OpenFile(reapplyMarkerPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.L().Debug("--reapply: не удалось создать throttle-маркер", "err", err)
		return
	}
	_ = f.Close()
}

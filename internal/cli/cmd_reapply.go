package cli

import (
	"context"
	"errors"

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

// handleReapply вызывается NDM netfilter.d hook после rebuild iptables.
// Восстанавливает mangle-цепочки sign-craze, не трогая sing-box и TUN —
// они переживают rebuild (NDM не управляет процессами и rtnetlink).
//
// Контракт hook'а:
//   - exit 0 ВСЕГДА (даже при внутренних ошибках), чтобы не ломать chain
//     обработки NDM events;
//   - non-blocking lock: если другой mutator (--start/--restart/...)
//     держит блокировку, он сам сделает Apply;
//   - early-exit если sing-box не запущен — без TUN reapply создал бы
//     правила, маршрутизирующие в несуществующий dev.
func handleReapply(ctx context.Context, _ []string) error {
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

	if err := applier.Apply(ctx, st.Mode); err != nil {
		log.L().Warn("--reapply: повторное применение правил", "err", err)
		return nil
	}

	log.L().Info("--reapply: правила восстановлены после NDM rebuild", "mode", st.Mode)
	return nil
}

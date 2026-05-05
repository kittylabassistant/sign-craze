package cli

import (
	"context"

	"github.com/kittylabassistant/sign-craze/internal/firewall"
)

func init() {
	Register(Cmd{
		Long:    "--service-watchdog",
		Help:    "(internal) standalone firewall watchdog daemon, запускается из init.d shim",
		Handler: handleServiceWatchdog,
		Hidden:  true,
	})
}

// handleServiceWatchdog — блокирующий standalone-демон firewall watchdog.
//
// Назначение: запускается из init.d shim в фоне после --service-start, чтобы
// watchdog существовал и переживал ребут роутера. Без этого watchdog существует
// только в процессе `sign-craze --ui on`, который не входит в boot path init.d.
//
// Не имеет web-сервера — только горутина watchdog. Простой и лёгкий процесс
// (~5MB RSS), приемлем даже на KN-1410 с 128MB RAM.
//
// Lifecycle: блокируется до отмены ctx (SIGTERM от init.d stop). При остановке
// горутина watchdog завершается через ctx.Done.
func handleServiceWatchdog(ctx context.Context, _ []string) error {
	wd := firewall.NewWatchdog(0, reconcileFirewall)
	wd.Run(ctx)
	return nil
}

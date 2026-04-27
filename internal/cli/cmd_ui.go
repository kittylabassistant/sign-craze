package cli

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/web"
)

func init() {
	Register(Cmd{
		Long:    "--ui",
		Help:    "запустить/остановить Web UI (on|off)",
		Handler: handleUI,
	})
}

func handleUI(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--ui: требуется аргумент on|off")
	}
	switch args[0] {
	case "on":
		s, err := web.NewServer(web.ServerConfig{
			CredsPath: "/opt/etc/sign-craze/admin.creds",
		})
		if err != nil {
			return fmt.Errorf("--ui on: %w", err)
		}
		return s.Start(ctx)
	case "off":
		// off используется когда UI запущен через --service-start;
		// в режиме standalone--ui on блокируется до SIGTERM.
		return fmt.Errorf("--ui off: сервис не запущен в этом процессе")
	default:
		return fmt.Errorf("--ui: неизвестный аргумент %q, ожидается on|off", args[0])
	}
}

package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/internal/web"
)

func init() {
	Register(Cmd{
		Long:    "--ui",
		Help:    "запустить/остановить Web UI (on|off)",
		Handler: handleUI,
	})

	// Подключаем реальный get-version sing-box для StatusReader.
	state.SingboxVersion = singbox.BinaryVersion
}

func handleUI(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--ui: требуется аргумент on|off")
	}
	switch args[0] {
	case "on":
		return startUI(ctx)
	case "off":
		// off используется когда UI запущен через --service-start;
		// в режиме standalone --ui on блокируется до SIGTERM.
		return fmt.Errorf("--ui off: сервис не запущен в этом процессе")
	default:
		return fmt.Errorf("--ui: неизвестный аргумент %q, ожидается on|off", args[0])
	}
}

func startUI(ctx context.Context) error {
	runner := newRunner()
	singboxLC := newSingboxLifecycle()
	dpiLC := newDPILifecycle()

	statusReader := state.NewStatusReader(
		singboxLC, dpiLC,
		state.DefaultPath, singbox.DefaultBinPath,
		runner, time.Now().Unix(),
	)
	configRW := state.NewConfigRW(
		filepath.Join(singbox.DefaultConfigDir, "config.json"),
		singbox.DefaultBinPath, runner,
	)
	portsMgr := state.NewPortsManager(state.DefaultPath)
	excludesMgr := state.NewExcludesManager(state.DefaultPath)

	s, err := web.NewServer(web.ServerConfig{
		CredsPath: filepath.Join(singbox.DefaultConfigDir, "admin.creds"),
		Status:    statusReader,
		Config:    configRW,
		Ports:     portsMgr,
		Excludes:  excludesMgr,
	})
	if err != nil {
		return fmt.Errorf("--ui on: %w", err)
	}
	return s.Start(ctx)
}

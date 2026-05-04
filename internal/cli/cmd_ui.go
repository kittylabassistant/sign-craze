package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/routing"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/internal/web"
	"github.com/kittylabassistant/sign-craze/pkg/types"
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

	// RoutingUI deps — рендерер использует singbox.Render с подсунутым RoutingConfig,
	// OnApply триггерит regenerateConfig для немедленного применения.
	st, _ := state.Load(state.DefaultPath)

	routingDeps := &web.RoutingUIDeps{
		RoutingPath: routing.DefaultPath,
		DefaultOutboundTag: func() string {
			if st != nil && len(st.Outbounds) > 0 {
				return st.Outbounds[0].Tag
			}
			return "direct"
		},
		Renderer: func(ctx context.Context, cfg *types.RoutingConfig) ([]byte, error) {
			params := singbox.DefaultConfigParams()
			if st != nil {
				params.Mode = st.Mode
				params.Outbounds = st.Outbounds
				if len(st.Outbounds) > 0 {
					params.DefaultOutboundTag = st.Outbounds[0].Tag
				}
			}
			params.RoutingConfig = cfg
			if params.DefaultOutboundTag == "" && len(cfg.Outbounds) > 0 {
				params.DefaultOutboundTag = cfg.Outbounds[0].Tag
			}
			return singbox.Render(params)
		},
		OnApply: func(ctx context.Context) error {
			fresh, err := state.Load(state.DefaultPath)
			if err != nil {
				return err
			}
			return regenerateConfig(ctx, fresh)
		},
	}

	cfg := web.ServerConfig{
		CredsPath: filepath.Join(singbox.DefaultConfigDir, "admin.creds"),
		Status:    statusReader,
		Config:    configRW,
		Ports:     portsMgr,
		Excludes:  excludesMgr,
		RoutingUI: routingDeps,
	}
	if st != nil {
		cfg.RoutingUIEnabled = st.RoutingUIEnabled
		cfg.RoutingUIPort = st.RoutingUIPort
		cfg.RoutingUIBind = st.RoutingUIBind
	} else {
		cfg.RoutingUIEnabled = true
		cfg.RoutingUIPort = state.DefaultRoutingUIPort
	}

	s, err := web.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("--ui on: %w", err)
	}
	return s.Start(ctx)
}

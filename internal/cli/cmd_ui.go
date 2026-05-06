package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/routing"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/internal/web"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// DefaultUIPIDFile — PID-файл фонового Web UI-демона.
const DefaultUIPIDFile = "/opt/var/run/sign-craze-ui.pid"

// DefaultUIStderrLog — stdout/stderr фонового Web UI-демона.
const DefaultUIStderrLog = "/opt/var/log/sign-craze/ui.stderr.log"

func init() {
	Register(Cmd{
		Long:    "--ui",
		Help:    "запустить/остановить Web UI (on|off)",
		Handler: handleUI,
	})

	Register(Cmd{
		Long:    "--ui-daemon",
		Help:    "(internal) блокирующий Web UI, запускается из --ui on",
		Handler: handleUIDaemon,
		Hidden:  true,
	})

	// Подключаем реальный get-version sing-box для StatusReader.
	state.SingboxVersion = singbox.BinaryVersion
}

// handleUI управляет фоновым Web UI-демоном через service.Lifecycle.
// --ui on   → форкает сам себя с --ui-daemon, пишет PID, возвращает управление.
// --ui off  → SIGTERM по PID, ждёт graceful shutdown, удаляет PID.
func handleUI(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--ui: требуется аргумент on|off")
	}
	lc, err := uiLifecycle()
	if err != nil {
		return err
	}
	switch args[0] {
	case "on":
		if err := lc.Start(ctx); err != nil {
			return fmt.Errorf("--ui on: %w", err)
		}
		st, statErr := lc.Status(ctx)
		if statErr != nil {
			// Lifecycle.Start уже отрапортовал успех; не валим команду
			// из-за дёрнутой Status — просто печатаем без PID.
			slog.Warn("--ui on: Status после Start не удался", "err", statErr)
			fmt.Println("Web UI запущен (Zashboard 9090, admin REST 9091, routing editor 9092). Доступ только из LAN.")
			return nil
		}
		fmt.Printf("Web UI запущен (pid=%d, Zashboard 9090, admin REST 9091, routing editor 9092). Доступ только из LAN.\n", st.PID)
		return nil
	case "off":
		if err := lc.Stop(ctx); err != nil {
			return fmt.Errorf("--ui off: %w", err)
		}
		fmt.Println("Web UI остановлен")
		return nil
	default:
		return fmt.Errorf("--ui: неизвестный аргумент %q, ожидается on|off", args[0])
	}
}

// handleUIDaemon — блокирующая часть: реально поднимает HTTP-серверы.
// Завершается по SIGTERM/SIGINT, прокинутым через ctx из main.
func handleUIDaemon(ctx context.Context, _ []string) error {
	return runUIServer(ctx)
}

// uiLifecycle строит service.Lifecycle, который форкает текущий бинарь
// с флагом --ui-daemon. PID-файл и stderr — стандартные пути.
func uiLifecycle() (service.Lifecycle, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("--ui: путь к бинарю: %w", err)
	}
	return service.NewLifecycle(service.ProcessConfig{
		Name:       "sign-craze-ui",
		BinPath:    self,
		Args:       []string{"--ui-daemon"},
		PIDFile:    DefaultUIPIDFile,
		StderrPath: DefaultUIStderrLog,
	}), nil
}

// runUIServer собирает web.ServerConfig и блокируется на ListenAndServe
// до отмены ctx. RoutingUI всегда включён: --ui — явная команда оператора.
func runUIServer(ctx context.Context) error {
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
	dpiTargetsMgr := state.NewDPITargetsManager(state.DefaultPath)

	st, stErr := state.Load(state.DefaultPath)
	if stErr != nil {
		slog.Warn("--ui: ошибка загрузки state.json (используются дефолты RoutingUI)", "err", stErr)
	}

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
		CredsPath:  filepath.Join(singbox.DefaultConfigDir, "admin.creds"),
		Status:     statusReader,
		Config:     configRW,
		Ports:      portsMgr,
		Excludes:   excludesMgr,
		DPITargets: dpiTargetsMgr,
		Cores:      activeCoreProvider{},
		Runner:     runner,
		RoutingUI:  routingDeps,
	}
	// --ui-daemon — явный запуск UI оператором. RoutingUI на 9092 включаем
	// всегда; флаг st.RoutingUIEnabled управляет авто-стартом при boot
	// (--service-start), а не интерактивной командой.
	cfg.RoutingUIEnabled = true
	cfg.RoutingUIPort = state.DefaultRoutingUIPort
	if st != nil {
		if st.RoutingUIPort != 0 {
			cfg.RoutingUIPort = st.RoutingUIPort
		}
		if st.RoutingUIBind != "" {
			cfg.RoutingUIBind = st.RoutingUIBind
		}
	}

	s, err := web.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("--ui-daemon: %w", err)
	}
	return s.Start(ctx)
}

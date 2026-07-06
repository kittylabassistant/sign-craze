package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/netif"
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
//
// Fail-secure: bind на LAN-bridge IP (br0/br-lan); при невозможности
// определить LAN-адрес — отказываемся стартовать, иначе UI без auth/TLS
// окажется на 0.0.0.0 и будет достижим из WAN.
func runUIServer(ctx context.Context) error {
	lanIP, err := netif.DetectLANAddr()
	if err != nil {
		return fmt.Errorf("--ui-daemon: невозможно определить LAN-bridge IP (UI без auth/TLS не должен слушать 0.0.0.0): %w", err)
	}
	slog.Info("--ui-daemon: bind на LAN", "addr", lanIP)
	runner := exectx.OS
	singboxLC := singbox.DefaultLifecycle()
	dpiLC := newDPILifecycle()
	// ruleSetClient — для RoutingUIDeps.RuleSetChecker (проверка rule_set.URL
	// при добавлении через Routing UI). Таймаут — на ctx, который выставляет
	// сам web-хендлер (routingui_rulesets.go), клиент собственного Timeout
	// не имеет.
	ruleSetClient := &http.Client{}

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

	// Если routing.json ещё не существует — создаём его из текущего state.
	// Это позволяет пользователю сразу видеть настроенные outbound'ы в UI,
	// вместо пустых таблиц при первом открытии редактора.
	if st != nil {
		if bsErr := routing.BootstrapFromState(routing.DefaultPath, st); bsErr != nil {
			slog.Warn("--ui: не удалось создать routing.json из state (UI будет пустым)", "err", bsErr)
		}
	}

	routingDeps := &web.RoutingUIDeps{
		RoutingPath: routing.DefaultPath,
		DefaultOutboundTag: func() string {
			fresh, freshErr := state.Load(state.DefaultPath)
			if freshErr == nil && fresh != nil && len(fresh.Outbounds) > 0 {
				return fresh.Outbounds[0].Tag
			}
			if st != nil && len(st.Outbounds) > 0 {
				return st.Outbounds[0].Tag
			}
			return "direct"
		},
		Renderer: func(ctx context.Context, cfg *types.RoutingConfig) ([]byte, error) {
			c := mustActiveCore()
			fresh := freshStateOr(st)
			return c.RenderConfig(uiRenderParams(fresh, cfg))
		},
		ConfigFormat: func() core.ConfigFormat {
			return mustActiveCore().ConfigFormat()
		},
		ActiveGeoFormat: func() core.GeoFormat {
			return mustActiveCore().GeoFormat()
		},
		Validator: func(ctx context.Context, cfg *types.RoutingConfig) ([]string, error) {
			return validateRoutingConfig(ctx, runner, cfg, freshStateOr(st))
		},
		// RuleSetChecker — синхронная HEAD/GET-проверка rule_set.URL на
		// POST /api/rule_sets (см. RoutingUIDeps.RuleSetChecker и
		// tasks/lessons.md, инцидент 2026-05-12: rule_set с 404-URL и rule_set
		// с mismatch формата (JSON source под .srs) валили sing-box на старте
		// у живых пользователей — теперь блокируется уже на сохранении).
		// Сетевая недоступность (offline-роутер) намеренно не блокирует
		// добавление — см. Unreachable-ветку ниже.
		RuleSetChecker: func(ctx context.Context, ref types.RuleSetRef) error {
			res := routing.CheckRuleSetURL(ctx, ruleSetClient, ref)
			if res.Unreachable {
				return nil
			}
			if res.Mismatch {
				return fmt.Errorf("%s", res.Detail)
			}
			return nil
		},
		OnApply: func(ctx context.Context) error {
			fresh, loadErr := state.Load(state.DefaultPath)
			if loadErr != nil {
				return fmt.Errorf("OnApply: state.Load: %w", loadErr)
			}
			if fresh == nil {
				fresh = state.Default()
			}
			return regenerateConfig(ctx, fresh)
		},
		OnRestart: func(ctx context.Context) error {
			return withLock(ctx, func() error {
				if stopErr := doStop(ctx); stopErr != nil {
					slog.Warn("OnRestart: stop не удался, продолжаем", "err", stopErr)
				}
				return doStart(ctx)
			})
		},
	}

	cfg := web.ServerConfig{
		ListenHost: lanIP,
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
	// LAN-trust: всегда привязываем routing UI к LAN-bridge IP, игнорируя
	// st.RoutingUIBind. Старое поле "0.0.0.0" из state.json небезопасно
	// (UI без auth доступен из WAN); пользовательский custom bind поддержан
	// только если он совпадает с LAN-IP, иначе перезаписываем.
	cfg.RoutingUIBind = lanIP
	if st != nil && st.RoutingUIPort != 0 {
		cfg.RoutingUIPort = st.RoutingUIPort
	}

	s, err := web.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("--ui-daemon: %w", err)
	}
	return s.Start(ctx)
}

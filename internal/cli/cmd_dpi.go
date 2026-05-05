package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/dpi"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func init() {
	Register(Cmd{Long: "--dpi", Help: "включить/выключить DPI: on|off", Handler: handleDPI})
	Register(Cmd{Long: "--dpi-strategy", Help: "установить стратегию DPI: <preset|file://...>", Handler: handleDPIStrategy})
	Register(Cmd{Long: "--dpi-update", Help: "обновить бинарь nfqws2", Handler: handleDPIUpdate})
	Register(Cmd{Long: "--dpi-targets", Help: "selective DPI: список доменов через запятую (пусто/clear = все)", Handler: handleDPITargets})
	Register(Cmd{Long: "--dpi-targets-list", Help: "показать активные DPI targets", Handler: handleDPITargetsList})
}

func handleDPI(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--dpi: требуется аргумент on|off")
	}
	switch args[0] {
	case "on":
		return withLock(ctx, func() error { return dpiEnable(ctx) })
	case "off":
		return withLock(ctx, func() error { return dpiDisable(ctx) })
	default:
		return fmt.Errorf("--dpi: неизвестный аргумент %q", args[0])
	}
}

func dpiEnable(ctx context.Context) error {
	st, err := loadState()
	if err != nil {
		return err
	}

	// Установка бинаря, если отсутствует.
	if _, statErr := os.Stat(dpi.DefaultBinPath); statErr != nil {
		arch, archErr := types.DetectHostArch()
		if archErr != nil {
			return fmt.Errorf("--dpi on: %w", archErr)
		}
		if mkErr := os.MkdirAll(singbox.DefaultCacheDir, 0o755); mkErr != nil {
			return fmt.Errorf("--dpi on: mkdir cache: %w", mkErr)
		}
		fmt.Printf("Загрузка nfqws2 (arch=%s)...\n", arch)
		res, dlErr := dpi.Download(ctx, arch, singbox.DefaultCacheDir)
		if dlErr != nil {
			return fmt.Errorf("--dpi on: download: %w", dlErr)
		}
		if instErr := dpi.Install(res.Path, dpi.DefaultBinPath); instErr != nil {
			return fmt.Errorf("--dpi on: install: %w", instErr)
		}
	}

	// Определить ISP-интерфейс.
	routeRes, err := newRunner().Run(ctx, "ip", "route", "show", "default")
	if err != nil {
		return fmt.Errorf("--dpi on: ip route: %w", err)
	}
	iface, err := dpi.DetectISPInterface(string(routeRes.Stdout))
	if err != nil {
		return fmt.Errorf("--dpi on: ISP-интерфейс: %w", err)
	}

	// Сгенерировать nfqws2.conf (+ hostlist если targets заданы).
	if err := writeDPIConfig(iface, st.DPITargets); err != nil {
		return fmt.Errorf("--dpi on: %w", err)
	}

	st.DPIEnabled = true
	if err := saveState(st); err != nil {
		return err
	}
	fmt.Println("DPI включён. Перезапустите сервис: sign-craze --restart")
	return nil
}

// ensureDPIConfigFresh регенерирует nfqws2.conf и hostlist из текущего state.
// Безопасно вызывать на каждом --start: атомарная запись + idempotency.
// Используется из cmd_lifecycle.go перед стартом nfqws2.
func ensureDPIConfigFresh(ctx context.Context, st *state.State) error {
	routeRes, err := newRunner().Run(ctx, "ip", "route", "show", "default")
	if err != nil {
		return fmt.Errorf("ip route: %w", err)
	}
	iface, err := dpi.DetectISPInterface(string(routeRes.Stdout))
	if err != nil {
		return fmt.Errorf("ISP-интерфейс: %w", err)
	}
	return writeDPIConfig(iface, st.DPITargets)
}

// writeDPIConfig пишет nfqws2.conf и (опционально) dpi-hostlist.txt.
// targets пусто → флаг --hostlist не добавляется в args (desync для всего).
// targets непусто → hostlist пишется атомарно, путь в params, флаг добавляется.
func writeDPIConfig(iface string, targets []string) error {
	params := dpi.DefaultConfigParams()
	params.ISPInterface = iface
	if len(targets) > 0 {
		params.HostlistPath = dpi.DefaultHostlistPath
		if err := dpi.WriteHostlist(dpi.DefaultHostlistPath, targets); err != nil {
			return fmt.Errorf("hostlist: %w", err)
		}
	}
	if err := dpi.GenerateConfig(params, dpi.DefaultConfigPath); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}

func dpiDisable(_ context.Context) error {
	st, err := loadState()
	if err != nil {
		return err
	}
	st.DPIEnabled = false
	if err := saveState(st); err != nil {
		return err
	}
	fmt.Println("DPI выключен. Перезапустите сервис: sign-craze --restart")
	return nil
}

func handleDPIStrategy(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--dpi-strategy: требуется preset или file://путь")
	}
	strategy := strings.TrimSpace(args[0])
	return withLock(ctx, func() error {
		st, err := loadState()
		if err != nil {
			return err
		}
		st.DPIStrategy = strategy
		if err := saveState(st); err != nil {
			return err
		}
		fmt.Printf("DPI-стратегия установлена: %s\n", strategy)
		return nil
	})
}

func handleDPITargets(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--dpi-targets: требуется список доменов (или 'clear' для очистки)")
	}
	raw := strings.TrimSpace(args[0])
	var targets []string
	switch raw {
	case "clear", "none", "-", "":
		targets = nil
	default:
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			targets = append(targets, part)
		}
	}

	return withLock(ctx, func() error {
		st, err := loadState()
		if err != nil {
			return err
		}
		st.DPITargets = targets

		// Регенерация конфига имеет смысл только если DPI уже включён.
		// Иначе конфиг будет создан при следующем `--dpi on` с актуальными targets.
		if st.DPIEnabled {
			routeRes, rErr := newRunner().Run(ctx, "ip", "route", "show", "default")
			if rErr != nil {
				return fmt.Errorf("--dpi-targets: ip route: %w", rErr)
			}
			iface, ifErr := dpi.DetectISPInterface(string(routeRes.Stdout))
			if ifErr != nil {
				return fmt.Errorf("--dpi-targets: ISP-интерфейс: %w", ifErr)
			}
			if err := writeDPIConfig(iface, targets); err != nil {
				return fmt.Errorf("--dpi-targets: %w", err)
			}
		}

		if err := saveState(st); err != nil {
			return err
		}
		if len(targets) == 0 {
			fmt.Println("DPI targets очищены (desync применяется ко всему трафику).")
		} else {
			fmt.Printf("DPI targets: %d домен(ов). Перезапустите: sign-craze --restart\n", len(targets))
		}
		return nil
	})
}

func handleDPITargetsList(_ context.Context, _ []string) error {
	st, err := loadState()
	if err != nil {
		return err
	}
	if len(st.DPITargets) == 0 {
		fmt.Println("DPI targets не заданы (desync применяется ко всему трафику).")
		return nil
	}
	for _, t := range st.DPITargets {
		fmt.Println(t)
	}
	return nil
}

func handleDPIUpdate(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		arch, err := types.DetectHostArch()
		if err != nil {
			return fmt.Errorf("--dpi-update: %w", err)
		}
		if mkErr := os.MkdirAll(singbox.DefaultCacheDir, 0o755); mkErr != nil {
			return fmt.Errorf("--dpi-update: mkdir cache: %w", mkErr)
		}
		fmt.Printf("Загрузка nfqws2 (arch=%s)...\n", arch)
		res, err := dpi.Download(ctx, arch, singbox.DefaultCacheDir)
		if err != nil {
			return fmt.Errorf("--dpi-update: %w", err)
		}
		if !res.Downloaded {
			fmt.Println("nfqws2 уже актуален.")
			return nil
		}
		if err := dpi.Install(res.Path, dpi.DefaultBinPath); err != nil {
			return fmt.Errorf("--dpi-update: %w", err)
		}
		fmt.Printf("nfqws2 обновлён до %s.\n", res.Version)
		return nil
	})
}

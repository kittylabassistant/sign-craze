package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/dpi"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func init() {
	Register(Cmd{Long: "--dpi", Help: "включить/выключить DPI: on|off", Handler: handleDPI})
	Register(Cmd{Long: "--dpi-strategy", Help: "установить стратегию DPI: <preset|file://...>", Handler: handleDPIStrategy})
	Register(Cmd{Long: "--dpi-update", Help: "обновить бинарь nfqws2", Handler: handleDPIUpdate})
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
		if err := os.MkdirAll(singbox.DefaultCacheDir, 0o755); err != nil {
			return fmt.Errorf("--dpi on: mkdir cache: %w", err)
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

	// Сгенерировать nfqws2.conf.
	params := dpi.DefaultConfigParams()
	params.ISPInterface = iface
	if err := dpi.GenerateConfig(params, dpi.DefaultConfigPath); err != nil {
		return fmt.Errorf("--dpi on: config: %w", err)
	}

	st.DPIEnabled = true
	if err := saveState(st); err != nil {
		return err
	}
	fmt.Println("DPI включён. Перезапустите сервис: sign-craze --restart")
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

func handleDPIUpdate(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		arch, err := types.DetectHostArch()
		if err != nil {
			return fmt.Errorf("--dpi-update: %w", err)
		}
		if err := os.MkdirAll(singbox.DefaultCacheDir, 0o755); err != nil {
			return fmt.Errorf("--dpi-update: mkdir cache: %w", err)
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

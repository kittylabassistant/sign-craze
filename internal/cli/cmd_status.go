package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/version"
)

func init() {
	Register(Cmd{Short: "-s", Long: "--status", Help: "состояние сервисов и режим", Handler: handleStatus})
	Register(Cmd{Short: "-v", Long: "--version", Help: "версия sign-craze и активного ядра", Handler: handleVersion})
}

func handleStatus(ctx context.Context, _ []string) error {
	st, err := loadState()
	if err != nil {
		return fmt.Errorf("--status: %w", err)
	}

	c := mustActiveCore()

	sb, sbErr := newSingboxLifecycle().Status(ctx)
	if sbErr != nil {
		fmt.Fprintf(os.Stderr, "warn: статус %s: %v\n", c.Name(), sbErr)
	}
	dpiSt, dpiErr := newDPILifecycle().Status(ctx)
	if dpiErr != nil {
		fmt.Fprintf(os.Stderr, "warn: статус nfqws2: %v\n", dpiErr)
	}

	coreVer := ""
	if _, errStat := os.Stat(c.BinaryPath()); errStat == nil {
		v, vErr := c.BinaryVersion(ctx, newRunner())
		if vErr != nil {
			fmt.Fprintf(os.Stderr, "warn: версия %s: %v\n", c.Name(), vErr)
		}
		coreVer = v
	}

	fmt.Printf("%s:  %s\n", c.Name(), formatService(sb.Running, sb.PID))
	fmt.Printf("nfqws2:    %s\n", formatService(dpiSt.Running, dpiSt.PID))
	fmt.Printf("режим:     %s\n", string(st.Mode))
	if coreVer != "" {
		fmt.Printf("версия:    sign-craze %s / %s v%s\n", version.Short(), c.Name(), coreVer)
	} else {
		fmt.Printf("версия:    sign-craze %s / %s не установлен\n", version.Short(), c.Name())
	}
	return nil
}

func formatService(running bool, pid int) string {
	if running {
		return fmt.Sprintf("запущен  (pid %d)", pid)
	}
	return "остановлен"
}

func handleVersion(ctx context.Context, _ []string) error {
	fmt.Printf("sign-craze %s\n", version.String())

	c := mustActiveCore()
	if _, err := os.Stat(c.BinaryPath()); err == nil {
		ver, vErr := c.BinaryVersion(ctx, newRunner())
		if vErr == nil {
			fmt.Printf("%s   v%s  (установлен в %s)\n", c.Name(), ver, c.BinaryPath())
		} else {
			fmt.Printf("%s   ошибка чтения версии: %v\n", c.Name(), vErr)
		}
	} else {
		fmt.Printf("%s   не установлен\n", c.Name())
	}
	return nil
}

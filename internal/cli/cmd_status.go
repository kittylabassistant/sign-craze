package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/version"
)

func init() {
	Register(Cmd{Short: "-s", Long: "--status", Help: "состояние сервисов и режим", Handler: handleStatus})
	Register(Cmd{Short: "-v", Long: "--version", Help: "версия sign-craze и sing-box", Handler: handleVersion})
}

func handleStatus(ctx context.Context, _ []string) error {
	st, err := loadState()
	if err != nil {
		return fmt.Errorf("--status: %w", err)
	}

	sb, sbErr := newSingboxLifecycle().Status(ctx)
	if sbErr != nil {
		fmt.Fprintf(os.Stderr, "warn: статус sing-box: %v\n", sbErr)
	}
	dpi, dpiErr := newDPILifecycle().Status(ctx)
	if dpiErr != nil {
		fmt.Fprintf(os.Stderr, "warn: статус nfqws2: %v\n", dpiErr)
	}

	sbVer := ""
	if _, errStat := os.Stat(singbox.DefaultBinPath); errStat == nil {
		v, vErr := singbox.BinaryVersion(ctx, newRunner(), singbox.DefaultBinPath)
		if vErr != nil {
			fmt.Fprintf(os.Stderr, "warn: версия sing-box: %v\n", vErr)
		}
		sbVer = v
	}

	fmt.Printf("sing-box:  %s\n", formatService(sb.Running, sb.PID))
	fmt.Printf("nfqws2:    %s\n", formatService(dpi.Running, dpi.PID))
	fmt.Printf("режим:     %s\n", string(st.Mode))
	if sbVer != "" {
		fmt.Printf("версия:    sign-craze v%s / sing-box v%s\n", version.Version, sbVer)
	} else {
		fmt.Printf("версия:    sign-craze v%s / sing-box не установлен\n", version.Version)
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

	if _, err := os.Stat(singbox.DefaultBinPath); err == nil {
		ver, vErr := singbox.BinaryVersion(ctx, newRunner(), singbox.DefaultBinPath)
		if vErr == nil {
			fmt.Printf("sing-box   v%s  (установлен в %s)\n", ver, singbox.DefaultBinPath)
		} else {
			fmt.Printf("sing-box   ошибка чтения версии: %v\n", vErr)
		}
	} else {
		fmt.Println("sing-box   не установлен")
	}
	return nil
}

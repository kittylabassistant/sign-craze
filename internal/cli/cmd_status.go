package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/peer"
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

	sb, sbErr := c.NewLifecycle().Status(ctx)
	if sbErr != nil {
		fmt.Fprintf(os.Stderr, "%s статус %s: %v\n", Warn("warn:"), c.Name(), sbErr)
	}
	dpiSt, dpiErr := newDPILifecycle().Status(ctx)
	if dpiErr != nil {
		fmt.Fprintf(os.Stderr, "%s статус nfqws2: %v\n", Warn("warn:"), dpiErr)
	}

	coreVer := ""
	if _, errStat := os.Stat(c.BinaryPath()); errStat == nil {
		v, vErr := c.BinaryVersion(ctx, exectx.OS)
		if vErr != nil {
			fmt.Fprintf(os.Stderr, "%s версия %s: %v\n", Warn("warn:"), c.Name(), vErr)
		}
		coreVer = v
	}

	fmt.Printf("%s  %s\n", Key(c.Name()+":"), formatService(sb.Running, sb.PID))
	fmt.Printf("%s    %s\n", Key("nfqws2:"), formatService(dpiSt.Running, dpiSt.PID))

	// Supervised peers (mieru). Выводим только если есть mieru-outbound'ы,
	// чтобы не засорять status пустой секцией для обычных пользователей.
	if peer.HasMieruOutbounds(st.Outbounds) {
		for _, p := range peer.CollectMieruStatuses(ctx, st.Outbounds) {
			label := fmt.Sprintf("mieru[%s]:", p.Tag)
			suffix := ""
			if p.LocalPort > 0 {
				suffix = fmt.Sprintf("  (socks5 → 127.0.0.1:%d)", p.LocalPort)
			}
			fmt.Printf("%s  %s%s\n", Key(label), formatService(p.Running, p.PID), suffix)
		}
	}

	fmt.Printf("%s     %s\n", Key("режим:"), string(st.Mode))
	if coreVer != "" {
		fmt.Printf("%s    sign-craze %s / %s v%s\n", Key("версия:"), version.Short(), c.Name(), coreVer)
	} else {
		fmt.Printf("%s    sign-craze %s / %s %s\n", Key("версия:"), version.Short(), c.Name(), Err("не установлен"))
	}
	return nil
}

func formatService(running bool, pid int) string {
	return ServiceStatus(running, pid)
}

func handleVersion(ctx context.Context, _ []string) error {
	fmt.Printf("%s %s\n", Header("sign-craze"), version.String())

	c := mustActiveCore()
	if _, err := os.Stat(c.BinaryPath()); err == nil {
		ver, vErr := c.BinaryVersion(ctx, exectx.OS)
		if vErr == nil {
			fmt.Printf("%s   v%s  (установлен в %s)\n", Key(c.Name()), ver, Hint(c.BinaryPath()))
		} else {
			fmt.Printf("%s   %s %v\n", Key(c.Name()), Err("ошибка чтения версии:"), vErr)
		}
	} else {
		fmt.Printf("%s   %s\n", Key(c.Name()), Err("не установлен"))
	}
	return nil
}

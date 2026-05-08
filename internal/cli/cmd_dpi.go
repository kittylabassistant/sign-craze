package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/dpi"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
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

// detectISPInterface определяет ISP-интерфейс через `ip route show default`.
// Общая утилита для всех DPI-функций, которые рендерят nfqws2.conf.
func detectISPInterface(ctx context.Context) (string, error) {
	routeRes, err := exectx.OS.Run(ctx, "ip", "route", "show", "default")
	if err != nil {
		return "", fmt.Errorf("ip route: %w", err)
	}
	iface, err := dpi.DetectISPInterface(string(routeRes.Stdout))
	if err != nil {
		return "", fmt.Errorf("ISP-интерфейс: %w", err)
	}
	return iface, nil
}

// installNfqws2WithBlobs скачивает .ipk пакет nfqws2-keenetic, устанавливает
// бинарь и распаковывает blob-файлы (quic_initial.bin, tls_clienthello.bin).
// Используется в dpiEnable и в --install --with-dpi.
func installNfqws2WithBlobs(ctx context.Context) error {
	arch, err := types.DetectHostArch()
	if err != nil {
		return fmt.Errorf("arch: %w", err)
	}
	if mkErr := os.MkdirAll(singbox.DefaultCacheDir, 0o755); mkErr != nil {
		return fmt.Errorf("mkdir cache: %w", mkErr)
	}
	fmt.Printf("Загрузка nfqws2 (arch=%s)...\n", arch)
	res, dlErr := dpi.Download(ctx, arch, singbox.DefaultCacheDir)
	if dlErr != nil {
		return fmt.Errorf("download: %w", dlErr)
	}
	if instErr := dpi.Install(res.Path, dpi.DefaultBinPath); instErr != nil {
		return fmt.Errorf("install: %w", instErr)
	}
	if assetErr := dpi.InstallAssets(res.Path, dpi.DefaultBlobDir, dpi.DefaultLuaDir); assetErr != nil {
		// Не фатально — без lua/blob'ов часть стратегий не работает, но fallback
		// (без --lua-desync=fake:blob=...) может функционировать.
		return fmt.Errorf("install assets: %w", assetErr)
	}
	return nil
}

func dpiEnable(ctx context.Context) error {
	st, err := loadState()
	if err != nil {
		return err
	}

	// Установка бинаря + blob-файлов, если отсутствуют.
	if _, statErr := os.Stat(dpi.DefaultBinPath); statErr != nil {
		if instErr := installNfqws2WithBlobs(ctx); instErr != nil {
			return fmt.Errorf("--dpi on: %w", instErr)
		}
	}

	iface, err := detectISPInterface(ctx)
	if err != nil {
		return fmt.Errorf("--dpi on: %w", err)
	}

	// Сгенерировать nfqws2.conf (+ hostlist если targets заданы).
	if err := writeDPIConfig(iface, st); err != nil {
		return fmt.Errorf("--dpi on: %w", err)
	}

	st.DPIEnabled = true
	if err := saveState(st); err != nil {
		return err
	}
	fmt.Println("DPI включён. Перезапустите сервис: sign-craze --restart")
	return nil
}

// writeDPIConfig пишет nfqws2.conf и (опционально) dpi-hostlist.txt из state.
// st.DPIStrategy непустой → override NFQWS_ARGS (TCP/TLS-блок).
// st.DPITargets непустой → hostlist пишется атомарно, --hostlist=<path> добавляется.
// st.DPITargets пустой → флаг --hostlist не добавляется (desync для всего трафика).
func writeDPIConfig(iface string, st *state.State) error {
	params := dpi.DefaultConfigParams()
	params.ISPInterface = iface
	if st != nil && st.DPIStrategy != "" {
		params.Args = st.DPIStrategy
	}
	if st != nil && len(st.DPITargets) > 0 {
		params.HostlistPath = dpi.DefaultHostlistPath
		if err := dpi.WriteHostlist(dpi.DefaultHostlistPath, st.DPITargets); err != nil {
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

// dpiStrategyForbidden — shell-метасимволы и управляющие символы, которых
// не должно быть в --dpi-strategy. Дефолтные стратегии nfqws2 содержат
// только alphanumeric, `-`, `_`, `.`, `,`, `:`, `=`, `@`, `/`, пробел.
// Запрещённые символы могут привести к экспедиции в exec.Command аргументы
// нежелательных флагов через `splitArgs(strings.Fields(...))` или к
// shell-инъекции если стратегия попадёт в shim-script.
const dpiStrategyForbidden = ";&|$`<>(){}\\\n\r\t\x00"

const dpiStrategyMaxLen = 1024

// validateDPIStrategy — blocklist shell-метасимволов в строке стратегии.
// Allowlist слишком узок: легитимные стратегии содержат непредсказуемый
// набор символов в hostnames (`sni=fonts.google.com`), числах (`pos=1,midsld`)
// и т.п. Blocklist чёрный — короткий, безопасный, не ломает upstream-стратегии.
func validateDPIStrategy(s string) error {
	if s == "" {
		return fmt.Errorf("--dpi-strategy: пустая строка")
	}
	if len(s) > dpiStrategyMaxLen {
		return fmt.Errorf("--dpi-strategy: длина %d > лимита %d", len(s), dpiStrategyMaxLen)
	}
	if i := strings.IndexAny(s, dpiStrategyForbidden); i >= 0 {
		return fmt.Errorf("--dpi-strategy: запрещённый символ %q в позиции %d (shell-метасимволы и управляющие не допускаются)", s[i], i)
	}
	return nil
}

func handleDPIStrategy(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--dpi-strategy: требуется preset или file://путь")
	}
	strategy := strings.TrimSpace(args[0])
	if err := validateDPIStrategy(strategy); err != nil {
		return err
	}
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
		fmt.Println("Перезапустите сервис: sign-craze --restart")
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
			iface, ifErr := detectISPInterface(ctx)
			if ifErr != nil {
				return fmt.Errorf("--dpi-targets: %w", ifErr)
			}
			if err := writeDPIConfig(iface, st); err != nil {
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
		// Бинарь устанавливаем только если свежее. Assets (lua/blobs)
		// перепроверяем всегда — пользователь мог удалить директории
		// или мы добавили новые требования (lua-расширения в v0.x → v1.x).
		if res.Downloaded {
			if err := dpi.Install(res.Path, dpi.DefaultBinPath); err != nil {
				return fmt.Errorf("--dpi-update: %w", err)
			}
		}
		if err := dpi.InstallAssets(res.Path, dpi.DefaultBlobDir, dpi.DefaultLuaDir); err != nil {
			return fmt.Errorf("--dpi-update: install assets: %w", err)
		}
		if res.Downloaded {
			fmt.Printf("nfqws2 обновлён до %s.\n", res.Version)
		} else {
			fmt.Printf("nfqws2 актуален (%s); ресурсы (lua, blobs) перепроверены.\n", res.Version)
		}
		return nil
	})
}

package cli

import (
	"context"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/geo"
	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/selfupdate"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func init() {
	Register(Cmd{Short: "-u", Long: "--update", Help: "обновить sign-craze", Handler: handleUpdate})
	Register(Cmd{Short: "-g", Long: "--update-geo", Help: "обновить geo-файлы", Handler: handleUpdateGeo})
	Register(Cmd{Long: "--update-core", Help: "обновить только sing-box", Handler: handleUpdateCore})
}

func handleUpdate(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		arch, err := types.DetectHostArch()
		if err != nil {
			return fmt.Errorf("--update: %w", err)
		}
		ver, err := selfupdate.Update(ctx, ghrelease.New(), selfupdate.Options{Arch: arch})
		if err != nil {
			return fmt.Errorf("--update: %w", err)
		}
		fmt.Printf("sign-craze обновлён до %s\n", ver)
		return nil
	})
}

func handleUpdateGeo(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		manifest, err := geo.FetchManifest(ctx)
		if err != nil {
			return fmt.Errorf("--update-geo: manifest: %w", err)
		}
		needed := make([]string, 0, len(manifest.Files))
		for _, f := range manifest.Files {
			needed = append(needed, f.Name)
		}
		count, err := geo.Update(ctx, needed, geo.DefaultGeoDir)
		if err != nil {
			return fmt.Errorf("--update-geo: %w", err)
		}
		fmt.Printf("Скачано %d/%d geo-файлов\n", count, len(needed))

		// Заполнить kernel ipset из .srs и сохранить дамп для restore при ребуте
		// (safety-fixes #17). Если sing-box ещё не установлен или decompile падает —
		// логируем и продолжаем: gen-update остаётся идемпотентным.
		if err := populateAndSaveIPSet(ctx, needed); err != nil {
			log.L().Warn("--update-geo: заполнение ipset пропущено", "err", err)
		}
		return nil
	})
}

// populateAndSaveIPSet декомпилирует .srs → CIDR → ipset signcraze_ipv4/ipv6
// и сохраняет дамп в DefaultDumpFile. Файлы без CIDR (geosite-only) пропускаются.
func populateAndSaveIPSet(ctx context.Context, fileNames []string) error {
	runner := newRunner()
	var allV4, allV6 []netip.Prefix

	for _, name := range fileNames {
		if !strings.HasSuffix(name, ".srs") {
			continue
		}
		srsPath := filepath.Join(geo.DefaultGeoDir, name)
		prefixes, err := geo.DecompileSRS(ctx, runner, singbox.DefaultBinPath, srsPath)
		if err != nil {
			log.L().Debug("decompile пропущен", "file", name, "err", err)
			continue
		}
		v4, v6 := geo.SplitByFamily(prefixes)
		allV4 = append(allV4, v4...)
		allV6 = append(allV6, v6...)
	}

	if len(allV4) == 0 && len(allV6) == 0 {
		return nil
	}

	if len(allV4) > 0 {
		if err := geo.ApplyToIPSet(ctx, runner, string(types.IPSetIPv4), "inet", allV4); err != nil {
			return fmt.Errorf("apply v4: %w", err)
		}
	}
	if len(allV6) > 0 {
		if err := geo.ApplyToIPSet(ctx, runner, string(types.IPSetIPv6), "inet6", allV6); err != nil {
			return fmt.Errorf("apply v6: %w", err)
		}
	}

	names := []string{string(types.IPSetIPv4), string(types.IPSetIPv6)}
	if err := firewall.SaveIPSet(ctx, runner, firewall.DefaultDumpFile, names); err != nil {
		return fmt.Errorf("save dump: %w", err)
	}
	log.L().Info("ipset заполнены и сохранены", "v4", len(allV4), "v6", len(allV6))
	return nil
}

func handleUpdateCore(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		arch, err := types.DetectHostArch()
		if err != nil {
			return fmt.Errorf("--update-core: %w", err)
		}
		fmt.Printf("Загрузка sing-box (arch=%s)...\n", arch)
		res, err := singbox.Download(ctx, arch, "/tmp")
		if err != nil {
			return fmt.Errorf("--update-core: %w", err)
		}
		if !res.Downloaded {
			fmt.Println("sing-box уже актуален.")
			return nil
		}
		if err := singbox.Install(ctx, newRunner(), res.Path, singbox.DefaultBinPath, configPath()); err != nil {
			return fmt.Errorf("--update-core: %w", err)
		}
		fmt.Printf("sing-box обновлён до %s. Перезапустите сервис: sign-craze --restart\n", res.Version)
		return nil
	})
}

package cli

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/geo"
	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
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
		return nil
	})
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

package cli

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
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
	Register(Cmd{Long: "--update-core", Help: "обновить активное прокси-ядро", Handler: handleUpdateCore})
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
//
// Если хотя бы один .srs обработан успешно — продолжаем; если ВСЕ декомпиляции
// провалились (декомпилятор не работает или sing-box отсутствует) — error,
// иначе ipset молча останется пустым и geo-фильтрация не заработает.
func populateAndSaveIPSet(ctx context.Context, fileNames []string) error {
	runner := exectx.OS
	var allV4, allV6 []netip.Prefix
	srsTotal, srsOK := 0, 0

	for _, name := range fileNames {
		if !strings.HasSuffix(name, ".srs") {
			continue
		}
		srsTotal++
		srsPath := filepath.Join(geo.DefaultGeoDir, name)
		prefixes, err := geo.DecompileSRS(ctx, runner, singbox.DefaultBinPath, srsPath)
		if err != nil {
			log.L().Debug("decompile пропущен", "file", name, "err", err)
			continue
		}
		srsOK++
		v4, v6 := geo.SplitByFamily(prefixes)
		allV4 = append(allV4, v4...)
		allV6 = append(allV6, v6...)
	}

	if srsTotal > 0 && srsOK == 0 {
		return fmt.Errorf("decompile: 0/%d .srs файлов обработано (проверьте /opt/sbin/sing-box)", srsTotal)
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
		c := mustActiveCore()
		arch, err := types.DetectHostArch()
		if err != nil {
			return fmt.Errorf("--update-core: %w", err)
		}
		// Cache на /opt — на /tmp tmpfs Keenetic ~50MB, ~12MB бинарь не влезает.
		if mkErr := os.MkdirAll(c.CacheDir(), 0o755); mkErr != nil {
			return fmt.Errorf("--update-core: mkdir cache: %w", mkErr)
		}
		fmt.Printf("Загрузка %s (arch=%s)...\n", c.Name(), arch)
		res, err := c.Download(ctx, arch, c.CacheDir())
		if err != nil {
			return fmt.Errorf("--update-core: %w", err)
		}
		if !res.Downloaded {
			fmt.Printf("%s уже актуален.\n", c.Name())
			return nil
		}

		// Валидируем новый бинарь с текущим конфигом ДО замены.
		// Sing-box использует специализированный PrepareAndValidate (атомарный
		// swap во временный путь + sing-box check). Для xray/mihomo: общий путь
		// Install → CheckConfig (бинарь уже валидирован Install).
		if c.Name() == "sing-box" {
			st, err := loadState()
			if err != nil {
				return fmt.Errorf("--update-core: state: %w", err)
			}
			params := singboxParamsForInstall(st)

			tempBin, err := singbox.PrepareAndValidate(ctx, exectx.OS, singbox.DefaultCacheDir, res.Path, configPath(), params)
			if err != nil {
				return fmt.Errorf("--update-core: %w", err)
			}
			defer os.RemoveAll(filepath.Dir(tempBin))

			// Стримим бинарь, чтобы не держать ~12MB в Go heap (см. cmd_install.go).
			binFile, err := os.Open(tempBin)
			if err != nil {
				return fmt.Errorf("--update-core: открытие валидированного бинаря: %w", err)
			}
			_, err = atomicfs.BackupAndReplaceFromReader(singbox.DefaultBinPath, binFile, 0o755)
			_ = binFile.Close()
			if err != nil {
				return fmt.Errorf("--update-core: установка бинаря: %w", err)
			}
		} else {
			if err := c.Install(ctx, exectx.OS, res.Path); err != nil {
				return fmt.Errorf("--update-core: установка %s: %w", c.Name(), err)
			}
			if err := c.CheckConfig(ctx, exectx.OS, c.ConfigPath()); err != nil {
				return fmt.Errorf("--update-core: проверка конфига %s: %w", c.Name(), err)
			}
		}

		fmt.Printf("%s обновлён до %s. Перезапустите сервис: sign-craze --restart\n", c.Name(), res.Version)
		return nil
	})
}

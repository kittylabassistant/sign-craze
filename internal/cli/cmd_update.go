package cli

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/geo"
	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/naiveproxy"
	"github.com/kittylabassistant/sign-craze/internal/selfupdate"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func init() {
	Register(Cmd{Short: "-u", Long: "--update", Help: "обновить sign-craze", Handler: handleUpdate})
	Register(Cmd{Short: "-g", Long: "--update-geo", Help: "обновить geo-файлы", Handler: handleUpdateGeo})
	Register(Cmd{Long: "--update-core", Help: "обновить активное прокси-ядро", Handler: handleUpdateCore})
	Register(Cmd{Long: "--update-naive", Help: "обновить бинарь naiveproxy", Handler: handleUpdateNaive})
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
		fmt.Printf("%s до %s\n", OK("sign-craze обновлён"), ver)
		return nil
	})
}

// geoDownloadDATFn — точка подмены geo.DownloadDAT для тестов
// (cmd_update_test.go, cmd_core_test.go). В проде — geo.DownloadDAT, тесты
// подставляют fake без сети (тот же паттерн, что coreLifecycleFn/uiLifecycleFn
// в cmd_lifecycle.go).
var geoDownloadDATFn = geo.DownloadDAT

func handleUpdateGeo(ctx context.Context, args []string) error {
	return withLock(ctx, func() error {
		c, err := coreFromArgsOrActive(args)
		if err != nil {
			return fmt.Errorf("--update-geo: %w", err)
		}
		if err := updateGeoForCore(ctx, c); err != nil {
			return fmt.Errorf("--update-geo: %w", err)
		}
		return nil
	})
}

// coreFromArgsOrActive парсит необязательный флаг `--core <name>` из args и
// возвращает соответствующее ядро; при отсутствии флага — активное ядро
// (mustActiveCore, т.е. state.Core). Флаг НЕ мутирует state.Core — это
// разовый override для конкретного вызова (персистентная смена — отдельная
// команда `--core <name>`, см. cmd_core.go).
//
// Формат соответствует документированному в BEHAVIOR_SPEC.md/TROUBLESHOOTING.md:
// `sign-craze --update-geo --core xray`.
func coreFromArgsOrActive(args []string) (core.Core, error) {
	for i, a := range args {
		if a != "--core" {
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("--core: требуется имя ядра")
		}
		return core.Get(args[i+1])
	}
	return mustActiveCore(), nil
}

// updateGeoForCore выполняет --update-geo для конкретного, уже разрешённого
// ядра c. Вынесено из handleUpdateGeo отдельной функцией, чтобы её можно было
// юнит-тестировать без withLock/state.json (см. cmd_update_test.go).
//
// Ветвление по Core.GeoFormat() — единый механизм для всех ядер, без
// хардкода имени ядра:
//   - GeoDAT (xray)   — скачивание geosite.dat/geoip.dat через geo.DownloadDAT
//     в <c.ConfigDir()>/assets (тот же путь, что coreImpl.RenderConfig
//     проставляет в GeoAssetsDir — internal/core/xray/coreadapter.go).
//   - GeoMRS (mihomo) — no-op: mihomo качает rule-providers сам.
//   - GeoSRS (sing-box, default) — текущее поведение без изменений: манифест
//     sign-craze-dat + geo.Update + заполнение kernel ipset.
func updateGeoForCore(ctx context.Context, c core.Core) error {
	switch c.GeoFormat() {
	case core.GeoMRS:
		fmt.Printf("%s %s\n", Info("mihomo качает rule-providers сам"), Hint("(--update-geo не требуется для .mrs)"))
		log.L().Info("--update-geo: mihomo управляет geo-данными самостоятельно", "core", c.Name())
		return nil

	case core.GeoDAT:
		dstDir := filepath.Join(c.ConfigDir(), "assets")
		updated, err := geoDownloadDATFn(ctx, c.CacheDir(), dstDir)
		if err != nil {
			return err
		}
		fmt.Printf("%s %d/%d geo-файлов (.dat) для %s → %s\n",
			OK("Скачано"), updated, len(geo.DATFileNames), c.Name(), Hint(dstDir))
		return nil

	default: // core.GeoSRS — sing-box, текущее поведение без изменений
		manifest, err := geo.FetchManifest(ctx)
		if err != nil {
			return fmt.Errorf("manifest: %w", err)
		}
		needed := make([]string, 0, len(manifest.Files))
		for _, f := range manifest.Files {
			needed = append(needed, f.Name)
		}
		count, err := geo.Update(ctx, needed, geo.DefaultGeoDir)
		if err != nil {
			return err
		}
		fmt.Printf("%s %d/%d geo-файлов\n", OK("Скачано"), count, len(needed))

		// Заполнить kernel ipset из .srs и сохранить дамп для restore при ребуте
		// (safety-fixes #17). Если sing-box ещё не установлен или decompile падает —
		// логируем и продолжаем: gen-update остаётся идемпотентным.
		if err := populateAndSaveIPSet(ctx, needed); err != nil {
			log.L().Warn("--update-geo: заполнение ipset пропущено", "err", err)
		}
		return nil
	}
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
		fmt.Printf("%s %s (arch=%s)...\n", Info("Загрузка"), c.Name(), arch)
		res, err := c.Download(ctx, arch, c.CacheDir())
		if err != nil {
			return fmt.Errorf("--update-core: %w", err)
		}
		if !res.Downloaded {
			fmt.Printf("%s %s\n", OK(c.Name()), Hint("уже актуален."))
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
			if err := installValidatedSingboxBinary(ctx, res.Path, st); err != nil {
				return fmt.Errorf("--update-core: %w", err)
			}
		} else {
			if err := c.Install(ctx, exectx.OS, res.Path); err != nil {
				return fmt.Errorf("--update-core: установка %s: %w", c.Name(), err)
			}
			if err := c.CheckConfig(ctx, exectx.OS, c.ConfigPath()); err != nil {
				return fmt.Errorf("--update-core: проверка конфига %s: %w", c.Name(), err)
			}
		}

		fmt.Printf("%s до %s. %s\n", OK(c.Name()+" обновлён"), res.Version, Hint("Перезапустите сервис: sign-craze --restart"))
		return nil
	})
}

func handleUpdateNaive(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		arch, err := types.DetectHostArch()
		if err != nil {
			return fmt.Errorf("--update-naive: %w", err)
		}
		if mkErr := os.MkdirAll(naiveproxy.DefaultCacheDir, 0o755); mkErr != nil {
			return fmt.Errorf("--update-naive: mkdir cache: %w", mkErr)
		}
		res, err := naiveproxy.Download(ctx, arch, naiveproxy.DefaultCacheDir)
		if err != nil {
			return fmt.Errorf("--update-naive: %w", err)
		}
		if !res.Downloaded {
			log.L().Info("naive: актуальная версия уже установлена", "version", res.Version)
			fmt.Printf("%s %s\n", OK("naive"), Hint("уже актуален."))
			return nil
		}
		if err := naiveproxy.Install(res.Path, naiveproxy.DefaultBinPath); err != nil {
			return fmt.Errorf("--update-naive: установка: %w", err)
		}
		log.L().Info("naive обновлён", "version", res.Version)
		fmt.Printf("%s до %s. %s\n", OK("naive обновлён"), res.Version, Hint("Перезапустите сервис: sign-craze --restart"))
		return nil
	})
}

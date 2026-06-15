package mihomo

import (
	"context"
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// archAssetPrefix — префикс имени asset на GitHub releases MetaCubeX/mihomo.
//
// Структура имени: `mihomo-linux-{prefix}-vX.Y.Z.gz`.
//
// Особенности:
//   - amd64: используется default-вариант (`mihomo-linux-amd64-vX.Y.Z.gz`),
//     не v1/v2/v3 microarch-варианты. Они отдельные сборки под cpu features
//     и нужны редко.
//   - mips: используется `softfloat`-вариант (`mihomo-linux-mips-softfloat-vX.Y.Z.gz`),
//     совместимый со всеми Keenetic SoC. Аналог GOMIPS=softfloat в Go-сборке.
var archAssetPrefix = map[types.Arch]string{
	types.ArchARM64:  "mihomo-linux-arm64-",
	types.ArchARM7:   "mihomo-linux-armv7-",
	types.ArchMIPSLE: "mihomo-linux-mipsle-softfloat-",
	types.ArchMIPS:   "mihomo-linux-mips-softfloat-",
	types.ArchAMD64:  "mihomo-linux-amd64-",
}

// archForbiddenSubstrs — подстроки, при наличии которых asset должен быть
// отвергнут даже если префикс совпал. Mihomo релиз публикует тонну
// микроархитектурных вариантов amd64 (v1/v2/v3, compatible, go120/go122),
// и для arm64 — arm64-go124. Берём только default-вариант.
var archForbiddenSubstrs = map[types.Arch][]string{
	types.ArchAMD64: {
		"-amd64-compatible-",
		"-amd64-v1-",
		"-amd64-v2-",
		"-amd64-v3-",
		"-go120",
		"-go122",
		"-go123",
		"-go124",
	},
	types.ArchARM64: {
		"-arm64-go120",
		"-arm64-go122",
		"-arm64-go123",
		"-arm64-go124",
	},
}

// matchAsset выбирает asset для заданной архитектуры:
// - имя начинается с archAssetPrefix[arch];
// - имя оканчивается на ".gz" (исключаем .deb, .rpm, .pkg.tar.zst);
// - имя НЕ содержит подстрок из archForbiddenSubstrs[arch].
func matchAsset(arch types.Arch) func(types.Asset) bool {
	prefix := archAssetPrefix[arch]
	forbidden := archForbiddenSubstrs[arch]
	return func(a types.Asset) bool {
		if !strings.HasPrefix(a.Name, prefix) {
			return false
		}
		if !strings.HasSuffix(a.Name, ".gz") {
			return false
		}
		for _, f := range forbidden {
			if strings.Contains(a.Name, f) {
				return false
			}
		}
		return true
	}
}

// Download скачивает GZIP-архив mihomo для указанной архитектуры в dstDir.
// Внутри архива — ELF-бинарь напрямую (без TAR-обёртки).
func Download(ctx context.Context, arch types.Arch, dstDir string) (core.DownloadResult, error) {
	if err := arch.Validate(); err != nil {
		return core.DownloadResult{}, err
	}

	if _, ok := archAssetPrefix[arch]; !ok {
		return core.DownloadResult{}, fmt.Errorf("mihomo download: нет паттерна для архитектуры %q", arch)
	}

	res, err := ghrelease.New().Fetch(ctx, ghrelease.FetchOptions{
		Owner:      "MetaCubeX",
		Repo:       "mihomo",
		AssetMatch: matchAsset(arch),
		DstDir:     dstDir,
		// MetaCubeX публикует sha256sum.txt в релизе. Текущий verifySHA256
		// ищет `<asset>.sha256` (per-asset формат), которого у mihomo нет —
		// поэтому AllowMissingSHA=true до миграции на sha256sum.txt parser.
		VerifySHA:       true,
		AllowMissingSHA: true,
	})
	if err != nil {
		return core.DownloadResult{}, fmt.Errorf("mihomo download: %w", err)
	}

	return core.DownloadResult{
		Downloaded: res.Downloaded,
		Version:    res.Version,
		Path:       res.Path,
	}, nil
}

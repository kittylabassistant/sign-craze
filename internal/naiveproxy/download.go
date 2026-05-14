package naiveproxy

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// naiveAssetPattern — суффикс asset для каждой архитектуры в релизе
// klzgrad/naiveproxy. Используются "generic linux" тарболлы (а не openwrt-*
// варианты, требующие точного match SoC). Имена внутри tarball:
// "naiveproxy-v<ver>-<arch>/naive".
var naiveAssetPattern = map[types.Arch]string{
	types.ArchARM64:  "linux-arm64.tar.xz",
	types.ArchARM7:   "linux-arm.tar.xz",
	types.ArchMIPSLE: "linux-mipsel.tar.xz",
	types.ArchAMD64:  "linux-x64.tar.xz",
	// ArchMIPS (big-endian) ОТСУТСТВУЕТ — klzgrad публикует только LE.
}

// DownloadResult описывает результат вызова Download.
type DownloadResult struct {
	Downloaded bool   // false если файл актуален (ETag совпал)
	Version    string // тег релиза, например "v148.0.7778.96-5"
	Path       string // путь к скачанному tarball
}

// DefaultCacheDir — постоянный каталог под кеш tarball'ов naiveproxy.
const DefaultCacheDir = "/opt/var/lib/sign-craze/cache/naive"

// Download скачивает актуальный tarball naiveproxy для указанной архитектуры в dstDir.
// При совпадении ETag повторная загрузка не производится.
// Возвращает явную ошибку для arch=mips (big-endian) — naive не публикует BE.
func Download(ctx context.Context, arch types.Arch, dstDir string) (DownloadResult, error) {
	if err := arch.Validate(); err != nil {
		return DownloadResult{}, err
	}
	if arch == types.ArchMIPS {
		return DownloadResult{}, fmt.Errorf("naiveproxy download: архитектура mips (big-endian) не поддерживается upstream klzgrad/naiveproxy (публикует только linux-mipsel)")
	}
	pattern, ok := naiveAssetPattern[arch]
	if !ok {
		return DownloadResult{}, fmt.Errorf("naiveproxy download: нет паттерна для архитектуры %q", arch)
	}

	res, err := ghrelease.New().Fetch(ctx, ghrelease.FetchOptions{
		Owner:      "klzgrad",
		Repo:       "naiveproxy",
		AssetMatch: ghrelease.MatchByContains(pattern),
		DstDir:     dstDir,
		// klzgrad публикует SHA256 встроенный в GitHub Release JSON digest field
		// (digest "sha256:..."), но не как отдельный .sha256 asset. ghrelease
		// не использует digest на текущий момент — AllowMissingSHA=true с WARN.
		VerifySHA:       true,
		AllowMissingSHA: true,
	})
	if err != nil {
		return DownloadResult{}, fmt.Errorf("naiveproxy download: %w", err)
	}

	return DownloadResult{
		Downloaded: res.Downloaded,
		Version:    res.Version,
		Path:       res.Path,
	}, nil
}

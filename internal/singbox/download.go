package singbox

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// assetPattern — подстрока в имени asset, идентифицирующая нужную архитектуру.
var assetPattern = map[types.Arch]string{
	types.ArchARM64:  "linux-arm64.tar.gz",
	types.ArchARM7:   "linux-armv7.tar.gz",
	types.ArchMIPSLE: "linux-mipsle-softfloat.tar.gz",
	types.ArchMIPS:   "linux-mips-softfloat.tar.gz",
	types.ArchAMD64:  "linux-amd64.tar.gz",
}

// DownloadResult описывает результат вызова Download.
type DownloadResult struct {
	Downloaded bool   // false если файл актуален (ETag совпал)
	Version    string // тег релиза, например "v1.10.0"
	Path       string // путь к скачанному файлу
}

// Download скачивает актуальный tarball sing-box для указанной архитектуры в директорию dst.
// При совпадении ETag повторная загрузка не производится.
func Download(ctx context.Context, arch types.Arch, dstDir string) (DownloadResult, error) {
	if err := arch.Validate(); err != nil {
		return DownloadResult{}, err
	}

	pattern, ok := assetPattern[arch]
	if !ok {
		return DownloadResult{}, fmt.Errorf("singbox download: нет паттерна для архитектуры %q", arch)
	}

	res, err := ghrelease.New().Fetch(ctx, ghrelease.FetchOptions{
		Owner:      "SagerNet",
		Repo:       "sing-box",
		AssetMatch: ghrelease.MatchByContains(pattern),
		DstDir:     dstDir,
	})
	if err != nil {
		return DownloadResult{}, fmt.Errorf("singbox download: %w", err)
	}

	return DownloadResult{
		Downloaded: res.Downloaded,
		Version:    res.Version,
		Path:       res.Path,
	}, nil
}

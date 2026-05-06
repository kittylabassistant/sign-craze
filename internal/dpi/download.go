package dpi

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// nfqws2AssetPattern — суффикс asset для каждой архитектуры в релизе nfqws2-keenetic.
// Начиная с v1.1.5 релизы публикуются как Entware .ipk пакеты (outer tar.gz → data.tar.gz → бинарь).
var nfqws2AssetPattern = map[types.Arch]string{
	types.ArchARM64:  "aarch64-3.10.ipk",
	types.ArchARM7:   "armv7-3.2.ipk",
	types.ArchMIPSLE: "mipsel-3.4.ipk",
	types.ArchMIPS:   "mips-3.4.ipk",
	types.ArchAMD64:  "x86_64-3.2.ipk",
}

// DownloadResult описывает результат вызова Download.
type DownloadResult struct {
	Downloaded bool   // false если файл актуален (ETag совпал)
	Version    string // тег релиза
	Path       string // путь к скачанному tarball
}

// Download скачивает актуальный tarball nfqws2-keenetic для указанной архитектуры в dstDir.
// При совпадении ETag повторная загрузка не производится.
func Download(ctx context.Context, arch types.Arch, dstDir string) (DownloadResult, error) {
	if err := arch.Validate(); err != nil {
		return DownloadResult{}, err
	}

	pattern, ok := nfqws2AssetPattern[arch]
	if !ok {
		return DownloadResult{}, fmt.Errorf("dpi download: нет паттерна для архитектуры %q", arch)
	}

	res, err := ghrelease.New().Fetch(ctx, ghrelease.FetchOptions{
		Owner:      "nfqws",
		Repo:       "nfqws2-keenetic",
		AssetMatch: ghrelease.MatchByContains(pattern),
		DstDir:     dstDir,
	})
	if err != nil {
		return DownloadResult{}, fmt.Errorf("dpi download: %w", err)
	}

	return DownloadResult{
		Downloaded: res.Downloaded,
		Version:    res.Version,
		Path:       res.Path,
	}, nil
}

package singbox

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/core"
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

// Download скачивает актуальный tarball sing-box для указанной архитектуры в директорию dst.
// При совпадении ETag повторная загрузка не производится.
func Download(ctx context.Context, arch types.Arch, dstDir string) (core.DownloadResult, error) {
	if err := arch.Validate(); err != nil {
		return core.DownloadResult{}, err
	}

	pattern, ok := assetPattern[arch]
	if !ok {
		return core.DownloadResult{}, fmt.Errorf("singbox download: нет паттерна для архитектуры %q", arch)
	}

	res, err := ghrelease.New().Fetch(ctx, ghrelease.FetchOptions{
		Owner:      "SagerNet",
		Repo:       "sing-box",
		AssetMatch: ghrelease.MatchByContains(pattern),
		DstDir:     dstDir,
		// SagerNet sing-box историчесĸи НЕ публикует <asset>.sha256 в релизах
		// (проверено на v1.13.11 — gh api repos/SagerNet/sing-box/releases/tags/...
		// не содержит .sha256 / SHA256SUMS / signature файлов).
		// AllowMissingSHA=true: integrity-проверка пропускается с WARN.
		// Защита от truncated download остаётся через manifest Size check
		// в ghrelease.downloadFromMirror (resp.Size != asset.Size → ошибка).
		// От targeted MITM не защищает — но sing-box качается через GitHub
		// HTTPS, для RCE-attack нужно компрометировать SagerNet/GitHub TLS.
		VerifySHA:       true,
		AllowMissingSHA: true,
	})
	if err != nil {
		return core.DownloadResult{}, fmt.Errorf("singbox download: %w", err)
	}

	return core.DownloadResult{
		Downloaded: res.Downloaded,
		Version:    res.Version,
		Path:       res.Path,
	}, nil
}

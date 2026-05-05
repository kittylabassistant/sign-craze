package xray

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// assetPattern — подстрока в имени asset на GitHub releases XTLS/Xray-core,
// идентифицирующая нужную архитектуру.
//
// Соответствие проверено на релизах XTLS/Xray-core (см. `gh api
// repos/XTLS/Xray-core/releases/latest`):
//
//	arm64  → Xray-linux-arm64-v8a.zip
//	arm7   → Xray-linux-arm32-v7a.zip
//	mipsle → Xray-linux-mips32le.zip
//	mips   → Xray-linux-mips32.zip
//	amd64  → Xray-linux-64.zip
var assetPattern = map[types.Arch]string{
	types.ArchARM64:  "linux-arm64-v8a.zip",
	types.ArchARM7:   "linux-arm32-v7a.zip",
	types.ArchMIPSLE: "linux-mips32le.zip",
	types.ArchMIPS:   "linux-mips32.zip",
	types.ArchAMD64:  "linux-64.zip",
}

// DownloadResult описывает результат вызова Download.
type DownloadResult struct {
	Downloaded bool   // false если файл актуален (ETag совпал)
	Version    string // тег релиза, например "v1.8.4"
	Path       string // путь к скачанному zip-файлу
}

// Download скачивает актуальный zip-архив xray для указанной архитектуры.
// При совпадении ETag повторная загрузка не производится.
//
// Возвращает путь к zip — распаковка делается отдельно через Install
// (см. install.go). Это позволяет ghrelease ETag-кешировать без повторной
// распаковки на каждый --update-core.
func Download(ctx context.Context, arch types.Arch, dstDir string) (DownloadResult, error) {
	if err := arch.Validate(); err != nil {
		return DownloadResult{}, err
	}

	pattern, ok := assetPattern[arch]
	if !ok {
		return DownloadResult{}, fmt.Errorf("xray download: нет паттерна для архитектуры %q", arch)
	}

	res, err := ghrelease.New().Fetch(ctx, ghrelease.FetchOptions{
		Owner:      "XTLS",
		Repo:       "Xray-core",
		AssetMatch: ghrelease.MatchByContains(pattern),
		DstDir:     dstDir,
	})
	if err != nil {
		return DownloadResult{}, fmt.Errorf("xray download: %w", err)
	}

	return DownloadResult{
		Downloaded: res.Downloaded,
		Version:    res.Version,
		Path:       res.Path,
	}, nil
}

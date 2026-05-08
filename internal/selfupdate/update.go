package selfupdate

import (
	"context"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// archAssetName возвращает имя asset релиза sign-craze для указанной архитектуры.
// Совпадает с матрицей в .github/workflows/release.yml.
func archAssetName(arch types.Arch) (string, error) {
	switch arch {
	case types.ArchARM64:
		return "sign-craze-arm64", nil
	case types.ArchARM7:
		return "sign-craze-arm7", nil
	case types.ArchMIPSLE:
		return "sign-craze-mipsle", nil
	case types.ArchMIPS:
		return "sign-craze-mips", nil
	case types.ArchAMD64:
		return "sign-craze-amd64", nil
	default:
		return "", fmt.Errorf("selfupdate: нет asset для архитектуры %q", arch)
	}
}

// Options описывает параметры обновления.
type Options struct {
	Owner   string // "kittylabassistant"
	Repo    string // "sign-craze"
	Arch    types.Arch
	BinPath string // /opt/sbin/sign-craze
	DstDir  string // временная директория для скачивания (по умолчанию os.TempDir())
}

// Update скачивает последний релиз sign-craze, проверяет SHA256 и заменяет бинарь.
// Возвращает версию нового релиза.
func Update(ctx context.Context, d *ghrelease.Downloader, opts Options) (string, error) {
	if opts.Owner == "" {
		opts.Owner = "kittylabassistant"
	}
	if opts.Repo == "" {
		opts.Repo = "sign-craze"
	}
	if opts.DstDir == "" {
		opts.DstDir = os.TempDir()
	}
	if opts.BinPath == "" {
		opts.BinPath = "/opt/sbin/sign-craze"
	}

	assetName, err := archAssetName(opts.Arch)
	if err != nil {
		return "", err
	}

	if d == nil {
		d = ghrelease.New()
	}

	res, err := d.Fetch(ctx, ghrelease.FetchOptions{
		Owner:      opts.Owner,
		Repo:       opts.Repo,
		AssetMatch: func(a types.Asset) bool { return a.Name == assetName },
		DstDir:     opts.DstDir,
		VerifySHA:  true,
	})
	if err != nil {
		return "", fmt.Errorf("selfupdate: %w", err)
	}

	// Streaming-замена: BackupAndReplaceFromReader использует io.Copy с
	// 8KB-буфером + atomic rename, не загружая весь бинарь (~10MB) в RAM.
	// На 128MB-роутере прежний os.ReadFile + BackupAndReplace создавал
	// два буфера в RAM одновременно (≈20MB) — риск OOM.
	src, err := os.Open(res.Path)
	if err != nil {
		return "", fmt.Errorf("selfupdate: открытие скачанного: %w", err)
	}
	defer func() { _ = src.Close() }()

	if _, err := atomicfs.BackupAndReplaceFromReader(opts.BinPath, src, 0o755); err != nil {
		return "", fmt.Errorf("selfupdate: замена %s: %w", opts.BinPath, err)
	}

	log.L().Info("selfupdate: бинарь обновлён", "version", res.Version, "path", opts.BinPath)
	return res.Version, nil
}

package mihomo

import (
	"compress/gzip"
	"context"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/core/corearchive"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// Install устанавливает бинарь mihomo из gzip-архива в binDst.
//
//  1. Распаковка gzip → потоковый ELF-check.
//  2. Backup существующего бинаря и атомарная подмена.
//  3. (опц.) Валидация config через `mihomo -t`.
//  4. При ошибке валидации — откат backup'а.
//
// Особенность mihomo: gzip содержит ELF-бинарь напрямую, без TAR-обёртки.
// Общая рамка (backup → запись → опциональная валидация конфига → откат)
// вынесена в corearchive.InstallWithRollback — здесь только специфика
// формата архива.
func Install(ctx context.Context, runner exectx.Runner, gzPath, binDst, configPath string) error {
	var checkConfig func(context.Context) error
	if configPath != "" {
		checkConfig = func(ctx context.Context) error {
			return CheckConfig(ctx, runner, binDst, configPath)
		}
	}

	return corearchive.InstallWithRollback(ctx, corearchive.InstallParams{
		Label:       "mihomo",
		ArchivePath: gzPath,
		BinDst:      binDst,
		Opener: func() (*corearchive.BinaryStream, error) {
			stream, err := openMihomoBinaryStream(gzPath)
			if err != nil {
				return nil, fmt.Errorf("распаковка gzip: %w", err)
			}
			return stream, nil
		},
		CheckConfig: checkConfig,
	})
}

// openMihomoBinaryStream открывает gzip-файл, проверяет ELF-magic первых
// 4 байт (corearchive.CheckELF) и возвращает stream на полное содержимое
// (включая magic).
//
// Не буферизует бинарь в RAM: первые 4 байта читаются для magic-check,
// остаток стримится через io.MultiReader. Критично для роутеров с 128MB.
func openMihomoBinaryStream(gzPath string) (*corearchive.BinaryStream, error) {
	f, err := os.Open(gzPath)
	if err != nil {
		return nil, fmt.Errorf("открытие gzip: %w", err)
	}

	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("gzip reader: %w", err)
	}

	full, err := corearchive.CheckELF(gz, "содержимое gzip")
	if err != nil {
		_ = gz.Close()
		_ = f.Close()
		return nil, fmt.Errorf("mihomo install: %w", err)
	}
	closer := func() error {
		_ = gz.Close()
		return f.Close()
	}
	return corearchive.NewBinaryStream(full, closer), nil
}

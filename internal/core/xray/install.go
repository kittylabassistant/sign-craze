package xray

import (
	"archive/zip"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/core/corearchive"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// Install устанавливает бинарь xray из zip-архива в binDst.
//
//  1. Распаковка zip → потоковый ELF-check.
//  2. Backup существующего бинаря и атомарная подмена.
//  3. Валидация config через `xray test` (если configPath != "").
//  4. При ошибке валидации — откат backup'а.
//
// Общая рамка (backup → запись → опциональная валидация конфига → откат)
// вынесена в corearchive.InstallWithRollback — здесь только специфика
// формата архива: zip, не tar.gz/plain gzip (см. singbox/mihomo).
// Алгоритм симметричен singbox.Install по духу, но архивы принципиально
// разные (см. пакет corearchive.doc.go про архитектурное отличие singbox).
func Install(ctx context.Context, runner exectx.Runner, zipPath, binDst, configPath string) error {
	var checkConfig func(context.Context) error
	if configPath != "" {
		checkConfig = func(ctx context.Context) error {
			return CheckConfig(ctx, runner, binDst, configPath)
		}
	}

	return corearchive.InstallWithRollback(ctx, corearchive.InstallParams{
		Label:       "xray",
		ArchivePath: zipPath,
		BinDst:      binDst,
		Opener: func() (*corearchive.BinaryStream, error) {
			stream, err := openXrayBinaryStream(zipPath)
			if err != nil {
				return nil, fmt.Errorf("распаковка zip: %w", err)
			}
			return stream, nil
		},
		CheckConfig: checkConfig,
	})
}

// openXrayBinaryStream открывает zip, ищет файл с базовым именем "xray",
// проверяет ELF-magic первых 4 байт (corearchive.CheckELF) и возвращает
// stream на полное содержимое.
//
// Не загружает бинарь в RAM целиком: первые 4 байта читаются для magic-check,
// затем io.MultiReader склеивает их с остатком zip-стрима. Критично для
// роутеров с 128MB.
//
// Особенность zip vs tar: archive/zip требует ReaderAt и общий размер файла,
// поэтому мы открываем zip через zip.OpenReader (file-based, без буферизации
// в RAM). io.ReadCloser файла внутри zip — потоковый.
func openXrayBinaryStream(zipPath string) (*corearchive.BinaryStream, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("открытие zip: %w", err)
	}

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if base != "xray" && !strings.HasSuffix(f.Name, "/xray") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			_ = r.Close()
			return nil, fmt.Errorf("открытие %s в zip: %w", f.Name, err)
		}

		full, err := corearchive.CheckELF(rc, f.Name)
		if err != nil {
			_ = rc.Close()
			_ = r.Close()
			return nil, fmt.Errorf("xray install: %w", err)
		}
		closer := func() error {
			_ = rc.Close()
			return r.Close()
		}
		return corearchive.NewBinaryStream(full, closer), nil
	}

	_ = r.Close()
	return nil, fmt.Errorf("бинарь 'xray' не найден в архиве %s", zipPath)
}

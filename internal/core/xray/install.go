package xray

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/elfcheck"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// Install устанавливает бинарь xray из zip-архива в binDst.
//
//  1. Распаковка zip → temp-файл.
//  2. Валидация config через `xray test` (если configPath != "").
//  3. Backup существующего бинаря и атомарная подмена.
//  4. При ошибке валидации — откат backup'а.
//
// Алгоритм симметричен singbox.Install, но архив — zip, не tar.gz.
// Внутри zip единственный регулярный файл `xray` (без вложенной директории).
func Install(ctx context.Context, runner exectx.Runner, zipPath, binDst, configPath string) error {
	log.L().Info("установка xray", "zip", zipPath, "dst", binDst)

	stream, err := openXrayBinaryStream(zipPath)
	if err != nil {
		return fmt.Errorf("xray install: распаковка zip: %w", err)
	}
	defer stream.Close()

	backupPath, err := atomicfs.BackupAndReplaceFromReader(binDst, stream.Reader, 0o755)
	if err != nil {
		return fmt.Errorf("xray install: запись бинаря: %w", err)
	}

	if configPath != "" {
		if err := CheckConfig(ctx, runner, binDst, configPath); err != nil {
			log.L().Warn("xray: конфиг невалиден, откат бинаря", "backup", backupPath, "err", err)
			if backupPath != "" {
				if restoreErr := atomicfs.RestoreBackup(backupPath, binDst); restoreErr != nil {
					return fmt.Errorf("xray install: конфиг невалиден И откат не удался: %w (откат: %w)", err, restoreErr)
				}
			}
			return fmt.Errorf("xray install: проверка конфига: %w", err)
		}
	}

	log.L().Info("xray установлен", "path", binDst)
	return nil
}

// binaryStream — потоковый ридер на содержимое xray-бинаря из zip.
type binaryStream struct {
	Reader io.Reader
	closer func() error
}

func (b *binaryStream) Close() error { return b.closer() }

// openXrayBinaryStream открывает zip, ищет файл с базовым именем "xray",
// проверяет ELF-magic первых 4 байт и возвращает stream на полное содержимое.
//
// Не загружает бинарь в RAM целиком: первые 4 байта читаются для magic-check,
// затем io.MultiReader склеивает их с остатком zip-стрима. Критично для
// роутеров с 128MB.
//
// Особенность zip vs tar: archive/zip требует ReaderAt и общий размер файла,
// поэтому мы открываем zip через zip.OpenReader (file-based, без буферизации
// в RAM). io.ReadCloser файла внутри zip — потоковый.
func openXrayBinaryStream(zipPath string) (*binaryStream, error) {
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

		// ELF-magic check.
		full, got, n, readErr := elfcheck.CheckAndRewind(rc)
		if readErr != nil {
			_ = rc.Close()
			_ = r.Close()
			return nil, fmt.Errorf("чтение ELF-magic из %s: %w", f.Name, readErr)
		}
		if !elfcheck.IsELF(got, n) {
			_ = rc.Close()
			_ = r.Close()
			return nil, fmt.Errorf("xray install: %s не ELF-бинарь (magic=%x)", f.Name, got[:n])
		}
		closer := func() error {
			_ = rc.Close()
			return r.Close()
		}
		return &binaryStream{Reader: full, closer: closer}, nil
	}

	_ = r.Close()
	return nil, fmt.Errorf("бинарь 'xray' не найден в архиве %s", zipPath)
}

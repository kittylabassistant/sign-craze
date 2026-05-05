package xray

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// elfMagic — первые 4 байта Linux ELF-бинаря (\x7f E L F).
var elfMagic = []byte{0x7f, 'E', 'L', 'F'}

// PrepareAndValidate распаковывает бинарь xray из zip во временный файл,
// проверяет ELF-magic и (если configPath не пустой) валидирует config через
// `xray test -c`. Возвращает путь к временному бинарю с правами 0755.
//
// Caller обязан после успешной валидации сделать atomic move temp → final
// или удалить tmpDir при отказе. Конфиг в configPath остаётся как есть.
//
// workDir — родительская директория для temp-папки распаковки. Должна быть
// на постоянной FS (см. DefaultCacheDir): xray ~16MB, /tmp на Keenetic
// — tmpfs ~50MB и часто переполнен.
func PrepareAndValidate(ctx context.Context, runner exectx.Runner, workDir, zipPath, configPath string) (tempBinPath string, err error) {
	tmpDir, err := os.MkdirTemp(workDir, "sign-craze-xray-install-*")
	if err != nil {
		return "", fmt.Errorf("xray prepare: tempdir: %w", err)
	}
	tempBin := filepath.Join(tmpDir, "xray")
	if err := extractBinaryToFile(zipPath, tempBin, 0o755); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("xray prepare: распаковка: %w", err)
	}

	if configPath != "" {
		if err := CheckConfig(ctx, runner, tempBin, configPath); err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", fmt.Errorf("xray prepare: валидация конфига: %w", err)
		}
	}

	return tempBin, nil
}

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
		magic := make([]byte, len(elfMagic))
		n, readErr := io.ReadFull(rc, magic)
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			_ = rc.Close()
			_ = r.Close()
			return nil, fmt.Errorf("чтение ELF-magic из %s: %w", f.Name, readErr)
		}
		if n < len(elfMagic) || !bytes.Equal(magic[:n], elfMagic) {
			_ = rc.Close()
			_ = r.Close()
			return nil, fmt.Errorf("xray install: %s не ELF-бинарь (magic=%x)", f.Name, magic[:n])
		}

		full := io.MultiReader(bytes.NewReader(magic), rc)
		closer := func() error {
			_ = rc.Close()
			return r.Close()
		}
		return &binaryStream{Reader: full, closer: closer}, nil
	}

	_ = r.Close()
	return nil, fmt.Errorf("бинарь 'xray' не найден в архиве %s", zipPath)
}

// extractBinaryToFile стримит бинарь xray из zip напрямую в dstPath.
func extractBinaryToFile(zipPath, dstPath string, perm os.FileMode) error {
	stream, err := openXrayBinaryStream(zipPath)
	if err != nil {
		return err
	}
	defer stream.Close()

	if err := atomicfs.WriteFileAtomicFromReader(dstPath, stream.Reader, perm); err != nil {
		return fmt.Errorf("запись бинаря: %w", err)
	}
	return nil
}

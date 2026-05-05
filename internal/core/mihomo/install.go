package mihomo

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// elfMagic — первые 4 байта Linux ELF-бинаря (\x7f E L F).
var elfMagic = []byte{0x7f, 'E', 'L', 'F'}

// PrepareAndValidate распаковывает бинарь mihomo из gzip во временный файл
// и валидирует через `mihomo -t -d <dir>`. Возвращает путь к временному
// бинарю с правами 0755.
//
// configPath может указывать на файл (yaml) — функция возьмёт его dir для
// `mihomo -t -d`.
func PrepareAndValidate(ctx context.Context, runner exectx.Runner, workDir, gzPath, configPath string) (tempBinPath string, err error) {
	tmpDir, err := os.MkdirTemp(workDir, "sign-craze-mihomo-install-*")
	if err != nil {
		return "", fmt.Errorf("mihomo prepare: tempdir: %w", err)
	}
	tempBin := filepath.Join(tmpDir, "mihomo")
	if err := extractBinaryToFile(gzPath, tempBin, 0o755); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("mihomo prepare: распаковка: %w", err)
	}

	if configPath != "" {
		if err := CheckConfig(ctx, runner, tempBin, configPath); err != nil {
			_ = os.RemoveAll(tmpDir)
			return "", fmt.Errorf("mihomo prepare: валидация конфига: %w", err)
		}
	}

	return tempBin, nil
}

// Install устанавливает бинарь mihomo из gzip-архива в binDst.
//
//  1. Распаковка gzip → temp-stream.
//  2. ELF-magic check.
//  3. Backup существующего бинаря и атомарная подмена.
//  4. (опц.) Валидация config через `mihomo -t`.
//  5. При ошибке валидации — откат backup'а.
//
// Особенность mihomo: gzip содержит ELF-бинарь напрямую, без TAR-обёртки.
func Install(ctx context.Context, runner exectx.Runner, gzPath, binDst, configPath string) error {
	log.L().Info("установка mihomo", "gz", gzPath, "dst", binDst)

	stream, err := openMihomoBinaryStream(gzPath)
	if err != nil {
		return fmt.Errorf("mihomo install: распаковка gzip: %w", err)
	}
	defer stream.Close()

	backupPath, err := atomicfs.BackupAndReplaceFromReader(binDst, stream.Reader, 0o755)
	if err != nil {
		return fmt.Errorf("mihomo install: запись бинаря: %w", err)
	}

	if configPath != "" {
		if err := CheckConfig(ctx, runner, binDst, configPath); err != nil {
			log.L().Warn("mihomo: конфиг невалиден, откат бинаря", "backup", backupPath, "err", err)
			if backupPath != "" {
				if restoreErr := atomicfs.RestoreBackup(backupPath, binDst); restoreErr != nil {
					return fmt.Errorf("mihomo install: конфиг невалиден И откат не удался: %w (откат: %w)", err, restoreErr)
				}
			}
			return fmt.Errorf("mihomo install: проверка конфига: %w", err)
		}
	}

	log.L().Info("mihomo установлен", "path", binDst)
	return nil
}

// binaryStream — потоковый ридер на содержимое mihomo-бинаря из gzip.
type binaryStream struct {
	Reader io.Reader
	closer func() error
}

func (b *binaryStream) Close() error { return b.closer() }

// openMihomoBinaryStream открывает gzip-файл, проверяет ELF-magic первых
// 4 байт и возвращает stream на полное содержимое (включая magic).
//
// Не буферизует бинарь в RAM: первые 4 байта читаются для magic-check,
// остаток стримится через io.MultiReader. Критично для роутеров с 128MB.
func openMihomoBinaryStream(gzPath string) (*binaryStream, error) {
	f, err := os.Open(gzPath)
	if err != nil {
		return nil, fmt.Errorf("открытие gzip: %w", err)
	}

	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("gzip reader: %w", err)
	}

	magic := make([]byte, len(elfMagic))
	n, readErr := io.ReadFull(gz, magic)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		_ = gz.Close()
		_ = f.Close()
		return nil, fmt.Errorf("чтение ELF-magic: %w", readErr)
	}
	if n < len(elfMagic) || !bytes.Equal(magic[:n], elfMagic) {
		_ = gz.Close()
		_ = f.Close()
		return nil, fmt.Errorf("mihomo install: содержимое gzip не ELF-бинарь (magic=%x)", magic[:n])
	}

	full := io.MultiReader(bytes.NewReader(magic), gz)
	closer := func() error {
		_ = gz.Close()
		return f.Close()
	}
	return &binaryStream{Reader: full, closer: closer}, nil
}

// extractBinaryToFile стримит бинарь mihomo из gzip напрямую в dstPath.
func extractBinaryToFile(gzPath, dstPath string, perm os.FileMode) error {
	stream, err := openMihomoBinaryStream(gzPath)
	if err != nil {
		return err
	}
	defer stream.Close()

	if err := atomicfs.WriteFileAtomicFromReader(dstPath, stream.Reader, perm); err != nil {
		return fmt.Errorf("запись бинаря: %w", err)
	}
	return nil
}

package mihomo

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/elfcheck"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

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

	full, got, n, readErr := elfcheck.CheckAndRewind(gz)
	if readErr != nil {
		_ = gz.Close()
		_ = f.Close()
		return nil, fmt.Errorf("чтение ELF-magic: %w", readErr)
	}
	if !elfcheck.IsELF(got, n) {
		_ = gz.Close()
		_ = f.Close()
		return nil, fmt.Errorf("mihomo install: содержимое gzip не ELF-бинарь (magic=%x)", got[:n])
	}
	closer := func() error {
		_ = gz.Close()
		return f.Close()
	}
	return &binaryStream{Reader: full, closer: closer}, nil
}

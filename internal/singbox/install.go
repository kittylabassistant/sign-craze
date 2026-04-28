package singbox

import (
	"archive/tar"
	"compress/gzip"
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

const (
	// DefaultBinPath — путь установки бинаря sing-box.
	DefaultBinPath = "/opt/sbin/sing-box"
	// DefaultConfigDir — директория конфигурации.
	DefaultConfigDir = "/opt/etc/sign-craze"
)

// PrepareAndValidate распаковывает бинарь во временный файл и валидирует конфиг
// (sing-box check) ДО установки в финальный путь. Это защищает от ситуации
// "бинарь установлен, конфиг битый" (safety-fixes #10).
//
// Возвращает путь к временному бинарю (chmod 0755). Вызывающий обязан после
// успешной валидации сделать атомарный move temp → final binPath, либо удалить
// temp при отказе. Конфиг в configPath остаётся как есть (либо валиден, либо
// требует ручной коррекции/отката).
func PrepareAndValidate(ctx context.Context, runner exectx.Runner, tarPath, configPath string, params ConfigParams) (tempBinPath string, err error) {
	binData, err := extractBinary(tarPath)
	if err != nil {
		return "", fmt.Errorf("singbox prepare: распаковка: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "sign-craze-install-*")
	if err != nil {
		return "", fmt.Errorf("singbox prepare: tempdir: %w", err)
	}
	tempBin := filepath.Join(tmpDir, "sing-box")
	if err := os.WriteFile(tempBin, binData, 0o755); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("singbox prepare: запись tmp бинаря: %w", err)
	}

	// WriteConfig пишет конфиг и валидирует через `tempBin check -c configPath`.
	if err := WriteConfig(ctx, runner, params, tempBin, configPath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("singbox prepare: валидация конфига: %w", err)
	}

	return tempBin, nil
}

// Install устанавливает бинарь sing-box из tarball в binDst.
// Алгоритм:
//  1. Резервная копия текущего бинаря (если существует).
//  2. Распаковка tarball в temp-директорию.
//  3. Атомарная запись бинаря в binDst с правами 0755.
//  4. Проверка конфига через `sing-box check -c configPath` (если configPath не пустой).
//  5. При ошибке шага 4 — откат через RestoreBackup.
//
// Deprecated: для нового кода используйте PrepareAndValidate + atomicfs.BackupAndReplace,
// чтобы валидация конфига происходила ДО установки бинаря.
func Install(ctx context.Context, runner exectx.Runner, tarPath, binDst, configPath string) error {
	log.L().Info("установка sing-box", "tarball", tarPath, "dst", binDst)

	binData, err := extractBinary(tarPath)
	if err != nil {
		return fmt.Errorf("singbox install: распаковка tarball: %w", err)
	}

	backupPath, err := atomicfs.BackupAndReplace(binDst, binData, 0o755)
	if err != nil {
		return fmt.Errorf("singbox install: запись бинаря: %w", err)
	}

	if configPath != "" {
		if err := checkConfig(ctx, runner, binDst, configPath); err != nil {
			log.L().Warn("конфиг невалиден, откат бинаря", "backup", backupPath, "err", err)
			if backupPath != "" {
				if restoreErr := atomicfs.RestoreBackup(backupPath, binDst); restoreErr != nil {
					return fmt.Errorf("singbox install: конфиг невалиден И откат не удался: %w (откат: %w)", err, restoreErr)
				}
			}
			return fmt.Errorf("singbox install: проверка конфига: %w", err)
		}
	}

	log.L().Info("sing-box установлен", "path", binDst)
	return nil
}

// extractBinary распаковывает tarball и возвращает содержимое бинаря sing-box.
// Ищет файл с именем "sing-box" в любом подкаталоге архива.
func extractBinary(tarPath string) ([]byte, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("открытие tarball: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("чтение tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// ищем файл с именем "sing-box" (без расширения) в любом каталоге
		if filepath.Base(hdr.Name) == "sing-box" || strings.HasSuffix(hdr.Name, "/sing-box") {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("чтение бинаря из tar: %w", err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("бинарь 'sing-box' не найден в архиве %s", tarPath)
}

// checkConfig запускает `sing-box check -c configPath` для валидации конфига.
func checkConfig(ctx context.Context, runner exectx.Runner, binPath, configPath string) error {
	res, err := runner.Run(ctx, binPath, "check", "-c", configPath)
	if err != nil {
		return fmt.Errorf("sing-box check: %w (stderr: %s)", err, res.Stderr)
	}
	return nil
}

// Backup создаёт резервную копию бинаря по binPath.
// Возвращает путь к бэкапу или пустую строку если файл не существует.
func Backup(ctx context.Context, binPath string) (string, error) {
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return "", nil
	}
	data, err := os.ReadFile(binPath)
	if err != nil {
		return "", fmt.Errorf("singbox backup: чтение бинаря: %w", err)
	}
	backupPath, err := atomicfs.BackupAndReplace(binPath, data, 0o755)
	if err != nil {
		return "", fmt.Errorf("singbox backup: %w", err)
	}
	return backupPath, nil
}

// Restore восстанавливает бинарь из резервной копии.
func Restore(ctx context.Context, backupPath, binPath string) error {
	if err := atomicfs.RestoreBackup(backupPath, binPath); err != nil {
		return fmt.Errorf("singbox restore: %w", err)
	}
	return nil
}

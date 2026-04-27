package dpi

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

const (
	// DefaultBinPath — путь установки бинаря nfqws2.
	DefaultBinPath = "/opt/sbin/nfqws2"
	// DefaultConfigDir — директория конфигурации (общая с sing-box).
	DefaultConfigDir = "/opt/etc/sign-craze"
	// DefaultConfigPath — путь к nfqws2.conf.
	DefaultConfigPath = "/opt/etc/sign-craze/nfqws2.conf"
)

// Install устанавливает бинарь nfqws2 из tarball в binDst.
// Алгоритм:
//  1. Резервная копия текущего бинаря (если существует).
//  2. Распаковка tarball — ищет файл с именем "nfqws2" в любом подкаталоге.
//  3. Атомарная запись бинаря в binDst с правами 0755.
//  4. При ошибке записи — откат через RestoreBackup.
func Install(tarPath, binDst string) error {
	log.L().Info("установка nfqws2", "tarball", tarPath, "dst", binDst)

	binData, err := extractBinary(tarPath)
	if err != nil {
		return fmt.Errorf("dpi install: распаковка tarball: %w", err)
	}

	_, err = atomicfs.BackupAndReplace(binDst, binData, 0o755)
	if err != nil {
		return fmt.Errorf("dpi install: запись бинаря: %w", err)
	}

	log.L().Info("nfqws2 установлен", "path", binDst)
	return nil
}

// extractBinary распаковывает tarball и возвращает содержимое бинаря nfqws2.
// Ищет файл с именем "nfqws2" в любом подкаталоге архива.
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
		if filepath.Base(hdr.Name) == "nfqws2" {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("чтение бинаря из tar: %w", err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("бинарь 'nfqws2' не найден в архиве %s", tarPath)
}

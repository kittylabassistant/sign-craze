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
	// DefaultHostlistPath — путь к файлу со списком доменов для selective DPI desync.
	// Передаётся в nfqws2 через --hostlist=<path>. Один домен на строку.
	DefaultHostlistPath = "/opt/etc/sign-craze/dpi-hostlist.txt"
)

// Install устанавливает бинарь nfqws2 из tarball в binDst.
// Алгоритм:
//  1. Резервная копия текущего бинаря (rename, без чтения в RAM).
//  2. Стриминговая распаковка tarball → атомарная запись в binDst с правами 0755.
//  3. При ошибке записи — откат через RestoreBackup.
//
// Не загружает бинарь в RAM — критично для роутеров с 128MB.
func Install(tarPath, binDst string) error {
	log.L().Info("установка nfqws2", "tarball", tarPath, "dst", binDst)

	stream, err := openNfqwsBinaryStream(tarPath)
	if err != nil {
		return fmt.Errorf("dpi install: распаковка tarball: %w", err)
	}
	defer stream.Close()

	if _, err := atomicfs.BackupAndReplaceFromReader(binDst, stream.Reader, 0o755); err != nil {
		return fmt.Errorf("dpi install: запись бинаря: %w", err)
	}

	log.L().Info("nfqws2 установлен", "path", binDst)
	return nil
}

// binaryStream — потоковый ридер на содержимое бинаря из tarball.
type binaryStream struct {
	Reader io.Reader
	closer func() error
}

func (b *binaryStream) Close() error { return b.closer() }

// openNfqwsBinaryStream открывает tarball, ищет файл "nfqws2" в любом
// подкаталоге, возвращает stream без буферизации в RAM.
func openNfqwsBinaryStream(tarPath string) (*binaryStream, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("открытие tarball: %w", err)
	}

	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("gzip reader: %w", err)
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			_ = gz.Close()
			_ = f.Close()
			return nil, fmt.Errorf("бинарь 'nfqws2' не найден в архиве %s", tarPath)
		}
		if err != nil {
			_ = gz.Close()
			_ = f.Close()
			return nil, fmt.Errorf("чтение tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "nfqws2" {
			continue
		}
		closer := func() error {
			_ = gz.Close()
			return f.Close()
		}
		return &binaryStream{Reader: tr, closer: closer}, nil
	}
}

// extractBinaryToFile стримит бинарь nfqws2 из tarball напрямую в dstPath.
func extractBinaryToFile(tarPath, dstPath string, perm os.FileMode) error {
	stream, err := openNfqwsBinaryStream(tarPath)
	if err != nil {
		return err
	}
	defer stream.Close()

	if err := atomicfs.WriteFileAtomicFromReader(dstPath, stream.Reader, perm); err != nil {
		return fmt.Errorf("запись бинаря: %w", err)
	}
	return nil
}

package dpi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

// Install устанавливает бинарь nfqws2 из tarball или .ipk пакета в binDst.
// Алгоритм:
//  1. Резервная копия текущего бинаря (rename, без чтения в RAM).
//  2. Стриминговая распаковка → атомарная запись в binDst с правами 0755.
//  3. При ошибке записи — откат через RestoreBackup.
//
// Поддерживаемые форматы:
//   - .tar.gz — прямой tar.gz, содержащий бинарь "nfqws2" в любом подкаталоге.
//   - .ipk    — Entware-пакет: outer tar.gz → data.tar.gz → бинарь "nfqws2".
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

// openNfqwsBinaryStream открывает tarball или .ipk пакет, ищет файл "nfqws2",
// возвращает stream без буферизации в RAM.
//
// Для .ipk (Entware): outer tar.gz содержит data.tar.gz, внутри которого бинарь.
// Для .tar.gz: прямой поиск бинаря "nfqws2" в любом подкаталоге.
func openNfqwsBinaryStream(tarPath string) (*binaryStream, error) {
	if strings.HasSuffix(tarPath, ".ipk") {
		return openNfqwsBinaryStreamIPK(tarPath)
	}
	return openNfqwsBinaryStreamTarGZ(tarPath)
}

// openNfqwsBinaryStreamTarGZ ищет бинарь "nfqws2" в обычном tar.gz архиве.
func openNfqwsBinaryStreamTarGZ(tarPath string) (*binaryStream, error) {
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

// openNfqwsBinaryStreamIPK извлекает бинарь "nfqws2" из Entware .ipk пакета.
// Формат .ipk: tar.gz → { data.tar.gz, control.tar.gz, debian-binary }.
// Бинарь находится в data.tar.gz по пути */sbin/nfqws2 или */bin/nfqws2.
// Всё содержимое data.tar.gz буферизуется в RAM (обычно < 500KB).
func openNfqwsBinaryStreamIPK(ipkPath string) (*binaryStream, error) {
	f, err := os.Open(ipkPath)
	if err != nil {
		return nil, fmt.Errorf("открытие .ipk: %w", err)
	}
	defer f.Close()

	outerGZ, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf(".ipk gzip reader: %w", err)
	}
	defer outerGZ.Close()

	// Ищем data.tar.gz в outer tar.
	outerTar := tar.NewReader(outerGZ)
	var dataTarBytes []byte
	var hdr *tar.Header
	for {
		hdr, err = outerTar.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf(".ipk outer tar: %w", err)
		}
		if hdr.Name == "./data.tar.gz" || hdr.Name == "data.tar.gz" {
			dataTarBytes, err = io.ReadAll(outerTar)
			if err != nil {
				return nil, fmt.Errorf(".ipk чтение data.tar.gz: %w", err)
			}
			break
		}
	}
	if dataTarBytes == nil {
		return nil, fmt.Errorf(".ipk: data.tar.gz не найден в %s", ipkPath)
	}

	// Ищем бинарь nfqws2 в data.tar.gz.
	innerGZ, err := gzip.NewReader(bytes.NewReader(dataTarBytes))
	if err != nil {
		return nil, fmt.Errorf(".ipk inner gzip: %w", err)
	}
	defer innerGZ.Close()

	innerTar := tar.NewReader(innerGZ)
	var binBytes []byte
	for {
		hdr, err = innerTar.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("бинарь 'nfqws2' не найден в data.tar.gz (%s)", ipkPath)
		}
		if err != nil {
			return nil, fmt.Errorf(".ipk inner tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "nfqws2" {
			continue
		}
		// Читаем бинарь в буфер — innerTar ссылается на dataTarBytes, уже в RAM.
		binBytes, err = io.ReadAll(innerTar)
		if err != nil {
			return nil, fmt.Errorf(".ipk чтение бинаря: %w", err)
		}
		return &binaryStream{
			Reader: bytes.NewReader(binBytes),
			closer: func() error { return nil },
		}, nil
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

// InstallBlobs извлекает blob-файлы (quic_initial.bin, tls_clienthello.bin и
// прочие *.bin из etc/nfqws2/blobs/) из .ipk пакета в dstDir.
//
// Без blob-файлов стратегии --lua-desync=fake:blob=quic_initial / blob=tls_clienthello
// не работают: lua-init nfqws2 загружает их через --blob-dir. Если файлов нет
// — nfqws2 падает в init с ошибкой "blob not found".
//
// Идемпотентно: перезаписывает существующие файлы атомарно.
// Поддерживает только .ipk (для .tar.gz без специфичной структуры безопасно
// возвращает nil — blobs не требуются для не-Entware дистрибутивов).
func InstallBlobs(ipkPath, dstDir string) error {
	if !strings.HasSuffix(ipkPath, ".ipk") {
		log.L().Debug("dpi: blob-распаковка пропущена (не .ipk)", "path", ipkPath)
		return nil
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("dpi blobs: mkdir %s: %w", dstDir, err)
	}

	f, err := os.Open(ipkPath)
	if err != nil {
		return fmt.Errorf("dpi blobs: открытие .ipk: %w", err)
	}
	defer f.Close()

	outerGZ, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("dpi blobs: outer gzip: %w", err)
	}
	defer outerGZ.Close()

	outerTar := tar.NewReader(outerGZ)
	var dataTarBytes []byte
	for {
		hdr, err := outerTar.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("dpi blobs: outer tar: %w", err)
		}
		if hdr.Name == "./data.tar.gz" || hdr.Name == "data.tar.gz" {
			dataTarBytes, err = io.ReadAll(outerTar)
			if err != nil {
				return fmt.Errorf("dpi blobs: чтение data.tar.gz: %w", err)
			}
			break
		}
	}
	if dataTarBytes == nil {
		return fmt.Errorf("dpi blobs: data.tar.gz не найден в %s", ipkPath)
	}

	innerGZ, err := gzip.NewReader(bytes.NewReader(dataTarBytes))
	if err != nil {
		return fmt.Errorf("dpi blobs: inner gzip: %w", err)
	}
	defer innerGZ.Close()

	innerTar := tar.NewReader(innerGZ)
	count := 0
	for {
		hdr, err := innerTar.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("dpi blobs: inner tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// Пропускаем path traversal: blob должен лежать ровно в etc/nfqws2/blobs/.
		clean := filepath.Clean(hdr.Name)
		if !strings.Contains(clean, "etc/nfqws2/blobs/") {
			continue
		}
		base := filepath.Base(clean)
		if !strings.HasSuffix(base, ".bin") {
			continue
		}
		dstPath := filepath.Join(dstDir, base)
		if err := atomicfs.WriteFileAtomicFromReader(dstPath, innerTar, 0o644); err != nil {
			return fmt.Errorf("dpi blobs: запись %s: %w", dstPath, err)
		}
		count++
		log.L().Debug("dpi blob извлечён", "name", base, "dst", dstPath)
	}
	if count == 0 {
		log.L().Warn("dpi blobs: ни одного *.bin не найдено в .ipk", "path", ipkPath)
	} else {
		log.L().Info("dpi blobs распакованы", "count", count, "dir", dstDir)
	}
	return nil
}

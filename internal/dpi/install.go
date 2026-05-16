package dpi

import (
	"archive/tar"
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
//
// Стриминговая реализация: файл остаётся открытым до вызова Close() на возвращённом
// binaryStream. io.ReadAll не используется — критично для 128MB MIPS-роутеров.
func openNfqwsBinaryStreamIPK(ipkPath string) (*binaryStream, error) {
	f, err := os.Open(ipkPath)
	if err != nil {
		return nil, fmt.Errorf("открытие .ipk: %w", err)
	}

	// closeAll закрывает все ресурсы при ошибке (до возврата stream).
	var (
		outerGZ *gzip.Reader
		innerGZ *gzip.Reader
	)
	cleanup := func() {
		if innerGZ != nil {
			_ = innerGZ.Close()
		}
		if outerGZ != nil {
			_ = outerGZ.Close()
		}
		_ = f.Close()
	}

	outerGZ, err = gzip.NewReader(f)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf(".ipk gzip reader: %w", err)
	}

	// Ищем data.tar.gz в outer tar потоково — без io.ReadAll.
	outerTar := tar.NewReader(outerGZ)
	found := false
	for {
		var hdr *tar.Header
		hdr, err = outerTar.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			cleanup()
			return nil, fmt.Errorf(".ipk outer tar: %w", err)
		}
		if hdr.Name == "./data.tar.gz" || hdr.Name == "data.tar.gz" {
			found = true
			break
		}
		// Пропускаем прочие записи (control.tar.gz, debian-binary).
	}
	if !found {
		cleanup()
		return nil, fmt.Errorf(".ipk: data.tar.gz не найден в %s", ipkPath)
	}

	// outerTar стоит на data.tar.gz — читаем его через вложенный gzip прямо из потока.
	innerGZ, err = gzip.NewReader(outerTar)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf(".ipk inner gzip: %w", err)
	}

	innerTar := tar.NewReader(innerGZ)
	for {
		hdr, hdrErr := innerTar.Next()
		if errors.Is(hdrErr, io.EOF) {
			cleanup()
			return nil, fmt.Errorf("бинарь 'nfqws2' не найден в data.tar.gz (%s)", ipkPath)
		}
		if hdrErr != nil {
			cleanup()
			return nil, fmt.Errorf(".ipk inner tar: %w", hdrErr)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "nfqws2" {
			continue
		}
		// Позиция innerTar — начало бинаря; возвращаем stream, держа файл открытым.
		closer := func() error {
			_ = innerGZ.Close()
			_ = outerGZ.Close()
			return f.Close()
		}
		return &binaryStream{Reader: innerTar, closer: closer}, nil
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

// InstallAssets извлекает из .ipk все runtime-ресурсы nfqws2:
//   - blob-payload'ы (etc/nfqws2/blobs/*.bin → blobDir/*.bin)
//   - lua-расширения (etc/nfqws2/lua/*.lua.gz → luaDir/*.lua, gunzip)
//
// Без lua-файлов функции стратегий типа `circular`, `multisplit`,
// `hostfakesplit` не существуют — они определены в zapret-antidpi.lua.
// Без blob-файлов стратегии `--lua-desync=fake:blob=...` падают с
// "blob not found".
//
// Идемпотентно: перезаписывает существующие файлы атомарно.
// Для .tar.gz без специфичной структуры — no-op.
func InstallAssets(ipkPath, blobDir, luaDir string) error {
	if !strings.HasSuffix(ipkPath, ".ipk") {
		log.L().Debug("dpi: assets-распаковка пропущена (не .ipk)", "path", ipkPath)
		return nil
	}
	if err := os.MkdirAll(blobDir, 0o755); err != nil {
		return fmt.Errorf("dpi assets: mkdir %s: %w", blobDir, err)
	}
	if err := os.MkdirAll(luaDir, 0o755); err != nil {
		return fmt.Errorf("dpi assets: mkdir %s: %w", luaDir, err)
	}

	f, err := os.Open(ipkPath)
	if err != nil {
		return fmt.Errorf("dpi assets: открытие .ipk: %w", err)
	}
	defer f.Close()

	outerGZ, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("dpi assets: outer gzip: %w", err)
	}
	defer outerGZ.Close()

	outerTar := tar.NewReader(outerGZ)
	foundData := false
	for {
		hdr, hdrErr := outerTar.Next()
		if errors.Is(hdrErr, io.EOF) {
			break
		}
		if hdrErr != nil {
			return fmt.Errorf("dpi assets: outer tar: %w", hdrErr)
		}
		if hdr.Name == "./data.tar.gz" || hdr.Name == "data.tar.gz" {
			foundData = true
			break
		}
	}
	if !foundData {
		return fmt.Errorf("dpi assets: data.tar.gz не найден в %s", ipkPath)
	}

	// outerTar стоит на data.tar.gz — читаем inner gzip прямо из потока без io.ReadAll.
	innerGZ, err := gzip.NewReader(outerTar)
	if err != nil {
		return fmt.Errorf("dpi assets: inner gzip: %w", err)
	}
	defer innerGZ.Close()

	innerTar := tar.NewReader(innerGZ)
	blobCount, luaCount := 0, 0
	for {
		hdr, err := innerTar.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("dpi assets: inner tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		clean := filepath.Clean(hdr.Name)
		if strings.Contains(clean, "..") {
			return fmt.Errorf("dpi assets: path traversal в записи %q", hdr.Name)
		}
		base := filepath.Base(clean)

		switch {
		case strings.Contains(clean, "etc/nfqws2/blobs/") && strings.HasSuffix(base, ".bin"):
			dstPath := filepath.Join(blobDir, base)
			if err := atomicfs.WriteFileAtomicFromReader(dstPath, innerTar, 0o644); err != nil {
				return fmt.Errorf("dpi assets: запись blob %s: %w", dstPath, err)
			}
			blobCount++
			log.L().Debug("dpi blob извлечён", "name", base, "dst", dstPath)

		case strings.Contains(clean, "etc/nfqws2/lua/") && strings.HasSuffix(base, ".lua.gz"):
			// Распаковываем .lua.gz → .lua потоково, без io.ReadAll.
			gz, err := gzip.NewReader(innerTar)
			if err != nil {
				return fmt.Errorf("dpi assets: %s gzip: %w", base, err)
			}
			outName := strings.TrimSuffix(base, ".gz")
			dstPath := filepath.Join(luaDir, outName)
			writeErr := atomicfs.WriteFileAtomicFromReader(dstPath, gz, 0o644)
			_ = gz.Close()
			if writeErr != nil {
				return fmt.Errorf("dpi assets: запись lua %s: %w", dstPath, writeErr)
			}
			luaCount++
			log.L().Debug("dpi lua извлечён", "name", outName, "dst", dstPath)
		}
	}
	if blobCount == 0 && luaCount == 0 {
		log.L().Warn("dpi assets: blobs/lua не найдены в .ipk", "path", ipkPath)
	} else {
		log.L().Info("dpi assets распакованы", "blobs", blobCount, "lua", luaCount, "blobDir", blobDir, "luaDir", luaDir)
	}
	return nil
}

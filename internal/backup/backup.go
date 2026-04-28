package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultDir — стандартная директория для backup-архивов.
const DefaultDir = "/opt/var/lib/sign-craze/backups"

// TimestampedName возвращает имя файла вида "backup-2026-04-28T12-34-56.tar.gz".
func TimestampedName(prefix string) string {
	return fmt.Sprintf("%s-%s.tar.gz", prefix, time.Now().UTC().Format("2006-01-02T15-04-05"))
}

// Create архивирует srcDir в dstFile (.tar.gz).
// Если srcDir указывает на одиночный файл — архивируется только этот файл.
// Создаёт filepath.Dir(dstFile) при отсутствии.
func Create(srcPath, dstFile string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("backup: stat %s: %w", srcPath, err)
	}

	if mkErr := os.MkdirAll(filepath.Dir(dstFile), 0o755); mkErr != nil {
		return fmt.Errorf("backup: mkdir: %w", mkErr)
	}

	out, err := os.OpenFile(dstFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("backup: создание %s: %w", dstFile, err)
	}
	defer func() { _ = out.Close() }()

	gz := gzip.NewWriter(out)
	defer func() { _ = gz.Close() }()

	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()

	root := srcPath
	if !info.IsDir() {
		root = filepath.Dir(srcPath)
	}

	walkFn := func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Имя в архиве — путь относительно root.
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		// Если архивируем один файл — пропустим саму root-директорию.
		if rel == "." {
			return nil
		}

		hdr, hdrErr := tar.FileInfoHeader(fi, "")
		if hdrErr != nil {
			return hdrErr
		}
		hdr.Name = rel

		if writeErr := tw.WriteHeader(hdr); writeErr != nil {
			return writeErr
		}
		if !fi.Mode().IsRegular() {
			return nil
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer f.Close()

		if _, copyErr := io.Copy(tw, f); copyErr != nil {
			return copyErr
		}
		return nil
	}

	if info.IsDir() {
		return filepath.Walk(srcPath, walkFn)
	}
	// Одиночный файл.
	return walkFn(srcPath, info, nil)
}

// Restore извлекает tar.gz srcFile в dstDir. Создаёт dstDir при отсутствии.
// Защищает от path traversal: пути с ".." отклоняются.
func Restore(srcFile, dstDir string) error {
	in, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("backup restore: открытие %s: %w", srcFile, err)
	}
	defer func() { _ = in.Close() }()

	gz, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("backup restore: gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()

	if mkErr := os.MkdirAll(dstDir, 0o755); mkErr != nil {
		return fmt.Errorf("backup restore: mkdir %s: %w", dstDir, mkErr)
	}

	tr := tar.NewReader(gz)
	for {
		hdr, hdrErr := tr.Next()
		if hdrErr == io.EOF {
			break
		}
		if hdrErr != nil {
			return fmt.Errorf("backup restore: tar: %w", hdrErr)
		}

		// Защита от path traversal.
		if strings.Contains(hdr.Name, "..") || filepath.IsAbs(hdr.Name) {
			return fmt.Errorf("backup restore: подозрительный путь в архиве: %q", hdr.Name)
		}
		dst := filepath.Join(dstDir, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if mkErr := os.MkdirAll(dst, os.FileMode(hdr.Mode)); mkErr != nil {
				return mkErr
			}
		case tar.TypeReg:
			if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
				return mkErr
			}
			f, fErr := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if fErr != nil {
				return fErr
			}
			if _, cpErr := io.Copy(f, tr); cpErr != nil {
				_ = f.Close()
				return cpErr
			}
			_ = f.Close()
		default:
			// Симлинки и спец. файлы пропускаем.
		}
	}
	return nil
}

package backup

import (
	"archive/tar"
	"compress/gzip"
	"errors"
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

// Лимиты Restore — защита от tar-bomb на 128MB-роутере. Подобраны с запасом
// для типичных backup'ов sign-craze (state.json, config.json, креды,
// несколько hostlist-файлов): десятки KB суммарно.
const (
	maxTarFiles      = 512
	maxTarTotalBytes = 64 << 20 // 64 MB суммарно (распакованных)
	maxTarSingleFile = 16 << 20 // 16 MB per файл
)

// Restore извлекает tar.gz srcFile в dstDir. Создаёт dstDir при отсутствии.
// Защищает от:
//   - path traversal через filepath.Clean + проверку HasPrefix(dstDir);
//   - tar-bomb через ограничения на число файлов, общий размер,
//     максимальный размер одного файла (см. константы maxTar*).
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

	cleanDst, absErr := filepath.Abs(dstDir)
	if absErr != nil {
		return fmt.Errorf("backup restore: abs %s: %w", dstDir, absErr)
	}

	tr := tar.NewReader(gz)
	var (
		fileCount  int
		totalBytes int64
	)
	for {
		hdr, hdrErr := tr.Next()
		if hdrErr == io.EOF {
			break
		}
		if hdrErr != nil {
			return fmt.Errorf("backup restore: tar: %w", hdrErr)
		}

		fileCount++
		if fileCount > maxTarFiles {
			return fmt.Errorf("backup restore: превышено максимальное число файлов %d (защита от tar-bomb)", maxTarFiles)
		}

		// Path traversal: filepath.Clean нормализует "../", "..//.." и т.п.;
		// после Join проверяем что dst остался внутри dstDir.
		if filepath.IsAbs(hdr.Name) {
			return fmt.Errorf("backup restore: абсолютный путь в архиве запрещён: %q", hdr.Name)
		}
		dst := filepath.Join(cleanDst, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(dst+string(filepath.Separator), cleanDst+string(filepath.Separator)) && dst != cleanDst {
			return fmt.Errorf("backup restore: путь %q выходит за пределы %s (path traversal)", hdr.Name, dstDir)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if mkErr := os.MkdirAll(dst, os.FileMode(hdr.Mode)); mkErr != nil {
				return mkErr
			}
		case tar.TypeReg:
			if hdr.Size > maxTarSingleFile {
				return fmt.Errorf("backup restore: файл %q заявлен размером %d > лимита %d", hdr.Name, hdr.Size, maxTarSingleFile)
			}
			if mkErr := os.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
				return mkErr
			}
			f, fErr := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if fErr != nil {
				return fErr
			}
			// io.CopyN с лимитом +1: если декомпрессия отдаёт больше заявленного
			// в hdr.Size (несоответствие manifest/payload — типичный признак
			// tar-bomb с искажённым заголовком), CopyN остановится и вернёт
			// именно столько байт сколько reader выдал.
			n, cpErr := io.CopyN(f, tr, maxTarSingleFile+1)
			_ = f.Close()
			if cpErr != nil && !errors.Is(cpErr, io.EOF) {
				return fmt.Errorf("backup restore: копирование %q: %w", hdr.Name, cpErr)
			}
			if n > maxTarSingleFile {
				return fmt.Errorf("backup restore: фактический размер %q превысил лимит %d (tar-bomb?)", hdr.Name, maxTarSingleFile)
			}
			totalBytes += n
			if totalBytes > maxTarTotalBytes {
				return fmt.Errorf("backup restore: суммарный размер %d > лимита %d (tar-bomb?)", totalBytes, maxTarTotalBytes)
			}
		default:
			// Симлинки и спец. файлы пропускаем.
		}
	}
	return nil
}

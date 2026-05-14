package naiveproxy

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/ulikunitz/xz"
)

// elfMagic — первые 4 байта Linux ELF (\x7f E L F).
var elfMagic = []byte{0x7f, 'E', 'L', 'F'}

// ExtractBinary распаковывает tar.xz и извлекает бинарь "naive" в dstPath.
// Стриминговая распаковка через xz → tar reader → atomicfs (без буферизации
// в RAM). Проверяет ELF magic первых 4 байт.
func ExtractBinary(tarXZPath, dstPath string, perm os.FileMode) error {
	f, err := os.Open(tarXZPath)
	if err != nil {
		return fmt.Errorf("naiveproxy extract: открытие %s: %w", tarXZPath, err)
	}
	defer f.Close()

	xzReader, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("naiveproxy extract: xz reader: %w", err)
	}

	tr := tar.NewReader(xzReader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("naiveproxy extract: бинарь 'naive' не найден в %s", tarXZPath)
		}
		if err != nil {
			return fmt.Errorf("naiveproxy extract: чтение tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "naive" {
			continue
		}

		magic := make([]byte, len(elfMagic))
		n, readErr := io.ReadFull(tr, magic)
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			return fmt.Errorf("naiveproxy extract: чтение ELF magic: %w", readErr)
		}
		if n < len(elfMagic) || !bytes.Equal(magic[:n], elfMagic) {
			return fmt.Errorf("naiveproxy extract: %s не ELF-бинарь (magic=%x)", hdr.Name, magic[:n])
		}

		full := io.MultiReader(bytes.NewReader(magic), tr)
		if _, err := atomicfs.BackupAndReplaceFromReader(dstPath, full, perm); err != nil {
			return fmt.Errorf("naiveproxy extract: запись %s: %w", dstPath, err)
		}
		return nil
	}
}

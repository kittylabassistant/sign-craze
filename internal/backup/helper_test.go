package backup

import (
	"archive/tar"
	"compress/gzip"
	"os"
)

// writeBadArchive создаёт tar.gz с одной записью имени name (для тестирования
// защиты от path traversal).
func writeBadArchive(path, name string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	hdr := &tar.Header{Name: name, Mode: 0o644, Size: 5, Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write([]byte("hello"))
	return err
}

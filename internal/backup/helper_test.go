package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
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
	if hdrErr := tw.WriteHeader(hdr); hdrErr != nil {
		return hdrErr
	}
	_, wrErr := tw.Write([]byte("hello"))
	return wrErr
}

// writeManyFiles создаёт tar.gz с n маленькими файлами для теста tar-bomb
// по числу файлов.
func writeManyFiles(path string, n int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	body := []byte("x")
	for i := 0; i < n; i++ {
		hdr := &tar.Header{
			Name:     fmt.Sprintf("f-%05d.txt", i),
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		if hErr := tw.WriteHeader(hdr); hErr != nil {
			return hErr
		}
		if _, wErr := tw.Write(body); wErr != nil {
			return wErr
		}
	}
	return nil
}

// writeOversizedFile создаёт tar.gz с одним файлом заявленного размера >
// maxTarSingleFile. Реальный payload — N нулевых байт, gzip сжимает его
// почти до нуля → file size on disk < 1KB, в то время как hdr.Size = N.
// Этого достаточно чтобы триггернуть лимит maxTarSingleFile в Restore.
func writeOversizedFile(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	hdr := &tar.Header{Name: "huge.bin", Mode: 0o644, Size: size, Typeflag: tar.TypeReg}
	if hErr := tw.WriteHeader(hdr); hErr != nil {
		return hErr
	}
	// записываем нули чанками — gzip сильно сжимает
	zeros := make([]byte, 64*1024)
	written := int64(0)
	for written < size {
		chunk := int64(len(zeros))
		if remaining := size - written; remaining < chunk {
			chunk = remaining
		}
		if _, wErr := tw.Write(zeros[:chunk]); wErr != nil {
			return wErr
		}
		written += chunk
	}
	return nil
}

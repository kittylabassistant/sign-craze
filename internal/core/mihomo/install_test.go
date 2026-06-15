package mihomo

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/elfcheck"
)

// makeMihomoGz создаёт gzip-файл с заданным content (mihomo-style: чистый ELF
// без TAR-обёртки).
func makeMihomoGz(t *testing.T, body []byte) string {
	t.Helper()
	dir := t.TempDir()
	gzPath := filepath.Join(dir, "mihomo-linux-test.gz")

	f, err := os.Create(gzPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	if _, err := gw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return gzPath
}

func elfBody() []byte {
	body := append([]byte{}, elfcheck.Magic...)
	body = append(body, []byte("rest-of-mihomo-binary")...)
	return body
}

// TestOpenMihomoBinaryStream_ReadsValidELF — поток возвращает полный ELF-контент.
func TestOpenMihomoBinaryStream_ReadsValidELF(t *testing.T) {
	gzPath := makeMihomoGz(t, elfBody())

	stream, err := openMihomoBinaryStream(gzPath)
	if err != nil {
		t.Fatalf("openMihomoBinaryStream: %v", err)
	}
	defer stream.Close()

	got, err := io.ReadAll(stream.Reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, elfBody()) {
		t.Errorf("содержимое отличается:\ngot:  %x\nwant: %x", got, elfBody())
	}
}

// TestOpenMihomoBinaryStream_RejectsNonELF — gzip с не-ELF содержимым отвергается.
func TestOpenMihomoBinaryStream_RejectsNonELF(t *testing.T) {
	gzPath := makeMihomoGz(t, []byte("MZ-not-elf"))
	_, err := openMihomoBinaryStream(gzPath)
	if err == nil {
		t.Fatal("ожидалась ошибка для не-ELF содержимого")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("не ELF")) {
		t.Errorf("ошибка должна упоминать ELF: %v", err)
	}
}

// TestOpenMihomoBinaryStream_RejectsCorruptGzip — битый gzip → понятная ошибка.
func TestOpenMihomoBinaryStream_RejectsCorruptGzip(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.gz")
	_ = os.WriteFile(bad, []byte("not a gzip"), 0o644)
	_, err := openMihomoBinaryStream(bad)
	if err == nil {
		t.Fatal("ожидалась ошибка для битого gzip")
	}
}

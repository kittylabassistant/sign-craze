package naiveproxy

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ulikunitz/xz"
)

// makeTarXZ создаёт in-memory tar.xz с одним файлом по path/content.
func makeTarXZ(t *testing.T, name string, content []byte) string {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	hdr := &tar.Header{
		Name:     name,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	var xzBuf bytes.Buffer
	xzWriter, err := xz.NewWriter(&xzBuf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := xzWriter.Write(tarBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := xzWriter.Close(); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.tar.xz")
	if err := os.WriteFile(path, xzBuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractBinary_TarXZ(t *testing.T) {
	// Корректный ELF prefix + minimal payload
	elf := append([]byte{0x7f, 'E', 'L', 'F'}, []byte("fake-naive-binary-payload")...)
	path := makeTarXZ(t, "naiveproxy-v148/naive", elf)
	dst := filepath.Join(t.TempDir(), "naive")
	if err := ExtractBinary(path, dst, 0o755); err != nil {
		t.Fatalf("ExtractBinary: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, elf) {
		t.Errorf("содержимое не совпадает: got=%x want=%x", got, elf)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("perm=%o, want 0755", info.Mode().Perm())
	}
}

func TestExtractBinary_NotFound(t *testing.T) {
	path := makeTarXZ(t, "naiveproxy-v148/README.md", []byte("hello"))
	dst := filepath.Join(t.TempDir(), "naive")
	if err := ExtractBinary(path, dst, 0o755); err == nil {
		t.Fatal("ожидалась ошибка про отсутствие naive")
	}
}

func TestExtractBinary_NotELF(t *testing.T) {
	path := makeTarXZ(t, "naive", []byte("not-elf"))
	dst := filepath.Join(t.TempDir(), "naive")
	if err := ExtractBinary(path, dst, 0o755); err == nil {
		t.Fatal("ожидалась ошибка про не-ELF magic")
	}
}

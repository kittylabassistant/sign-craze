package mihomo

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
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
	body := append([]byte{}, elfMagic...)
	body = append(body, []byte("rest-of-mihomo-binary")...)
	return body
}

func TestExtractBinaryToFile_Success(t *testing.T) {
	gzPath := makeMihomoGz(t, elfBody())

	dst := filepath.Join(t.TempDir(), "out", "mihomo")
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	if err := extractBinaryToFile(gzPath, dst, 0o755); err != nil {
		t.Fatalf("extractBinaryToFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if !bytes.Equal(got, elfBody()) {
		t.Errorf("содержимое отличается:\ngot:  %x\nwant: %x", got, elfBody())
	}
	info, _ := os.Stat(dst)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("perm = %o, ожидалось 0755", info.Mode().Perm())
	}
}

// TestExtractBinaryToFile_RejectsNonELF — gzip с не-ELF содержимым отвергается.
func TestExtractBinaryToFile_RejectsNonELF(t *testing.T) {
	gzPath := makeMihomoGz(t, []byte("MZ-not-elf"))
	dst := filepath.Join(t.TempDir(), "out", "mihomo")
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	err := extractBinaryToFile(gzPath, dst, 0o755)
	if err == nil {
		t.Fatal("ожидалась ошибка для не-ELF содержимого")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("не ELF")) {
		t.Errorf("ошибка должна упоминать ELF: %v", err)
	}
}

// TestExtractBinaryToFile_RejectsCorruptGzip — битый gzip → понятная ошибка.
func TestExtractBinaryToFile_RejectsCorruptGzip(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.gz")
	_ = os.WriteFile(bad, []byte("not a gzip"), 0o644)
	dst := filepath.Join(dir, "mihomo")
	err := extractBinaryToFile(bad, dst, 0o755)
	if err == nil {
		t.Fatal("ожидалась ошибка для битого gzip")
	}
}

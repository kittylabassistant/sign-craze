package xray

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// makeXrayZip создаёт zip-архив с одним файлом "xray", содержащим
// заданный body. Использует zip.Writer, как настоящий релиз.
func makeXrayZip(t *testing.T, body []byte, entryName string) string {
	t.Helper()
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "Xray-linux-test.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	fw, err := w.Create(entryName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return zipPath
}

// elfBody — минимальный ELF-magic-подобный body для прохождения проверки.
func elfBody() []byte {
	body := append([]byte{}, elfMagic...)
	body = append(body, []byte("rest-of-binary-content")...)
	return body
}

func TestExtractBinaryToFile_Success(t *testing.T) {
	zipPath := makeXrayZip(t, elfBody(), "xray")

	dst := filepath.Join(t.TempDir(), "out", "xray")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractBinaryToFile(zipPath, dst, 0o755); err != nil {
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

// TestExtractBinaryToFile_RejectsNonELF — содержимое без ELF-magic должно
// быть отвергнуто (защита от компрометированного релиза).
func TestExtractBinaryToFile_RejectsNonELF(t *testing.T) {
	zipPath := makeXrayZip(t, []byte("MZ-not-elf"), "xray")
	dst := filepath.Join(t.TempDir(), "out", "xray")
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	err := extractBinaryToFile(zipPath, dst, 0o755)
	if err == nil {
		t.Fatal("ожидалась ошибка для не-ELF содержимого")
	}
	if !contains(err.Error(), "не ELF") {
		t.Errorf("ошибка должна упоминать ELF: %v", err)
	}
}

// TestOpenXrayBinaryStream_NotFound — при отсутствии файла xray в zip
// должна быть явная ошибка.
func TestOpenXrayBinaryStream_NotFound(t *testing.T) {
	zipPath := makeXrayZip(t, elfBody(), "config.json")
	_, err := openXrayBinaryStream(zipPath)
	if err == nil {
		t.Fatal("ожидалась ошибка для zip без xray")
	}
	if !contains(err.Error(), "не найден") {
		t.Errorf("ошибка должна сообщать о ненайденном бинаре: %v", err)
	}
}

// TestOpenXrayBinaryStream_NestedPath — zip может содержать xray в подкаталоге
// (некоторые сборки делают `Xray-linux-...zip` с префиксом каталога).
// Парсер должен принять и `xray`, и `*/xray`.
func TestOpenXrayBinaryStream_NestedPath(t *testing.T) {
	zipPath := makeXrayZip(t, elfBody(), "Xray-linux-arm64-v8a/xray")
	stream, err := openXrayBinaryStream(zipPath)
	if err != nil {
		t.Fatalf("openXrayBinaryStream: %v", err)
	}
	defer stream.Close()
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

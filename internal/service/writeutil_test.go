package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteIfChanged_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact")

	changed, err := writeIfChanged(path, []byte("v1"), 0o644, "test-artifact")
	if err != nil {
		t.Fatalf("writeIfChanged: %v", err)
	}
	if !changed {
		t.Error("ожидался changed=true при создании нового файла")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("файл не создан: %v", readErr)
	}
	if string(got) != "v1" {
		t.Errorf("содержимое = %q, ожидалось %q", got, "v1")
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat: %v", statErr)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("права = %o, ожидалось 0644", info.Mode().Perm())
	}
}

// TestWriteIfChanged_SkipsWriteWhenIdentical — вторая запись с тем же
// содержимым не должна трогать файл (mtime не меняется). Это и есть
// идемпотентность, на которой держится WriteShim/WriteHook при повторных
// --reapply/--install на роутере.
func TestWriteIfChanged_SkipsWriteWhenIdentical(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact")

	if _, err := writeIfChanged(path, []byte("same"), 0o644, "test-artifact"); err != nil {
		t.Fatalf("первый writeIfChanged: %v", err)
	}
	info1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat после первой записи: %v", err)
	}

	changed, err := writeIfChanged(path, []byte("same"), 0o644, "test-artifact")
	if err != nil {
		t.Fatalf("второй writeIfChanged: %v", err)
	}
	if changed {
		t.Error("ожидался changed=false при идентичном содержимом")
	}

	info2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat после второй записи: %v", err)
	}
	if info1.ModTime() != info2.ModTime() {
		t.Error("writeIfChanged перезаписал файл без изменений (идемпотентность нарушена)")
	}
}

func TestWriteIfChanged_OverwritesWhenDifferent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact")

	if _, err := writeIfChanged(path, []byte("v1"), 0o644, "test-artifact"); err != nil {
		t.Fatalf("первый writeIfChanged: %v", err)
	}

	changed, err := writeIfChanged(path, []byte("v2"), 0o644, "test-artifact")
	if err != nil {
		t.Fatalf("второй writeIfChanged: %v", err)
	}
	if !changed {
		t.Error("ожидался changed=true при отличающемся содержимом")
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("чтение результата: %v", readErr)
	}
	if string(got) != "v2" {
		t.Errorf("содержимое = %q, ожидалось %q (перезапись)", got, "v2")
	}
}

// TestWriteIfChanged_ReturnsErrorOnWriteFailure — родитель пути является
// обычным файлом, не каталогом: atomicfs.WriteFileAtomic не сможет создать
// под ним temp-файл, writeIfChanged должен вернуть ошибку (а не молча
// проглотить её).
func TestWriteIfChanged_ReturnsErrorOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	path := filepath.Join(blocker, "sub", "artifact")

	if _, err := writeIfChanged(path, []byte("v1"), 0o644, "test-artifact"); err == nil {
		t.Fatal("ожидалась ошибка: родитель пути — обычный файл, не каталог")
	}
}

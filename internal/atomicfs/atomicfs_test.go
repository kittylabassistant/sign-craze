package atomicfs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomic_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.txt")

	if err := WriteFileAtomic(dst, []byte("привет"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "привет" {
		t.Errorf("содержимое = %q, ожидалось %q", got, "привет")
	}
}

func TestWriteFileAtomic_MkdirAll(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "sub", "dir", "out.txt")

	if err := WriteFileAtomic(dst, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic с вложенной директорией: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
}

func TestWriteFileAtomic_NoTempFileLeft(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.txt")

	if err := WriteFileAtomic(dst, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".atomic-") {
			t.Errorf("temp-файл не удалён: %s", e.Name())
		}
	}
}

func TestWriteFileAtomic_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.txt")

	_ = WriteFileAtomic(dst, []byte("старое"), 0o644)
	_ = WriteFileAtomic(dst, []byte("новое"), 0o644)

	got, _ := os.ReadFile(dst)
	if string(got) != "новое" {
		t.Errorf("содержимое = %q, ожидалось %q", got, "новое")
	}
}

func TestWriteFileAtomic_SetsPermissions(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "exec.sh")

	if err := WriteFileAtomic(dst, []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("права = %o, ожидалось 0755", info.Mode().Perm())
	}
}

func TestRestoreBackup(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "bin")
	backupPath := filepath.Join(dir, "bin.bak")

	_ = WriteFileAtomic(backupPath, []byte("old-version"), 0o755)
	_ = WriteFileAtomic(dst, []byte("broken"), 0o755)

	if err := RestoreBackup(backupPath, dst); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}

	got, _ := os.ReadFile(dst)
	if string(got) != "old-version" {
		t.Errorf("после восстановления = %q, ожидалось %q", got, "old-version")
	}

	// резервный файл должен быть удалён
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("резервный файл не удалён после восстановления")
	}
}

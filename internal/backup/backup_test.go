package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateRestore_Directory(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("beta"), 0o600); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := Create(src, dst); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if info, err := os.Stat(dst); err != nil || info.Size() == 0 {
		t.Fatalf("backup пуст или отсутствует: %v", err)
	}

	restored := t.TempDir()
	if err := Restore(dst, restored); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(restored, "a.txt"))
	if err != nil || string(got) != "alpha" {
		t.Errorf("a.txt = %q (err=%v)", got, err)
	}
	got, err = os.ReadFile(filepath.Join(restored, "sub", "b.txt"))
	if err != nil || string(got) != "beta" {
		t.Errorf("sub/b.txt = %q (err=%v)", got, err)
	}
}

func TestCreateRestore_SingleFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(src, []byte(`{"k":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "backup.tar.gz")
	if err := Create(src, dst); err != nil {
		t.Fatalf("Create: %v", err)
	}

	restored := t.TempDir()
	if err := Restore(dst, restored); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(restored, "config.json"))
	if err != nil || string(got) != `{"k":1}` {
		t.Errorf("config.json = %q (err=%v)", got, err)
	}
}

func TestRestore_PathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "evil.tar.gz")
	if err := writeBadArchive(bad, "../escape.txt"); err != nil {
		t.Fatal(err)
	}
	err := Restore(bad, t.TempDir())
	if err == nil {
		t.Fatal("ожидалась ошибка path traversal")
	}
	if !strings.Contains(err.Error(), "path traversal") && !strings.Contains(err.Error(), "выходит за пределы") {
		t.Errorf("ожидалось сообщение про path traversal, получено: %v", err)
	}
}

func TestRestore_TarBomb_МногоФайлов(t *testing.T) {
	dir := t.TempDir()
	bomb := filepath.Join(dir, "bomb.tar.gz")
	if err := writeManyFiles(bomb, maxTarFiles+10); err != nil {
		t.Fatal(err)
	}
	err := Restore(bomb, t.TempDir())
	if err == nil {
		t.Fatal("ожидалась ошибка tar-bomb (превышено число файлов)")
	}
	if !strings.Contains(err.Error(), "число файлов") {
		t.Errorf("ожидалось сообщение про число файлов, получено: %v", err)
	}
}

func TestRestore_TarBomb_БольшойФайл(t *testing.T) {
	dir := t.TempDir()
	bomb := filepath.Join(dir, "bigbomb.tar.gz")
	if err := writeOversizedFile(bomb, maxTarSingleFile+1024); err != nil {
		t.Fatal(err)
	}
	err := Restore(bomb, t.TempDir())
	if err == nil {
		t.Fatal("ожидалась ошибка tar-bomb (большой файл)")
	}
	if !strings.Contains(err.Error(), "лимита") && !strings.Contains(err.Error(), "tar-bomb") {
		t.Errorf("ожидалось сообщение про лимит размера, получено: %v", err)
	}
}

func TestTimestampedName(t *testing.T) {
	name := TimestampedName("backup")
	if !strings.HasPrefix(name, "backup-") || !strings.HasSuffix(name, ".tar.gz") {
		t.Errorf("неожиданный формат %q", name)
	}
}

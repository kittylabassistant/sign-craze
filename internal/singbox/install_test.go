package singbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// makeTarball создаёт в памяти .tar.gz с одним файлом sing-box.
func makeTarball(content []byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	_ = tw.WriteHeader(&tar.Header{
		Name: "sing-box-v1.10.0-linux-arm64/sing-box",
		Mode: 0o755,
		Size: int64(len(content)),
	})
	_, _ = tw.Write(content)
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// TestPrepareAndValidate_OK — happy path: бинарь распакован во временный файл,
// конфиг валидирован. Возвращённый путь существует и chmod 0755.
func TestPrepareAndValidate_OK(t *testing.T) {
	tarPath := filepath.Join(t.TempDir(), "sb.tar.gz")
	_ = os.WriteFile(tarPath, makeTarball([]byte("fake-binary")), 0o644)

	configPath := filepath.Join(t.TempDir(), "config.json")

	// Mock: check всегда успешен.
	r := exectx.MockMatcher(func(cmd string, args ...string) (exectx.Result, error) {
		if filepath.Base(cmd) == "sing-box" && len(args) > 0 && args[0] == "check" {
			return exectx.Result{ExitCode: 0}, nil
		}
		return exectx.Result{ExitCode: 1}, nil
	})

	params := DefaultConfigParams()
	params.Outbounds = []types.Outbound{{Tag: "direct", Type: "direct"}}
	params.DefaultOutboundTag = "direct"

	tempBin, err := PrepareAndValidate(context.Background(), r, tarPath, configPath, params)
	if err != nil {
		t.Fatalf("PrepareAndValidate: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(tempBin))

	info, err := os.Stat(tempBin)
	if err != nil {
		t.Fatalf("временный бинарь не создан: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("права = %o, ожидалось 0755", info.Mode().Perm())
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("конфиг не создан: %v", err)
	}
}

// TestPrepareAndValidate_InvalidConfig — sing-box check падает → temp очищается,
// финальный бинарь /opt/sbin/sing-box не появляется (вызывающий не получит путь).
func TestPrepareAndValidate_InvalidConfig(t *testing.T) {
	tarPath := filepath.Join(t.TempDir(), "sb.tar.gz")
	_ = os.WriteFile(tarPath, makeTarball([]byte("fake-binary")), 0o644)

	configPath := filepath.Join(t.TempDir(), "config.json")

	// Mock: check возвращает ошибку.
	r := exectx.MockMatcher(func(cmd string, args ...string) (exectx.Result, error) {
		if len(args) > 0 && args[0] == "check" {
			return exectx.Result{ExitCode: 1, Stderr: []byte("bad config")}, fmt.Errorf("exit 1")
		}
		return exectx.Result{ExitCode: 0}, nil
	})

	params := DefaultConfigParams()
	params.Outbounds = []types.Outbound{{Tag: "direct", Type: "direct"}}
	params.DefaultOutboundTag = "direct"

	_, err := PrepareAndValidate(context.Background(), r, tarPath, configPath, params)
	if err == nil {
		t.Fatal("ожидалась ошибка валидации")
	}
}

func TestExtractBinary_Found(t *testing.T) {
	content := []byte("fake-sing-box-binary")
	tarPath := filepath.Join(t.TempDir(), "sing-box.tar.gz")
	_ = os.WriteFile(tarPath, makeTarball(content), 0o644)

	got, err := extractBinary(tarPath)
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("содержимое = %q, ожидалось %q", got, content)
	}
}

func TestExtractBinary_NotFound(t *testing.T) {
	// tarball без файла sing-box
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: 3})
	_, _ = tw.Write([]byte("hi\n"))
	_ = tw.Close()
	_ = gz.Close()

	tarPath := filepath.Join(t.TempDir(), "empty.tar.gz")
	_ = os.WriteFile(tarPath, buf.Bytes(), 0o644)

	_, err := extractBinary(tarPath)
	if err == nil {
		t.Fatal("ожидалась ошибка для архива без sing-box")
	}
}

func TestInstall_Success(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "sing-box.tar.gz")
	binDst := filepath.Join(dir, "sing-box")

	_ = os.WriteFile(tarPath, makeTarball([]byte("binary-v2")), 0o644)

	// mock runner — sing-box check возвращает успех
	runner := exectx.Mock(map[string]exectx.Result{
		binDst + " check -c " + filepath.Join(dir, "config.json"): {ExitCode: 0},
	})

	if err := Install(context.Background(), runner, tarPath, binDst, ""); err != nil {
		t.Fatalf("Install: %v", err)
	}

	got, _ := os.ReadFile(binDst)
	if string(got) != "binary-v2" {
		t.Errorf("установленный бинарь = %q, ожидалось %q", got, "binary-v2")
	}

	info, _ := os.Stat(binDst)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("права = %o, ожидалось 0755", info.Mode().Perm())
	}
}

func TestInstall_ConfigCheckFails_Rollback(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "sing-box.tar.gz")
	binDst := filepath.Join(dir, "sing-box")
	configPath := filepath.Join(dir, "config.json")

	// предустановленный "старый" бинарь
	_ = os.WriteFile(binDst, []byte("binary-v1"), 0o755)
	_ = os.WriteFile(tarPath, makeTarball([]byte("binary-v2")), 0o644)
	_ = os.WriteFile(configPath, []byte("{}"), 0o644)

	// mock runner — sing-box check падает
	runner := exectx.Mock(map[string]exectx.Result{
		binDst + " check -c " + configPath: {
			ExitCode: 1,
			Stderr:   []byte("invalid config"),
		},
	})

	err := Install(context.Background(), runner, tarPath, binDst, configPath)
	if err == nil {
		t.Fatal("ожидалась ошибка при падении sing-box check")
	}

	// после отката должен вернуться старый бинарь
	got, _ := os.ReadFile(binDst)
	if string(got) != "binary-v1" {
		t.Errorf("после отката бинарь = %q, ожидалось %q", got, "binary-v1")
	}
}

func TestBackupAndRestore(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "sing-box")
	_ = os.WriteFile(binPath, []byte("v1"), 0o755)

	backupPath, err := Backup(context.Background(), binPath)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if backupPath == "" {
		t.Fatal("ожидался непустой backupPath")
	}

	// заменяем бинарь "сломанной" версией
	_ = os.WriteFile(binPath, []byte("broken"), 0o755)

	if err := Restore(context.Background(), backupPath, binPath); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	got, _ := os.ReadFile(binPath)
	if string(got) != "v1" {
		t.Errorf("после Restore = %q, ожидалось %q", got, "v1")
	}
}

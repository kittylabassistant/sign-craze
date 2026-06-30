package singbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// makeTarball создаёт в памяти .tar.gz с одним файлом sing-box.
// Префиксует content ELF-magic, чтобы пройти проверку extractBinaryToFile.
func makeTarball(content []byte) []byte {
	full := append([]byte{0x7f, 'E', 'L', 'F'}, content...)
	return makeTarballRaw(full)
}

// makeTarballRaw кладёт content в архив без модификации.
func makeTarballRaw(content []byte) []byte {
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

// TestExtractBinary_RejectsNonELF — если файл sing-box в tarball не ELF,
// extractBinaryToFile должен вернуть error до записи бинаря на диск.
func TestExtractBinary_RejectsNonELF(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "sb.tar.gz")
	dstPath := filepath.Join(dir, "sing-box")
	_ = os.WriteFile(tarPath, makeTarballRaw([]byte("#!/bin/sh\necho fake")), 0o644)

	err := extractBinaryToFile(tarPath, dstPath, 0o755)
	if err == nil {
		t.Fatal("ожидалась ошибка для non-ELF binary")
	}
	if _, statErr := os.Stat(dstPath); statErr == nil {
		t.Errorf("dst создан несмотря на ошибку ELF-magic")
	}
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

	tempBin, err := PrepareAndValidate(context.Background(), r, t.TempDir(), tarPath, configPath, params)
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

	_, err := PrepareAndValidate(context.Background(), r, t.TempDir(), tarPath, configPath, params)
	if err == nil {
		t.Fatal("ожидалась ошибка валидации")
	}
}

func TestExtractBinary_Found(t *testing.T) {
	content := []byte("fake-sing-box-binary")
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "sing-box.tar.gz")
	dstPath := filepath.Join(dir, "sing-box")
	_ = os.WriteFile(tarPath, makeTarball(content), 0o644)

	if err := extractBinaryToFile(tarPath, dstPath, 0o755); err != nil {
		t.Fatalf("extractBinaryToFile: %v", err)
	}
	got, _ := os.ReadFile(dstPath)
	want := append([]byte{0x7f, 'E', 'L', 'F'}, content...)
	if !bytes.Equal(got, want) {
		t.Errorf("содержимое = %q, ожидалось %q", got, want)
	}
	info, _ := os.Stat(dstPath)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("права = %o, ожидалось 0755", info.Mode().Perm())
	}
}

// makeDynamicELF создаёт минимальный валидный ELF64 LE с PT_INTERP → interpPath.
// Дубликат helper'а из internal/elfcheck (разные пакеты) — ~25 строк приемлемо.
func makeDynamicELF(interpPath string) []byte {
	interp := append([]byte(interpPath), 0)
	var ident [16]byte
	copy(ident[:], []byte{0x7f, 'E', 'L', 'F', byte(elf.ELFCLASS64), byte(elf.ELFDATA2LSB), byte(elf.EV_CURRENT), 0})
	const phoff = 64
	hdr := elf.Header64{
		Ident: ident, Type: uint16(elf.ET_DYN), Machine: uint16(elf.EM_AARCH64),
		Version: uint32(elf.EV_CURRENT), Phoff: phoff, Ehsize: 64,
		Phentsize: 56, Phnum: 1, Shentsize: 64, Shnum: 0,
	}
	ph := elf.Prog64{
		Type: uint32(elf.PT_INTERP), Flags: uint32(elf.PF_R), Off: phoff + 56,
		Filesz: uint64(len(interp)), Memsz: uint64(len(interp)), Align: 1,
	}
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, hdr)
	_ = binary.Write(&buf, binary.LittleEndian, ph)
	buf.Write(interp)
	return buf.Bytes()
}

// TestPrepareAndValidate_DynamicBinaryRejected — распакованный бинарь динамически
// слинкован с отсутствующим интерпретатором (glibc-сборка на musl, issue #3).
// PrepareAndValidate обязан отклонить его ДО `sing-box check` с понятной ошибкой.
func TestPrepareAndValidate_DynamicBinaryRejected(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent-ld.so") // гарантированно не существует
	tarPath := filepath.Join(t.TempDir(), "sb.tar.gz")
	_ = os.WriteFile(tarPath, makeTarballRaw(makeDynamicELF(missing)), 0o644)

	configPath := filepath.Join(t.TempDir(), "config.json")

	checkCalled := false
	r := exectx.MockMatcher(func(_ string, args ...string) (exectx.Result, error) {
		if len(args) > 0 && args[0] == "check" {
			checkCalled = true
		}
		return exectx.Result{ExitCode: 0}, nil
	})

	params := DefaultConfigParams()
	params.Outbounds = []types.Outbound{{Tag: "direct", Type: "direct"}}
	params.DefaultOutboundTag = "direct"

	_, err := PrepareAndValidate(context.Background(), r, t.TempDir(), tarPath, configPath, params)
	if err == nil {
		t.Fatal("ожидалась ошибка для динамического бинаря с отсутствующим интерпретатором")
	}
	if !strings.Contains(err.Error(), "несовместимый") {
		t.Errorf("сообщение = %q, ожидалось содержащее 'несовместимый'", err.Error())
	}
	if checkCalled {
		t.Error("sing-box check не должен вызываться — отклонение до валидации конфига")
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

	dir := t.TempDir()
	tarPath := filepath.Join(dir, "empty.tar.gz")
	dstPath := filepath.Join(dir, "sing-box")
	_ = os.WriteFile(tarPath, buf.Bytes(), 0o644)

	err := extractBinaryToFile(tarPath, dstPath, 0o755)
	if err == nil {
		t.Fatal("ожидалась ошибка для архива без sing-box")
	}
}

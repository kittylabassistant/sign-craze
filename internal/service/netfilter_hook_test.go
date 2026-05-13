package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderHook_ContainsBinPath(t *testing.T) {
	data, err := RenderHook(HookParams{BinPath: "/opt/sbin/sign-craze"})
	if err != nil {
		t.Fatalf("RenderHook: %v", err)
	}
	if !strings.Contains(string(data), "/opt/sbin/sign-craze") {
		t.Errorf("hook не содержит путь к бинарю:\n%s", data)
	}
}

func TestRenderHook_DefaultBinPath(t *testing.T) {
	data, err := RenderHook(HookParams{})
	if err != nil {
		t.Fatalf("RenderHook: %v", err)
	}
	if !strings.Contains(string(data), DefaultSignCrazeBin) {
		t.Errorf("hook не содержит дефолтный путь %q:\n%s", DefaultSignCrazeBin, data)
	}
}

// TestRenderHook_FiltersByTypeTable — hook должен пропускать ненужные NDM-event'ы
// (например filter-table) чтобы не плодить лишние reapply-вызовы.
func TestRenderHook_FiltersByTypeTable(t *testing.T) {
	data, err := RenderHook(HookParams{BinPath: "/opt/sbin/sign-craze"})
	if err != nil {
		t.Fatalf("RenderHook: %v", err)
	}
	s := string(data)
	for _, needle := range []string{
		"iptables/mangle",
		"ip6tables/mangle",
		"ipset/",
	} {
		if !strings.Contains(s, needle) {
			t.Errorf("hook не содержит фильтр %q:\n%s", needle, s)
		}
	}
}

// TestRenderHook_EarlyExitOnNoService — hook должен выходить рано если ядро
// не запущено, иначе reapply пытался бы применить правила без TUN-интерфейса.
// PID-файл по умолчанию (sing-box) проверяется при пустом HookParams.PIDPath.
func TestRenderHook_EarlyExitOnNoService(t *testing.T) {
	data, err := RenderHook(HookParams{BinPath: "/opt/sbin/sign-craze"})
	if err != nil {
		t.Fatalf("RenderHook: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, DefaultCorePIDPath) {
		t.Errorf("hook должен проверять дефолтный PID-файл %q:\n%s", DefaultCorePIDPath, s)
	}
}

// TestRenderHook_CustomPIDPath — для xray/mihomo PID-путь подменяется.
func TestRenderHook_CustomPIDPath(t *testing.T) {
	data, err := RenderHook(HookParams{
		BinPath: "/opt/sbin/sign-craze",
		PIDPath: "/opt/var/run/sign-craze-xray.pid",
	})
	if err != nil {
		t.Fatalf("RenderHook: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "/opt/var/run/sign-craze-xray.pid") {
		t.Errorf("hook должен проверять переданный PID-файл:\n%s", s)
	}
	if strings.Contains(s, "sign-craze-singbox.pid") {
		t.Errorf("hook не должен содержать singbox PID при переданном xray:\n%s", s)
	}
}

// TestRenderHook_InvokesReapply — hook вызывает --reapply CLI-команду.
func TestRenderHook_InvokesReapply(t *testing.T) {
	data, err := RenderHook(HookParams{BinPath: "/opt/sbin/sign-craze"})
	if err != nil {
		t.Fatalf("RenderHook: %v", err)
	}
	if !strings.Contains(string(data), "--reapply") {
		t.Errorf("hook должен вызывать --reapply:\n%s", data)
	}
}

func TestWriteHook_CreatesExecutableFile(t *testing.T) {
	dir := t.TempDir()
	hookPath := filepath.Join(dir, "50-sign-craze")

	if err := WriteHook(hookPath, HookParams{BinPath: "/opt/sbin/sign-craze"}); err != nil {
		t.Fatalf("WriteHook: %v", err)
	}

	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("hook не создан: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("права hook = %o, ожидалось 0755", info.Mode().Perm())
	}
}

// TestWriteHook_Idempotent — вторая запись с тем же содержимым не должна
// перезаписывать файл (SHA256-гард). Иначе при каждом NDM rebuild атомарная
// запись через temp+rename создавала бы лишний I/O.
func TestWriteHook_Idempotent(t *testing.T) {
	dir := t.TempDir()
	hookPath := filepath.Join(dir, "50-sign-craze")
	p := HookParams{BinPath: "/opt/sbin/sign-craze"}

	if err := WriteHook(hookPath, p); err != nil {
		t.Fatalf("первый WriteHook: %v", err)
	}
	info1, _ := os.Stat(hookPath)

	if err := WriteHook(hookPath, p); err != nil {
		t.Fatalf("второй WriteHook: %v", err)
	}
	info2, _ := os.Stat(hookPath)

	if info1.ModTime() != info2.ModTime() {
		t.Error("WriteHook перезаписал файл без изменений (idempotency нарушена)")
	}
}

// TestRenderHook_ContainsFlockSerialize — шаблон должен содержать все маркеры
// flock-логики для сериализации параллельных NDM-событий на slow MIPS.
func TestRenderHook_ContainsFlockSerialize(t *testing.T) {
	data, err := RenderHook(HookParams{BinPath: "/opt/sbin/sign-craze"})
	if err != nil {
		t.Fatalf("RenderHook: %v", err)
	}
	s := string(data)
	// Основные маркеры flock-логики
	for _, marker := range []string{
		"flock -x -n 9",
		"LOCK=/opt/var/run/sign-craze-hook.lock",
		"PENDING=/opt/var/run/sign-craze-hook.pending",
		"sleep 2",
		"command -v flock",
	} {
		if !strings.Contains(s, marker) {
			t.Errorf("шаблон должен содержать %q, получено:\n%s", marker, s)
		}
	}
}

// TestRenderHook_TrailingDebouncePresent — --reapply должен вызываться дважды:
// первый раз сразу после захвата lock, второй — trailing после sleep 2
// если был pending-маркер от вытесненных конкурентов.
func TestRenderHook_TrailingDebouncePresent(t *testing.T) {
	data, err := RenderHook(HookParams{BinPath: "/opt/sbin/sign-craze"})
	if err != nil {
		t.Fatalf("%v", err)
	}
	s := string(data)
	// --reapply должен вызываться дважды (initial + trailing)
	if strings.Count(s, "--reapply") < 2 {
		t.Errorf("ожидалось минимум 2 вызова --reapply (initial + trailing), получено %d", strings.Count(s, "--reapply"))
	}
}

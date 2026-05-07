package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderShim_ContainsBinPath(t *testing.T) {
	p := ShimParams{BinPath: "/opt/sbin/sign-craze"}
	data, err := RenderShim(p)
	if err != nil {
		t.Fatalf("RenderShim: %v", err)
	}
	if !strings.Contains(string(data), "/opt/sbin/sign-craze") {
		t.Errorf("shim не содержит путь к бинарю:\n%s", data)
	}
}

func TestRenderShim_ContainsAllCases(t *testing.T) {
	data, err := RenderShim(ShimParams{BinPath: "/opt/sbin/sign-craze"})
	if err != nil {
		t.Fatalf("RenderShim: %v", err)
	}
	s := string(data)
	for _, keyword := range []string{"start", "stop", "restart", "status"} {
		if !strings.Contains(s, keyword) {
			t.Errorf("shim не содержит case %q:\n%s", keyword, s)
		}
	}
}

func TestRenderShim_DefaultBinPath(t *testing.T) {
	data, err := RenderShim(ShimParams{})
	if err != nil {
		t.Fatalf("RenderShim с пустым BinPath: %v", err)
	}
	if !strings.Contains(string(data), DefaultSignCrazeBin) {
		t.Errorf("shim не содержит дефолтный путь %q:\n%s", DefaultSignCrazeBin, data)
	}
}

// TestRenderShim_HasErrorHandling — шим должен использовать set -eu и логировать
// ошибки в boot.log, иначе init молча игнорирует фейл --service-start
// (safety-fixes #5).
func TestRenderShim_HasErrorHandling(t *testing.T) {
	data, err := RenderShim(ShimParams{BinPath: "/opt/sbin/sign-craze"})
	if err != nil {
		t.Fatalf("RenderShim: %v", err)
	}
	s := string(data)

	if !strings.Contains(s, "set -eu") {
		t.Errorf("shim должен начинаться с set -eu:\n%s", s)
	}
	if !strings.Contains(s, "/opt/var/log/sign-craze/boot.log") {
		t.Errorf("shim должен логировать в boot.log:\n%s", s)
	}
	// Должен быть код, выводящий ошибку и завершающийся с exit 1
	if !strings.Contains(s, "exit 1") {
		t.Errorf("shim должен явно exit 1 при ошибке:\n%s", s)
	}
}

// TestRenderShim_NoNonExistentCommands — shim должен ссылаться только на
// зарегистрированные CLI команды. Раньше использовались --service-stop /
// --service-restart / --service-status, которых не существует — init.d stop
// тихо падал.
func TestRenderShim_NoNonExistentCommands(t *testing.T) {
	data, err := RenderShim(ShimParams{BinPath: "/opt/sbin/sign-craze"})
	if err != nil {
		t.Fatalf("RenderShim: %v", err)
	}
	s := string(data)
	for _, bad := range []string{"--service-stop", "--service-restart", "--service-status"} {
		if strings.Contains(s, bad) {
			t.Errorf("shim ссылается на несуществующую команду %q (используйте --stop/--restart/--status):\n%s", bad, s)
		}
	}
}

// TestRenderShim_HasWatchdogIntegration — shim должен запускать
// --service-watchdog в фоне на start и убивать на stop, чтобы watchdog пережил
// ребут роутера (без него watchdog существует только в --ui процессе, который
// не входит в init.d boot path).
func TestRenderShim_HasWatchdogIntegration(t *testing.T) {
	data, err := RenderShim(ShimParams{BinPath: "/opt/sbin/sign-craze"})
	if err != nil {
		t.Fatalf("RenderShim: %v", err)
	}
	s := string(data)

	for _, marker := range []string{
		"--service-watchdog",
		"start_watchdog",
		"stop_watchdog",
		"WATCHDOG_PID=/opt/var/run/sign-craze-watchdog.pid",
		"nohup",
	} {
		if !strings.Contains(s, marker) {
			t.Errorf("shim не содержит %q (требуется для watchdog persistence через ребут):\n%s", marker, s)
		}
	}
}

func TestWriteShim_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	shimPath := filepath.Join(dir, "S99signcraze")

	if err := WriteShim(shimPath, ShimParams{BinPath: "/opt/sbin/sign-craze"}); err != nil {
		t.Fatalf("WriteShim: %v", err)
	}

	info, err := os.Stat(shimPath)
	if err != nil {
		t.Fatalf("shim не создан: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("права shim = %o, ожидалось 0755", info.Mode().Perm())
	}
}

func TestWriteShim_Idempotent(t *testing.T) {
	dir := t.TempDir()
	shimPath := filepath.Join(dir, "S99signcraze")
	p := ShimParams{BinPath: "/opt/sbin/sign-craze"}

	if err := WriteShim(shimPath, p); err != nil {
		t.Fatalf("первый WriteShim: %v", err)
	}

	info1, _ := os.Stat(shimPath)

	// вторая запись — содержимое не изменилось, mtime не должен меняться
	if err := WriteShim(shimPath, p); err != nil {
		t.Fatalf("второй WriteShim: %v", err)
	}

	info2, _ := os.Stat(shimPath)
	if info1.ModTime() != info2.ModTime() {
		t.Error("WriteShim перезаписал файл без изменений (idempotency нарушена)")
	}
}

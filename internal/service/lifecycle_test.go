package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWriteReadPID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	if err := writePID(path, 12345); err != nil {
		t.Fatalf("writePID: %v", err)
	}

	pid, err := readPID(path)
	if err != nil {
		t.Fatalf("readPID: %v", err)
	}
	if pid != 12345 {
		t.Errorf("pid = %d, ожидалось 12345", pid)
	}
}

func TestReadPID_MissingFile(t *testing.T) {
	pid, err := readPID(filepath.Join(t.TempDir(), "nonexistent.pid"))
	if err != nil {
		t.Fatalf("readPID несуществующего файла: %v", err)
	}
	if pid != 0 {
		t.Errorf("ожидался PID=0, получили %d", pid)
	}
}

func TestReadPID_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.pid")
	_ = os.WriteFile(path, []byte("not-a-number\n"), 0o644)

	_, err := readPID(path)
	if err == nil {
		t.Fatal("ожидалась ошибка для невалидного PID")
	}
}

func TestProcessAlive_CurrentProcess(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("текущий процесс должен быть alive")
	}
}

func TestProcessAlive_DeadPID(t *testing.T) {
	// PID 99999999 почти наверняка не существует
	if processAlive(99999999) {
		t.Skip("PID 99999999 неожиданно существует — пропускаем тест")
	}
}

func TestStatus_NoPIDFile(t *testing.T) {
	lc := NewLifecycle(ProcessConfig{
		Name:    "test",
		PIDFile: filepath.Join(t.TempDir(), "test.pid"),
	})

	st, err := lc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Running {
		t.Error("ожидался Running=false при отсутствии PID-файла")
	}
}

func TestStatus_StalePIDFile(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "test.pid")

	// записываем несуществующий PID
	_ = os.WriteFile(pidFile, []byte("99999999\n"), 0o644)

	lc := NewLifecycle(ProcessConfig{
		Name:    "test",
		PIDFile: pidFile,
	})

	st, err := lc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Running {
		t.Error("ожидался Running=false для устаревшего PID-файла")
	}

	// устаревший PID-файл должен быть удалён
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("устаревший PID-файл не удалён")
	}
}

func TestLifecycle_StartStop(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("тест ненадёжен под root (Setpgid может вести себя иначе)")
	}

	dir := t.TempDir()
	pidFile := filepath.Join(dir, "sleep.pid")

	lc := NewLifecycle(ProcessConfig{
		Name:    "sleep",
		BinPath: "/bin/sleep",
		Args:    []string{"30"},
		PIDFile: pidFile,
	})

	ctx := context.Background()

	if err := lc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// проверяем что процесс запущен
	st, err := lc.Status(ctx)
	if err != nil {
		t.Fatalf("Status после Start: %v", err)
	}
	if !st.Running {
		t.Fatal("ожидался Running=true после Start")
	}

	pid := st.PID

	if err := lc.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// даём процессу умереть
	time.Sleep(200 * time.Millisecond)

	if processAlive(pid) {
		t.Errorf("процесс %d всё ещё жив после Stop", pid)
	}
}

// TestLifecycle_StartCrashesEarly — процесс падает с exit(1) сразу после старта.
// Stabilization-проверка должна это поймать и вернуть error, иначе Start()
// возвращал бы success для уже мёртвого процесса.
func TestLifecycle_StartCrashesEarly(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("тест ненадёжен под root")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "false.pid")

	lc := NewLifecycle(ProcessConfig{
		Name:    "false",
		BinPath: "/bin/false", // exit(1) немедленно
		Args:    nil,
		PIDFile: pidFile,
	})

	err := lc.Start(context.Background())
	if err == nil {
		t.Fatal("ожидалась ошибка для процесса, упавшего сразу после старта")
	}
	if !strings.Contains(err.Error(), "упал") {
		t.Errorf("сообщение ошибки = %q, ожидалось содержащее 'упал'", err.Error())
	}
	if _, statErr := os.Stat(pidFile); statErr == nil {
		t.Error("PID-файл не должен оставаться после неудачного старта")
	}
}

func TestLifecycle_StartAlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "sleep.pid")

	lc := NewLifecycle(ProcessConfig{
		Name:    "sleep",
		BinPath: "/bin/sleep",
		Args:    []string{"30"},
		PIDFile: pidFile,
	})

	ctx := context.Background()
	if err := lc.Start(ctx); err != nil {
		t.Fatalf("первый Start: %v", err)
	}
	defer lc.Stop(ctx)

	err := lc.Start(ctx)
	if err == nil {
		t.Fatal("ожидалась ошибка при повторном Start")
	}
	if !strings.Contains(err.Error(), "уже запущен") {
		t.Errorf("сообщение ошибки = %q, ожидалось содержащее 'уже запущен'", err.Error())
	}
}

func TestLifecycle_StopNotRunning(t *testing.T) {
	lc := NewLifecycle(ProcessConfig{
		Name:    "ghost",
		PIDFile: filepath.Join(t.TempDir(), "ghost.pid"),
	})

	// не должен возвращать ошибку
	if err := lc.Stop(context.Background()); err != nil {
		t.Errorf("Stop незапущенного сервиса: %v", err)
	}
}

func TestDirOf(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/opt/var/run/sign-craze-singbox.pid", "/opt/var/run"},
		{"pid.file", "."},
		{"/tmp/x.pid", "/tmp"},
	}
	for _, tt := range tests {
		if got := dirOf(tt.path); got != tt.want {
			t.Errorf("dirOf(%q) = %q, ожидалось %q", tt.path, got, tt.want)
		}
	}
}

// TestWritePID_MkdirAll проверяет, что writePID создаёт недостающие директории.
func TestWritePID_MkdirAll(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "sub", "dir", "test.pid")

	if err := writePID(pidFile, 42); err != nil {
		t.Fatalf("writePID с вложенными директориями: %v", err)
	}

	data, _ := os.ReadFile(pidFile)
	if strings.TrimSpace(string(data)) != strconv.Itoa(42) {
		t.Errorf("содержимое PID-файла = %q, ожидалось %q", data, "42")
	}
}

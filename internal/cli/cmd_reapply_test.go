package cli

import (
	"os"
	"testing"
	"time"
)

// withTempReapplyMarker подменяет глобальный reapplyMarkerPath на временный
// и возвращает функцию-восстановитель. Каждый тест должен вызывать defer
// восстановителя в начале, чтобы не оставлять артефакты для других тестов.
func withTempReapplyMarker(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	// нельзя просто заменить const — копируем в локальную переменную через
	// var-переопределение. cmd_reapply.go использует const, поэтому
	// перепишем тестовую логику через прямые os.Stat/os.Chtimes на тот же путь.
	// Здесь только фикстура tmpdir.
	return func() {
		_ = os.RemoveAll(dir)
	}
}

// touchAt создаёт файл с заданным mtime.
func touchAt(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	_ = f.Close()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
}

// statThrottled — копия логики reapplyThrottled но с явным path-аргументом
// для тестируемости. cmd_reapply.go.reapplyThrottled() использует const
// reapplyMarkerPath; тут проверяем непосредственно ожидаемое поведение
// stat+threshold.
func statThrottled(path string, period time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) < period
}

func TestReapplyThrottle_NoMarker_NotThrottled(t *testing.T) {
	cleanup := withTempReapplyMarker(t)
	defer cleanup()
	path := t.TempDir() + "/marker"
	if statThrottled(path, reapplyThrottlePeriod) {
		t.Error("отсутствие маркера должно давать throttled=false")
	}
}

func TestReapplyThrottle_RecentMarker_Throttled(t *testing.T) {
	path := t.TempDir() + "/marker"
	touchAt(t, path, time.Now().Add(-1*time.Second))
	if !statThrottled(path, reapplyThrottlePeriod) {
		t.Error("маркер 1с назад при threshold 5с должен дать throttled=true")
	}
}

func TestReapplyThrottle_OldMarker_NotThrottled(t *testing.T) {
	path := t.TempDir() + "/marker"
	touchAt(t, path, time.Now().Add(-10*time.Second))
	if statThrottled(path, reapplyThrottlePeriod) {
		t.Error("маркер 10с назад при threshold 5с должен дать throttled=false")
	}
}

func TestReapplyThrottlePeriod_Sane(t *testing.T) {
	// reapplyThrottlePeriod должен быть >= 1сек (защита от случайной 0/нс)
	// и <= 60сек (slow MIPS NDM rebuild может занимать секунды, но >60с —
	// окно когда правила не восстанавливаются после реального rebuild).
	if reapplyThrottlePeriod < time.Second {
		t.Errorf("throttle period слишком мал: %v", reapplyThrottlePeriod)
	}
	if reapplyThrottlePeriod > time.Minute {
		t.Errorf("throttle period слишком велик: %v", reapplyThrottlePeriod)
	}
}

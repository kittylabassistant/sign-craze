package firewall

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

func TestIsModuleLoaded_NotLoaded(t *testing.T) {
	// Используем несуществующий модуль — не должен быть в /proc/modules.
	if IsModuleLoaded("xt_nonexistent_sign_craze_test_module") {
		t.Error("ожидалось false для несуществующего модуля")
	}
}

func TestIsModuleLoaded_EmptyName(t *testing.T) {
	// Пустое имя не должно матчиться ни на одну строку.
	if IsModuleLoaded("") {
		t.Error("IsModuleLoaded(\"\") должна возвращать false")
	}
}

func TestEnsureKernelModule_AlreadyLoaded(t *testing.T) {
	// Находим любой реально загруженный модуль из /proc/modules и проверяем no-op.
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		t.Skip("пропуск: /proc/modules недоступен")
	}
	lines := splitLines(string(data))
	if len(lines) == 0 {
		t.Skip("пропуск: /proc/modules пуст")
	}
	// Берём первый реальный модуль.
	name := firstModuleName(lines)
	if name == "" {
		t.Skip("пропуск: не удалось разобрать /proc/modules")
	}

	// Runner не должен вызываться — модуль уже загружен.
	called := false
	runner := exectx.MockMatcher(func(n string, args ...string) (exectx.Result, error) {
		called = true
		return exectx.Result{ExitCode: 1, Stderr: []byte("не должно вызываться")}, nil
	})

	if err := EnsureKernelModule(context.Background(), runner, name, "/nonexistent/path.ko"); err != nil {
		t.Errorf("EnsureKernelModule для уже загруженного модуля вернул ошибку: %v", err)
	}
	if called {
		t.Error("insmod не должен вызываться если модуль уже загружен")
	}
}

func TestEnsureKernelModule_PathMissing(t *testing.T) {
	// Путь не существует → ошибка.
	runner := exectx.MockMatcher(func(n string, args ...string) (exectx.Result, error) {
		return exectx.Result{ExitCode: 0}, nil
	})

	err := EnsureKernelModule(context.Background(), runner, "xt_nonexistent_sign_craze", "/no/such/path.ko")
	if err == nil {
		t.Error("EnsureKernelModule с несуществующим путём должен вернуть ошибку")
	}
}

func TestEnsureKernelModule_Insmod_FileExists(t *testing.T) {
	// Создаём временный .ko-файл чтобы os.Stat не зафейлился.
	tmp := filepath.Join(t.TempDir(), "xt_test.ko")
	if err := os.WriteFile(tmp, []byte("fake"), 0o644); err != nil {
		t.Fatalf("не удалось создать временный файл: %v", err)
	}

	// Runner возвращает "File exists" в stderr — должен вернуть nil (race).
	runner := exectx.MockMatcher(func(n string, args ...string) (exectx.Result, error) {
		return exectx.Result{ExitCode: 1, Stderr: []byte("insmod: ERROR: could not insert 'xt_test': File exists")},
			errFakeInsmod
	})

	err := EnsureKernelModule(context.Background(), runner, "xt_sign_craze_not_loaded_module", tmp)
	if err != nil {
		t.Errorf("EnsureKernelModule: 'File exists' stderr должен возвращать nil, получено: %v", err)
	}
}

func TestEnsureKernelModule_Insmod_AlreadyLoaded(t *testing.T) {
	// Runner возвращает "already loaded" в stderr — должен вернуть nil.
	tmp := filepath.Join(t.TempDir(), "xt_test.ko")
	if err := os.WriteFile(tmp, []byte("fake"), 0o644); err != nil {
		t.Fatalf("не удалось создать временный файл: %v", err)
	}

	runner := exectx.MockMatcher(func(n string, args ...string) (exectx.Result, error) {
		return exectx.Result{ExitCode: 1, Stderr: []byte("already loaded")}, errFakeInsmod
	})

	err := EnsureKernelModule(context.Background(), runner, "xt_sign_craze_not_loaded_module", tmp)
	if err != nil {
		t.Errorf("EnsureKernelModule: 'already loaded' stderr должен возвращать nil, получено: %v", err)
	}
}

func TestEnsureKernelModule_Insmod_RealError(t *testing.T) {
	// Runner возвращает ошибку без "File exists" → должна пробрасываться.
	tmp := filepath.Join(t.TempDir(), "xt_test.ko")
	if err := os.WriteFile(tmp, []byte("fake"), 0o644); err != nil {
		t.Fatalf("не удалось создать временный файл: %v", err)
	}

	runner := exectx.MockMatcher(func(n string, args ...string) (exectx.Result, error) {
		return exectx.Result{ExitCode: 1, Stderr: []byte("invalid module format")}, errFakeInsmod
	})

	err := EnsureKernelModule(context.Background(), runner, "xt_sign_craze_not_loaded_module", tmp)
	if err == nil {
		t.Error("EnsureKernelModule: реальная ошибка insmod должна возвращать ошибку")
	}
}

func TestFindKernelModule_GlobNoMatch(t *testing.T) {
	// Нет модуля с таким именем → пустая строка.
	result := FindKernelModule("xt_nonexistent_sign_craze_test_module_xyz")
	if result != "" {
		t.Errorf("FindKernelModule для несуществующего модуля: ожидалась пустая строка, получено %q", result)
	}
}

// errFakeInsmod — фиктивная ошибка для использования в MockMatcher.
var errFakeInsmod = &fakeError{"fake insmod error"}

type fakeError struct{ msg string }

func (e *fakeError) Error() string { return e.msg }

// splitLines возвращает непустые строки.
func splitLines(s string) []string {
	var result []string
	for _, line := range splitByNewline(s) {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func splitByNewline(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

func firstModuleName(lines []string) string {
	for _, line := range lines {
		fields := splitFields(line)
		if len(fields) > 0 && fields[0] != "" {
			return fields[0]
		}
	}
	return ""
}

func splitFields(s string) []string {
	var fields []string
	inField := false
	start := 0
	for i := 0; i <= len(s); i++ {
		isSpace := i == len(s) || s[i] == ' ' || s[i] == '\t'
		if inField && isSpace {
			fields = append(fields, s[start:i])
			inField = false
		} else if !inField && !isSpace {
			inField = true
			start = i
		}
	}
	return fields
}

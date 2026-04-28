package firewall

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

func TestSaveIPSet_AtomicWrite(t *testing.T) {
	dumpFile := filepath.Join(t.TempDir(), "ipset.dump")
	r := exectx.MockMatcher(func(name string, args ...string) (exectx.Result, error) {
		if name != "ipset" || len(args) < 1 || args[0] != "save" {
			return exectx.Result{ExitCode: 1}, nil
		}
		return exectx.Result{Stdout: []byte("create signcraze_ipv4 hash:net family inet\nadd signcraze_ipv4 1.1.1.0/24\n")}, nil
	})

	err := SaveIPSet(context.Background(), r, dumpFile, []string{"signcraze_ipv4"})
	if err != nil {
		t.Fatalf("SaveIPSet: %v", err)
	}

	data, err := os.ReadFile(dumpFile)
	if err != nil {
		t.Fatalf("dump не записан: %v", err)
	}
	if len(data) == 0 {
		t.Error("dump пуст")
	}
}

func TestSaveIPSet_EmptyNamesError(t *testing.T) {
	r := exectx.MockMatcher(func(_ string, _ ...string) (exectx.Result, error) {
		return exectx.Result{}, nil
	})
	if err := SaveIPSet(context.Background(), r, "/tmp/x", nil); err == nil {
		t.Error("ожидалась ошибка при пустом списке")
	}
}

// TestRestoreIPSet_MissingFileNoOp проверяет что отсутствие dump-файла
// не считается ошибкой — это нормальный случай первого запуска до --update-geo.
func TestRestoreIPSet_MissingFileNoOp(t *testing.T) {
	r := exectx.MockMatcher(func(_ string, _ ...string) (exectx.Result, error) {
		t.Error("runner.Run не должен вызываться при отсутствии файла")
		return exectx.Result{}, nil
	})
	if err := RestoreIPSet(context.Background(), r, "/non/existent/path"); err != nil {
		t.Errorf("ожидался nil, получено %v", err)
	}
}

func TestRestoreIPSet_EmptyFileNoOp(t *testing.T) {
	dumpFile := filepath.Join(t.TempDir(), "empty.dump")
	_ = os.WriteFile(dumpFile, []byte{}, 0o644)

	called := false
	r := exectx.MockMatcher(func(_ string, _ ...string) (exectx.Result, error) {
		called = true
		return exectx.Result{}, nil
	})
	if err := RestoreIPSet(context.Background(), r, dumpFile); err != nil {
		t.Fatalf("RestoreIPSet: %v", err)
	}
	if called {
		t.Error("runner не должен вызываться для пустого dump")
	}
}

func TestRestoreIPSet_CallsRestoreWithFile(t *testing.T) {
	dumpFile := filepath.Join(t.TempDir(), "ipset.dump")
	_ = os.WriteFile(dumpFile, []byte("create x hash:net\n"), 0o644)

	gotArgs := []string{}
	r := exectx.MockMatcher(func(name string, args ...string) (exectx.Result, error) {
		if name == "ipset" {
			gotArgs = append([]string{}, args...)
		}
		return exectx.Result{}, nil
	})
	if err := RestoreIPSet(context.Background(), r, dumpFile); err != nil {
		t.Fatalf("RestoreIPSet: %v", err)
	}
	// Должно быть: restore -! -f <dumpFile>
	if len(gotArgs) != 4 || gotArgs[0] != "restore" || gotArgs[1] != "-!" || gotArgs[2] != "-f" || gotArgs[3] != dumpFile {
		t.Errorf("неверные аргументы: %v", gotArgs)
	}
}

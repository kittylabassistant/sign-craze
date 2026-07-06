package corearchive

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestBinDst возвращает путь к несуществующему файлу-бинарю во временной
// директории теста.
func newTestBinDst(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "core-bin")
}

// TestInstallWithRollback_HappyPath — Opener отдаёт валидный поток,
// CheckConfig проходит: бинарь записан, CheckConfig вызван ровно один раз.
func TestInstallWithRollback_HappyPath(t *testing.T) {
	binDst := newTestBinDst(t)
	content := []byte("new-binary-content")
	checkCalls := 0

	err := InstallWithRollback(context.Background(), InstallParams{
		Label:       "testcore",
		ArchivePath: "irrelevant.archive",
		BinDst:      binDst,
		Opener: func() (*BinaryStream, error) {
			return NewBinaryStream(bytes.NewReader(content), func() error { return nil }), nil
		},
		CheckConfig: func(ctx context.Context) error {
			checkCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("InstallWithRollback: %v", err)
	}
	if checkCalls != 1 {
		t.Errorf("CheckConfig вызван %d раз(а), ожидался 1", checkCalls)
	}
	got, readErr := os.ReadFile(binDst)
	if readErr != nil {
		t.Fatalf("чтение binDst: %v", readErr)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("binDst = %q, want %q", got, content)
	}
}

// TestInstallWithRollback_NilCheckConfigSkipsValidation — CheckConfig == nil
// (аналог пустого configPath у caller'а) — установка проходит без валидации.
func TestInstallWithRollback_NilCheckConfigSkipsValidation(t *testing.T) {
	binDst := newTestBinDst(t)
	content := []byte("content-no-validation")

	err := InstallWithRollback(context.Background(), InstallParams{
		Label:       "testcore",
		ArchivePath: "irrelevant.archive",
		BinDst:      binDst,
		Opener: func() (*BinaryStream, error) {
			return NewBinaryStream(bytes.NewReader(content), func() error { return nil }), nil
		},
		CheckConfig: nil,
	})
	if err != nil {
		t.Fatalf("InstallWithRollback: %v", err)
	}
	got, readErr := os.ReadFile(binDst)
	if readErr != nil {
		t.Fatalf("чтение binDst: %v", readErr)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("binDst = %q, want %q", got, content)
	}
}

// TestInstallWithRollback_OpenerError — ошибка Opener (напр. ELF-magic
// reject внутри конкретного ядра) должна остановить установку ДО записи в
// binDst и не должна вызывать CheckConfig.
func TestInstallWithRollback_OpenerError(t *testing.T) {
	binDst := newTestBinDst(t)
	sentinel := errors.New("содержимое не ELF-бинарь (magic=deadbeef)")
	checkCalls := 0

	err := InstallWithRollback(context.Background(), InstallParams{
		Label:       "testcore",
		ArchivePath: "bad.archive",
		BinDst:      binDst,
		Opener: func() (*BinaryStream, error) {
			return nil, sentinel
		},
		CheckConfig: func(ctx context.Context) error {
			checkCalls++
			return nil
		},
	})
	if err == nil {
		t.Fatal("ожидалась ошибка от Opener")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("ошибка должна оборачивать sentinel через %%w: %v", err)
	}
	if !strings.Contains(err.Error(), "testcore install:") {
		t.Errorf("ошибка должна содержать префикс %q: %v", "testcore install:", err)
	}
	if checkCalls != 0 {
		t.Errorf("CheckConfig не должен вызываться при ошибке Opener, вызван %d раз(а)", checkCalls)
	}
	if _, statErr := os.Stat(binDst); !os.IsNotExist(statErr) {
		t.Error("binDst не должен быть создан при ошибке Opener")
	}
}

// TestInstallWithRollback_RollbackOnFailedCheckConfig — при уже
// установленном бинаре: новый бинарь пишется, CheckConfig проваливается →
// старое содержимое должно быть восстановлено (rollback), ошибка упоминает
// проверку конфига.
func TestInstallWithRollback_RollbackOnFailedCheckConfig(t *testing.T) {
	binDst := newTestBinDst(t)
	oldContent := []byte("old-binary-content-v1")
	if err := os.WriteFile(binDst, oldContent, 0o755); err != nil {
		t.Fatalf("подготовка старого бинаря: %v", err)
	}
	newContent := []byte("new-binary-content-BROKEN-CONFIG")
	checkErr := errors.New("конфиг сломан")

	err := InstallWithRollback(context.Background(), InstallParams{
		Label:       "testcore",
		ArchivePath: "irrelevant.archive",
		BinDst:      binDst,
		Opener: func() (*BinaryStream, error) {
			return NewBinaryStream(bytes.NewReader(newContent), func() error { return nil }), nil
		},
		CheckConfig: func(ctx context.Context) error {
			return checkErr
		},
	})
	if err == nil {
		t.Fatal("ожидалась ошибка от CheckConfig")
	}
	if !errors.Is(err, checkErr) {
		t.Errorf("ошибка должна оборачивать checkErr через %%w: %v", err)
	}
	if !strings.Contains(err.Error(), "проверка конфига") {
		t.Errorf("текст ошибки должен упоминать проверку конфига: %v", err)
	}
	got, readErr := os.ReadFile(binDst)
	if readErr != nil {
		t.Fatalf("чтение binDst после отката: %v", readErr)
	}
	if !bytes.Equal(got, oldContent) {
		t.Errorf("после отката binDst = %q, want %q (старое содержимое)", got, oldContent)
	}
}

// TestInstallWithRollback_ClosesStreamAfterUse — BinaryStream.Close()
// вызывается независимо от исхода установки (в т.ч. на happy path).
func TestInstallWithRollback_ClosesStreamAfterUse(t *testing.T) {
	binDst := newTestBinDst(t)
	closed := false

	err := InstallWithRollback(context.Background(), InstallParams{
		Label:       "testcore",
		ArchivePath: "irrelevant.archive",
		BinDst:      binDst,
		Opener: func() (*BinaryStream, error) {
			return NewBinaryStream(bytes.NewReader([]byte("x")), func() error {
				closed = true
				return nil
			}), nil
		},
	})
	if err != nil {
		t.Fatalf("InstallWithRollback: %v", err)
	}
	if !closed {
		t.Error("stream.Close() должен быть вызван после установки")
	}
}

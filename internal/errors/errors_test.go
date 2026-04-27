package errors

import (
	stderrors "errors"
	"testing"
)

func TestSentinels_Distinct(t *testing.T) {
	sentinels := []error{
		ErrNotInstalled,
		ErrAlreadyInstalled,
		ErrLockHeld,
		ErrCoreDown,
		ErrDPIDown,
		ErrAlreadyRunning,
		ErrNotRunning,
		ErrConfigInvalid,
		ErrChecksumMismatch,
		ErrNoSpace,
		ErrUnsupportedArch,
	}
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && stderrors.Is(a, b) {
				t.Errorf("sentinel[%d] ошибочно совпадает с sentinel[%d]: %v == %v", i, j, a, b)
			}
		}
	}
}

func TestWrap_UnwrapsToSentinel(t *testing.T) {
	wrapped := Wrap("singbox install", ErrNotInstalled)
	if !stderrors.Is(wrapped, ErrNotInstalled) {
		t.Errorf("errors.Is не видит sentinel через Wrap: %v", wrapped)
	}
}

func TestWrap_MessageContainsContext(t *testing.T) {
	wrapped := Wrap("singbox install", ErrNotInstalled)
	msg := wrapped.Error()
	if msg != "singbox install: не установлен" {
		t.Errorf("неожиданное сообщение: %q", msg)
	}
}

func TestWrap_DoubleWrap(t *testing.T) {
	inner := Wrap("download", ErrChecksumMismatch)
	outer := Wrap("install", inner)
	if !stderrors.Is(outer, ErrChecksumMismatch) {
		t.Error("errors.Is не видит sentinel через двойной Wrap")
	}
}

func TestJoin_MultipleErrors(t *testing.T) {
	err := Join(ErrNotInstalled, ErrLockHeld)
	if !stderrors.Is(err, ErrNotInstalled) {
		t.Error("Join: ErrNotInstalled не найден")
	}
	if !stderrors.Is(err, ErrLockHeld) {
		t.Error("Join: ErrLockHeld не найден")
	}
}

func TestJoin_AllNil(t *testing.T) {
	if err := Join(nil, nil); err != nil {
		t.Errorf("Join(nil, nil) должен вернуть nil, получили %v", err)
	}
}

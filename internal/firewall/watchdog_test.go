package firewall

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchdog_ВызываетReconcileПоТику(t *testing.T) {
	var calls atomic.Int32
	w := NewWatchdog(20*time.Millisecond, func(_ context.Context) error {
		calls.Add(1)
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	got := calls.Load()
	if got < 3 || got > 6 {
		t.Errorf("ожидалось 3-6 вызовов за 100ms (interval 20ms), получено %d", got)
	}
}

func TestWatchdog_ОшибкиReconcileНеПрерываютЦикл(t *testing.T) {
	var calls atomic.Int32
	w := NewWatchdog(15*time.Millisecond, func(_ context.Context) error {
		calls.Add(1)
		return errors.New("симулированная ошибка")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	got := calls.Load()
	if got < 3 {
		t.Errorf("ожидалось >=3 вызовов даже при ошибках, получено %d", got)
	}
}

func TestWatchdog_NilCallbackВыходитСразу(t *testing.T) {
	w := NewWatchdog(10*time.Millisecond, nil)
	done := make(chan struct{})
	go func() {
		w.Run(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Run должен вернуться сразу при nil callback")
	}
}

func TestWatchdog_ОстанавливаетсяПоCtxCancel(t *testing.T) {
	w := NewWatchdog(10*time.Millisecond, func(_ context.Context) error {
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	time.Sleep(25 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Run не остановился после cancel")
	}
}

func TestNewWatchdog_ZeroIntervalUseDefault(t *testing.T) {
	w := NewWatchdog(0, func(_ context.Context) error { return nil })
	if w.interval != DefaultWatchdogInterval {
		t.Errorf("interval = %v, ожидался DefaultWatchdogInterval %v", w.interval, DefaultWatchdogInterval)
	}
}

func TestNewWatchdog_NegativeIntervalUseDefault(t *testing.T) {
	w := NewWatchdog(-1*time.Second, func(_ context.Context) error { return nil })
	if w.interval != DefaultWatchdogInterval {
		t.Errorf("interval = %v, ожидался default", w.interval)
	}
}

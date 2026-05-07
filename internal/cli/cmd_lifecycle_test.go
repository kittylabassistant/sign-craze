package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/service"
)

// fakeUILifecycle — минимальная реализация service.Lifecycle для тестов
// перезапуска UI: фиксирует вызовы Start/Stop и возвращает заданный Status.
type fakeUILifecycle struct {
	running bool

	statusErr error
	stopErr   error
	startErr  error

	startCalls int
	stopCalls  int
}

func (f *fakeUILifecycle) Start(_ context.Context) error {
	f.startCalls++
	if f.startErr != nil {
		return f.startErr
	}
	f.running = true
	return nil
}

func (f *fakeUILifecycle) Stop(_ context.Context) error {
	f.stopCalls++
	if f.stopErr != nil {
		return f.stopErr
	}
	f.running = false
	return nil
}

func (f *fakeUILifecycle) Restart(ctx context.Context) error {
	if err := f.Stop(ctx); err != nil {
		return err
	}
	return f.Start(ctx)
}

func (f *fakeUILifecycle) Status(_ context.Context) (service.Status, error) {
	if f.statusErr != nil {
		return service.Status{}, f.statusErr
	}
	if !f.running {
		return service.Status{}, nil
	}
	return service.Status{Running: true, PID: 4242}, nil
}

func withFakeUILifecycle(t *testing.T, fake *fakeUILifecycle) {
	t.Helper()
	prev := uiLifecycleFn
	uiLifecycleFn = func() (service.Lifecycle, error) { return fake, nil }
	t.Cleanup(func() { uiLifecycleFn = prev })
}

func TestStopUIIfRunning_RunningProcessGetsStopped(t *testing.T) {
	fake := &fakeUILifecycle{running: true}
	withFakeUILifecycle(t, fake)

	if !stopUIIfRunning(context.Background()) {
		t.Fatal("ожидалось true для запущенного UI")
	}
	if fake.stopCalls != 1 {
		t.Errorf("stop вызван %d раз, ожидалось 1", fake.stopCalls)
	}
	if fake.running {
		t.Error("UI должен быть остановлен")
	}
}

func TestStopUIIfRunning_NotRunning(t *testing.T) {
	fake := &fakeUILifecycle{running: false}
	withFakeUILifecycle(t, fake)

	if stopUIIfRunning(context.Background()) {
		t.Fatal("ожидалось false для не запущенного UI")
	}
	if fake.stopCalls != 0 {
		t.Errorf("stop не должен вызываться, вызван %d раз", fake.stopCalls)
	}
}

func TestStopUIIfRunning_StatusErrorReturnsFalse(t *testing.T) {
	fake := &fakeUILifecycle{statusErr: errors.New("permission denied")}
	withFakeUILifecycle(t, fake)

	if stopUIIfRunning(context.Background()) {
		t.Fatal("при ошибке Status ожидалось false (не пытаемся стопать)")
	}
	if fake.stopCalls != 0 {
		t.Errorf("stop не должен вызываться при ошибке Status, вызван %d раз", fake.stopCalls)
	}
}

func TestStartUI_DelegatesToLifecycle(t *testing.T) {
	fake := &fakeUILifecycle{}
	withFakeUILifecycle(t, fake)

	if err := startUI(context.Background()); err != nil {
		t.Fatalf("startUI: %v", err)
	}
	if fake.startCalls != 1 {
		t.Errorf("start вызван %d раз, ожидалось 1", fake.startCalls)
	}
	if !fake.running {
		t.Error("UI должен быть запущен")
	}
}

package cli

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/core"
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

// --- stopAllCoreLifecycles (баг B2: doStop останавливал только активное ядро) ---

// fakeCoreLifecycle — минимальная реализация service.Lifecycle для тестов
// stopAllCoreLifecycles: фиксирует вызовы Stop и позволяет задать ошибку.
// Start/Status/Restart не используются в этих тестах, но нужны для
// удовлетворения интерфейса service.Lifecycle.
type fakeCoreLifecycle struct {
	stopErr   error
	stopCalls int
}

func (f *fakeCoreLifecycle) Start(_ context.Context) error { return nil }

func (f *fakeCoreLifecycle) Stop(_ context.Context) error {
	f.stopCalls++
	return f.stopErr
}

func (f *fakeCoreLifecycle) Status(_ context.Context) (service.Status, error) {
	return service.Status{}, nil
}

func (f *fakeCoreLifecycle) Restart(ctx context.Context) error {
	if err := f.Stop(ctx); err != nil {
		return err
	}
	return f.Start(ctx)
}

// withFakeCoreLifecycles подменяет coreLifecycleFn на карту fake-lifecycle по
// имени ядра — по образцу withFakeUILifecycle выше. Имя ядра, не найденное в
// map, трактуется как ошибка получения lifecycle (аналог core.Get на
// незарегистрированное имя) — stopAllCoreLifecycles обязан залогировать и
// продолжить со следующим ядром.
func withFakeCoreLifecycles(t *testing.T, fakes map[string]*fakeCoreLifecycle) {
	t.Helper()
	prev := coreLifecycleFn
	coreLifecycleFn = func(name string) (service.Lifecycle, error) {
		f, ok := fakes[name]
		if !ok {
			return nil, fmt.Errorf("fake lifecycle для ядра %q не задан", name)
		}
		return f, nil
	}
	t.Cleanup(func() { coreLifecycleFn = prev })
}

// TestStopAllCoreLifecycles_StopsEveryRegisteredCore — регрессия бага B2:
// раньше doStop останавливал только mustActiveCore(), поэтому при смене ядра
// (`--core xray --restart` после сбоя предыдущего) процесс ПРЕДЫДУЩЕГО ядра
// оставался orphan'ом (tasks/lessons.md, 2026-05-12). stopAllCoreLifecycles
// должен останавливать ВСЕ зарегистрированные ядра (core.Names()), а не
// только то, что сейчас активно в state.Core.
func TestStopAllCoreLifecycles_StopsEveryRegisteredCore(t *testing.T) {
	names := core.Names()
	if len(names) < 2 {
		t.Fatalf("тест требует минимум 2 зарегистрированных ядра (core.Names()=%v) — проверьте blank-imports в cores.go", names)
	}

	fakes := make(map[string]*fakeCoreLifecycle, len(names))
	for _, name := range names {
		fakes[name] = &fakeCoreLifecycle{}
	}
	withFakeCoreLifecycles(t, fakes)

	stopAllCoreLifecycles(context.Background())

	for _, name := range names {
		if fakes[name].stopCalls != 1 {
			t.Errorf("ядро %q: Stop вызван %d раз, ожидался 1 (должны останавливаться ВСЕ ядра, не только активное)",
				name, fakes[name].stopCalls)
		}
	}
}

// TestStopAllCoreLifecycles_ErrorOnOneCoreDoesNotStopOthers проверяет, что
// ошибка Stop одного ядра (например, зависший процесс) не прерывает
// остановку остальных — иначе orphan одного ядра помешал бы --stop/--restart
// остановить прочие и снять firewall.
func TestStopAllCoreLifecycles_ErrorOnOneCoreDoesNotStopOthers(t *testing.T) {
	names := core.Names()
	if len(names) < 2 {
		t.Fatalf("тест требует минимум 2 зарегистрированных ядра (core.Names()=%v)", names)
	}

	fakes := make(map[string]*fakeCoreLifecycle, len(names))
	for _, name := range names {
		fakes[name] = &fakeCoreLifecycle{}
	}
	// Первое (в алфавитном порядке) ядро падает при Stop — остальные должны
	// быть остановлены несмотря на это.
	fakes[names[0]].stopErr = errors.New("stop failed: process not responding")
	withFakeCoreLifecycles(t, fakes)

	stopAllCoreLifecycles(context.Background())

	for _, name := range names {
		if fakes[name].stopCalls != 1 {
			t.Errorf("ядро %q: Stop вызван %d раз, ожидался 1 (ошибка одного ядра не должна прерывать цикл по остальным)",
				name, fakes[name].stopCalls)
		}
	}
}

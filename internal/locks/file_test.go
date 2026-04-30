package locks

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAcquireRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	lk, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := lk.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestAcquire_SecondBlocksUntilRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	lk1, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	acquired := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lk2, err := Acquire(context.Background(), path)
		if err != nil {
			t.Errorf("second Acquire: %v", err)
			return
		}
		close(acquired)
		_ = lk2.Release()
	}()

	// give goroutine time to block
	time.Sleep(50 * time.Millisecond)
	select {
	case <-acquired:
		t.Fatal("second lock acquired before first was released")
	default:
	}

	_ = lk1.Release()

	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("second lock not acquired after first release")
	}

	wg.Wait()
}

func TestTryAcquire_ReturnsLockWhenFree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	lk, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if err := lk.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestTryAcquire_ReturnsErrLockedWhenHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	held, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatalf("первый Acquire: %v", err)
	}
	defer held.Release()

	if _, err := TryAcquire(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("ожидался ErrLocked, получено: %v", err)
	}
}

func TestAcquire_ContextCancelledWhileWaiting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	lk1, err := Acquire(context.Background(), path)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer lk1.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err = Acquire(ctx, path)
	if err == nil {
		t.Fatal("expected error when context cancelled")
	}
}

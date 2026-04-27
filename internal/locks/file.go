package locks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const pollInterval = 100 * time.Millisecond

// Lock представляет удерживаемую эксклюзивную блокировку flock.
type Lock struct {
	path string
	f    *os.File
}

// Acquire получает эксклюзивную неблокирующую блокировку flock на указанном файле.
// Повторяет попытку каждые 100 мс до отмены ctx.
func Acquire(ctx context.Context, path string) (*Lock, error) {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return nil, fmt.Errorf("locks: mkdir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("locks: open %s: %w", path, err)
	}

	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return &Lock{path: path, f: f}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			f.Close()
			return nil, fmt.Errorf("locks: flock %s: %w", path, err)
		}

		select {
		case <-ctx.Done():
			f.Close()
			return nil, fmt.Errorf("locks: таймаут ожидания %s: %w", path, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
}

// Release снимает блокировку flock и закрывает файл.
func (l *Lock) Release() error {
	if err := unix.Flock(int(l.f.Fd()), unix.LOCK_UN); err != nil {
		_ = l.f.Close()
		return fmt.Errorf("locks: разблокировка %s: %w", l.path, err)
	}
	return l.f.Close()
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

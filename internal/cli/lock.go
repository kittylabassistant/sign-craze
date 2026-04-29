package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kittylabassistant/sign-craze/internal/locks"
)

// withLock захватывает /opt/var/lock/sign-craze.lock на время вызова fn.
// Возвращает ошибку, если не удалось получить блокировку (например, другой
// экземпляр sign-craze уже работает) или если сама fn вернула ошибку.
func withLock(ctx context.Context, fn func() error) error {
	l, err := locks.Acquire(ctx, locks.DefaultPath)
	if err != nil {
		return fmt.Errorf("cli: захват блокировки: %w", err)
	}
	defer func() {
		if relErr := l.Release(); relErr != nil {
			// Лог; основная ошибка fn не теряется.
			slog.Warn("cli: ошибка снятия блокировки", "path", locks.DefaultPath, "err", relErr)
		}
	}()
	return fn()
}

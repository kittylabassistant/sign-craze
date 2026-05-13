package cli

import "sync"

// ResetColorStateForTest сбрасывает sync.Once и color-флаги. Только для тестов.
func ResetColorStateForTest() {
	colorOnce = sync.Once{}
	colorEnabledStdout = false
	colorEnabledStderr = false
}

// SetIsTerminalFnForTest подменяет TTY-детектор в тестах.
// Возвращает restore-функцию для отката (defer-friendly).
func SetIsTerminalFnForTest(fn func(fd uintptr) bool) (restore func()) {
	prev := isTerminalFn
	isTerminalFn = fn
	return func() { isTerminalFn = prev }
}

package cli

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/term"
)

// ANSI escape sequences. Без фоновых цветов и 256-color — на Keenetic
// через SSH часто доступен лишь базовый xterm/vt100.
const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiBlue   = "\x1b[34m"
	ansiCyan   = "\x1b[36m"
	ansiRedB   = "\x1b[31;1m"
)

var (
	colorOnce          sync.Once
	colorEnabledStdout bool
	colorEnabledStderr bool

	// isTerminalFn — подменяется в тестах для стабильного результата
	// независимо от TTY-состояния процесса go test.
	isTerminalFn = func(fd uintptr) bool { return term.IsTerminal(int(fd)) }
)

// PreParseNoColor извлекает глобальный флаг --no-color из argv до
// маршрутизации команды и возвращает остаток. Флаг убирается, чтобы
// не мешать обработчикам подкоманд.
func PreParseNoColor(args []string) (rest []string, noColor bool) {
	rest = make([]string, 0, len(args))
	for _, a := range args {
		if a == "--no-color" {
			noColor = true
			continue
		}
		rest = append(rest, a)
	}
	return
}

// InitColor определяет, включать ли ANSI-цвет для stdout и stderr.
// Вызывается ровно один раз (sync.Once); повторные вызовы игнорируются.
//
// Приоритет (от высшего к низшему):
//  1. флаг --no-color → выкл.
//  2. NO_COLOR env (любое значение, включая пустое) → выкл.
//  3. FORCE_COLOR env (непустое) → вкл. для обоих потоков.
//  4. TERM=dumb → выкл.
//  5. term.IsTerminal — авто-детект отдельно для stdout и stderr.
func InitColor(noColorFlag bool) {
	colorOnce.Do(func() {
		stdoutOk, stderrOk := computeColorState(noColorFlag)
		colorEnabledStdout = stdoutOk
		colorEnabledStderr = stderrOk
	})
}

func computeColorState(noColorFlag bool) (stdoutOk, stderrOk bool) {
	if noColorFlag {
		return false, false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false, false
	}
	if v := os.Getenv("FORCE_COLOR"); v != "" {
		return true, true
	}
	if os.Getenv("TERM") == "dumb" {
		return false, false
	}
	return isTerminalFn(os.Stdout.Fd()), isTerminalFn(os.Stderr.Fd())
}

// ColorEnabled сообщает, активен ли ANSI-цвет для stdout.
func ColorEnabled() bool { return colorEnabledStdout }

// StderrColorEnabled сообщает, активен ли ANSI-цвет для stderr
// (используется логгером для выбора между обычным и цветным slog handler).
func StderrColorEnabled() bool { return colorEnabledStderr }

func wrap(code, s string) string {
	if !colorEnabledStdout {
		return s
	}
	return code + s + ansiReset
}

// Низкоуровневые ANSI-хелперы. No-op при выключенном цвете.
func Red(s string) string    { return wrap(ansiRed, s) }
func Green(s string) string  { return wrap(ansiGreen, s) }
func Yellow(s string) string { return wrap(ansiYellow, s) }
func Blue(s string) string   { return wrap(ansiBlue, s) }
func Cyan(s string) string   { return wrap(ansiCyan, s) }
func Dim(s string) string    { return wrap(ansiDim, s) }
func Bold(s string) string   { return wrap(ansiBold, s) }

// Семантические хелперы — код звучит по смыслу, не по цвету.
func OK(s string) string     { return Green(s) }
func Warn(s string) string   { return Yellow(s) }
func Err(s string) string    { return Red(s) }
func Info(s string) string   { return Cyan(s) }
func Hint(s string) string   { return Dim(s) }
func Header(s string) string { return Bold(s) }
func Key(s string) string    { return Cyan(s) }

// ServiceStatus формирует строку статуса сервиса с цветовой пометкой.
// Используется в --status и итоговых сообщениях --start/--stop.
func ServiceStatus(running bool, pid int) string {
	if running {
		return OK("запущен") + fmt.Sprintf("  (pid %d)", pid)
	}
	return Err("остановлен")
}

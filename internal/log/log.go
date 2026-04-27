package log

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

const (
	defaultLogPath = "/opt/var/log/sign-craze/sign-craze.log"
	maxSizeBytes   = 10 * 1024 * 1024 // 10 МБ
)

var (
	global *slog.Logger
	once   sync.Once
)

// Init инициализирует глобальный логгер. levelStr соответствует именам уровней slog
// (DEBUG, INFO, WARN, ERROR); пустая строка означает INFO.
func Init(levelStr string) {
	once.Do(func() { global = newLogger(levelStr) })
}

// L возвращает глобальный логгер. При отсутствии Init возвращает slog.Default().
func L() *slog.Logger {
	if global == nil {
		return slog.Default()
	}
	return global
}

func newLogger(levelStr string) *slog.Logger {
	level := parseLevel(levelStr)

	var w io.Writer
	if term.IsTerminal(int(os.Stderr.Fd())) {
		w = os.Stderr
		return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
	}

	w = newRotatingWriter(defaultLogPath, maxSizeBytes)
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

func parseLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// rotatingWriter переоткрывает и архивирует лог-файл при превышении maxSize.
type rotatingWriter struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	f       *os.File
}

func newRotatingWriter(path string, maxSize int64) io.Writer {
	rw := &rotatingWriter{path: path, maxSize: maxSize}
	if err := rw.open(); err != nil {
		// директория логов ещё не создана — пишем в stderr до первого старта
		return os.Stderr
	}
	return rw
}

func (rw *rotatingWriter) Write(p []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.f == nil {
		if err := rw.open(); err != nil {
			return os.Stderr.Write(p)
		}
	}

	info, err := rw.f.Stat()
	if err == nil && info.Size()+int64(len(p)) > rw.maxSize {
		rw.rotate()
	}

	return rw.f.Write(p)
}

func (rw *rotatingWriter) open() error {
	if err := os.MkdirAll(dirOf(rw.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(rw.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	rw.f = f
	return nil
}

func (rw *rotatingWriter) rotate() {
	archived := rw.path + "." + time.Now().Format("20060102T150405")
	_ = rw.f.Close()
	_ = os.Rename(rw.path, archived)
	_ = rw.open()
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

package log

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
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

// Init инициализирует глобальный логгер. levelStr соответствует именам уровней
// slog (DEBUG, INFO, WARN, ERROR); пустая строка означает INFO. stderrColor
// включает цветной handler при выводе в TTY-stderr — реальное состояние
// рассчитывается caller'ом (cli.StderrColorEnabled).
func Init(levelStr string, stderrColor bool) {
	once.Do(func() { global = newLogger(levelStr, stderrColor) })
}

// L возвращает глобальный логгер. При отсутствии Init возвращает slog.Default().
func L() *slog.Logger {
	if global == nil {
		return slog.Default()
	}
	return global
}

func newLogger(levelStr string, stderrColor bool) *slog.Logger {
	level := parseLevel(levelStr)

	if term.IsTerminal(int(os.Stderr.Fd())) {
		if stderrColor {
			return slog.New(newColorTextHandler(os.Stderr, level))
		}
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	}

	w := newRotatingWriter(defaultLogPath, maxSizeBytes)
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

// colorTextHandler — простой человеко-читаемый slog.Handler с ANSI-цветом
// уровня. Формат: "HH:MM:SS.mmm LEVEL message key=value ...".
//
// Реализован с нуля, а не оборачивает slog.NewTextHandler: TextHandler сам
// форматирует поле `level=`, в которое после факта вставить ANSI можно лишь
// post-processing'ом (хрупко). Свой Handler даёт прямой контроль над
// порядком и цветом полей.
type colorTextHandler struct {
	w      io.Writer
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

func newColorTextHandler(w io.Writer, level slog.Leveler) *colorTextHandler {
	return &colorTextHandler{w: w, level: level, mu: &sync.Mutex{}}
}

func (h *colorTextHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *colorTextHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	if !r.Time.IsZero() {
		b.WriteString(r.Time.Format("15:04:05.000"))
		b.WriteByte(' ')
	}
	b.WriteString(colorLevel(r.Level))
	b.WriteByte(' ')
	b.WriteString(r.Message)

	for _, a := range h.attrs {
		appendAttr(&b, a, h.groups)
	}
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(&b, a, h.groups)
		return true
	})
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *colorTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &colorTextHandler{w: h.w, level: h.level, attrs: merged, groups: h.groups, mu: h.mu}
}

func (h *colorTextHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	groups := make([]string, 0, len(h.groups)+1)
	groups = append(groups, h.groups...)
	groups = append(groups, name)
	return &colorTextHandler{w: h.w, level: h.level, attrs: h.attrs, groups: groups, mu: h.mu}
}

func colorLevel(l slog.Level) string {
	s := l.String()
	switch l {
	case slog.LevelDebug:
		return "\x1b[2m" + s + "\x1b[0m"
	case slog.LevelInfo:
		return "\x1b[36m" + s + "\x1b[0m"
	case slog.LevelWarn:
		return "\x1b[33m" + s + "\x1b[0m"
	case slog.LevelError:
		return "\x1b[31;1m" + s + "\x1b[0m"
	}
	return s
}

func appendAttr(b *strings.Builder, a slog.Attr, groups []string) {
	if a.Equal(slog.Attr{}) || a.Key == "" {
		return
	}
	b.WriteByte(' ')
	if len(groups) > 0 {
		b.WriteString(strings.Join(groups, "."))
		b.WriteByte('.')
	}
	b.WriteString(a.Key)
	b.WriteByte('=')
	b.WriteString(formatValue(a.Value))
}

func formatValue(v slog.Value) string {
	v = v.Resolve()
	switch v.Kind() {
	case slog.KindString:
		s := v.String()
		if s == "" || strings.ContainsAny(s, " \t\"=\n") {
			return strconv.Quote(s)
		}
		return s
	case slog.KindTime:
		return v.Time().Format(time.RFC3339Nano)
	case slog.KindAny:
		// fmt.Sprintf("%v") через slog.Value.String() — он сам делает анной формат.
		return v.String()
	default:
		return v.String()
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
	if err := os.MkdirAll(filepath.Dir(rw.path), 0o755); err != nil {
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
	if err := rw.f.Close(); err != nil {
		return
	}
	if err := os.Rename(rw.path, archived); err != nil {
		return
	}
	if err := rw.open(); err != nil {
		rw.f = nil
	}
}

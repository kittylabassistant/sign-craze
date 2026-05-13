package log

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"DEBUG", "DEBUG"},
		{"debug", "DEBUG"},
		{"INFO", "INFO"},
		{"", "INFO"},
		{"WARN", "WARN"},
		{"WARNING", "WARN"},
		{"ERROR", "ERROR"},
		{"invalid", "INFO"},
	}
	for _, tt := range tests {
		got := parseLevel(tt.input).String()
		if got != tt.want {
			t.Errorf("parseLevel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRotatingWriter_Rotates(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	const small = 64 // rotate after 64 bytes
	rw := &rotatingWriter{path: logPath, maxSize: small}
	if err := rw.open(); err != nil {
		t.Fatalf("open: %v", err)
	}

	payload := strings.Repeat("x", small+1)
	if _, err := rw.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	// second write triggers rotation
	if _, err := rw.Write([]byte("after-rotate")); err != nil {
		t.Fatalf("write after rotate: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected archived + current log, got %d files", len(entries))
	}
}

func TestDirOf(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/opt/var/log/sign-craze/sign-craze.log", "/opt/var/log/sign-craze"},
		{"sign-craze.log", "."},
		{"/tmp/foo.log", "/tmp"},
	}
	for _, tt := range tests {
		if got := filepath.Dir(tt.path); got != tt.want {
			t.Errorf("filepath.Dir(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestColorTextHandler_LevelsStyled(t *testing.T) {
	tests := []struct {
		level  slog.Level
		method func(*slog.Logger)
		want   string // ANSI prefix expected
	}{
		{slog.LevelDebug, func(l *slog.Logger) { l.Debug("m") }, "\x1b[2mDEBUG\x1b[0m"},
		{slog.LevelInfo, func(l *slog.Logger) { l.Info("m") }, "\x1b[36mINFO\x1b[0m"},
		{slog.LevelWarn, func(l *slog.Logger) { l.Warn("m") }, "\x1b[33mWARN\x1b[0m"},
		{slog.LevelError, func(l *slog.Logger) { l.Error("m") }, "\x1b[31;1mERROR\x1b[0m"},
	}
	for _, tc := range tests {
		t.Run(tc.level.String(), func(t *testing.T) {
			var buf bytes.Buffer
			h := newColorTextHandler(&buf, slog.LevelDebug)
			tc.method(slog.New(h))
			out := buf.String()
			if !strings.Contains(out, tc.want) {
				t.Errorf("missing ANSI for %s in %q", tc.level, out)
			}
			if !strings.Contains(out, "m\n") && !strings.HasSuffix(out, "\n") {
				t.Errorf("expected trailing newline in %q", out)
			}
		})
	}
}

func TestColorTextHandler_AttrsAndGroups(t *testing.T) {
	var buf bytes.Buffer
	h := newColorTextHandler(&buf, slog.LevelDebug)
	l := slog.New(h).With("user", "alice").WithGroup("net")
	l.Info("connect", "ip", "10.0.0.1", "port", 443)

	out := buf.String()
	for _, want := range []string{"connect", "user=alice", "net.ip=10.0.0.1", "net.port=443"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestColorTextHandler_Enabled(t *testing.T) {
	var buf bytes.Buffer
	h := newColorTextHandler(&buf, slog.LevelWarn)
	l := slog.New(h)
	l.Info("hidden")
	l.Warn("visible")

	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Errorf("INFO leaked at WARN level: %q", out)
	}
	if !strings.Contains(out, "visible") {
		t.Errorf("WARN dropped: %q", out)
	}
}

func TestColorTextHandler_QuotesValuesWithSpaces(t *testing.T) {
	var buf bytes.Buffer
	h := newColorTextHandler(&buf, slog.LevelDebug)
	slog.New(h).Info("m", "key", "value with spaces")
	out := buf.String()
	if !strings.Contains(out, `key="value with spaces"`) {
		t.Errorf("expected quoted value in %q", out)
	}
}

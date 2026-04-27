package log

import (
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
		if got := dirOf(tt.path); got != tt.want {
			t.Errorf("dirOf(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

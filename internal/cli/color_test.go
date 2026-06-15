package cli

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func alwaysTTY(uintptr) bool { return true }
func neverTTY(uintptr) bool  { return false }

// cleanEnv удаляет цветовые env-переменные и регистрирует cleanup, который
// восстановит исходное значение. t.Setenv("X","") не подходит, потому что
// он лишь выставляет пустую строку — os.LookupEnv по-прежнему вернёт ok=true.
func cleanEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		prev, had := os.LookupEnv(k)
		_ = os.Unsetenv(k)
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(k, prev)
			} else {
				_ = os.Unsetenv(k)
			}
		})
	}
}

func TestInitColor_PriorityMatrix(t *testing.T) {
	type envState struct {
		noColor    *string
		forceColor *string
		term       *string
	}
	str := func(s string) *string { return &s }

	tests := []struct {
		name        string
		noColorFlag bool
		env         envState
		tty         func(uintptr) bool
		wantStdout  bool
		wantStderr  bool
	}{
		{
			name:        "flag wins over everything",
			noColorFlag: true,
			env:         envState{forceColor: str("1")},
			tty:         alwaysTTY,
		},
		{
			name:        "NO_COLOR present (empty value) wins over FORCE_COLOR",
			noColorFlag: false,
			env:         envState{noColor: str(""), forceColor: str("1")},
			tty:         alwaysTTY,
		},
		{
			name:        "FORCE_COLOR forces ON without TTY",
			noColorFlag: false,
			env:         envState{forceColor: str("1")},
			tty:         neverTTY,
			wantStdout:  true,
			wantStderr:  true,
		},
		{
			name:        "TERM=dumb disables in TTY",
			noColorFlag: false,
			env:         envState{term: str("dumb")},
			tty:         alwaysTTY,
		},
		{
			name:        "auto on with TTY",
			noColorFlag: false,
			env:         envState{},
			tty:         alwaysTTY,
			wantStdout:  true,
			wantStderr:  true,
		},
		{
			name:        "auto off without TTY",
			noColorFlag: false,
			env:         envState{},
			tty:         neverTTY,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanEnv(t, "NO_COLOR", "FORCE_COLOR", "TERM")
			if tc.env.noColor != nil {
				t.Setenv("NO_COLOR", *tc.env.noColor)
			}
			if tc.env.forceColor != nil {
				t.Setenv("FORCE_COLOR", *tc.env.forceColor)
			}
			if tc.env.term != nil {
				t.Setenv("TERM", *tc.env.term)
			}

			restore := SetIsTerminalFnForTest(tc.tty)
			defer restore()
			ResetColorStateForTest()

			InitColor(tc.noColorFlag)

			if got := ColorEnabled(); got != tc.wantStdout {
				t.Errorf("stdout: got %v, want %v", got, tc.wantStdout)
			}
			if got := StderrColorEnabled(); got != tc.wantStderr {
				t.Errorf("stderr: got %v, want %v", got, tc.wantStderr)
			}
		})
	}
}

func TestColorFuncs_DisabledAreNoop(t *testing.T) {
	cleanEnv(t, "NO_COLOR", "FORCE_COLOR", "TERM")
	t.Setenv("NO_COLOR", "1")
	ResetColorStateForTest()
	InitColor(false)

	fns := []struct {
		name string
		fn   func(string) string
	}{
		{"Red", Red}, {"Green", Green}, {"Yellow", Yellow},
		{"Cyan", Cyan}, {"Dim", Dim}, {"Bold", Bold},
		{"OK", OK}, {"Warn", Warn}, {"Err", Err}, {"Info", Info},
		{"Hint", Hint}, {"Header", Header}, {"Key", Key},
	}
	for _, f := range fns {
		s := f.fn("payload")
		if s != "payload" {
			t.Errorf("%s: expected no-op, got %q", f.name, s)
		}
		if strings.Contains(s, "\x1b[") {
			t.Errorf("%s: unexpected ANSI in %q", f.name, s)
		}
	}
}

func TestColorFuncs_EnabledWrapWithANSI(t *testing.T) {
	cleanEnv(t, "NO_COLOR", "FORCE_COLOR", "TERM")
	t.Setenv("FORCE_COLOR", "1")
	ResetColorStateForTest()
	InitColor(false)

	cases := map[string]string{
		"Red": Red("x"), "Green": Green("x"), "Yellow": Yellow("x"),
		"Cyan": Cyan("x"), "Dim": Dim("x"), "Bold": Bold("x"),
		"OK": OK("x"), "Warn": Warn("x"), "Err": Err("x"), "Info": Info("x"),
		"Hint": Hint("x"), "Header": Header("x"), "Key": Key("x"),
	}
	for name, got := range cases {
		if !strings.Contains(got, "\x1b[") {
			t.Errorf("%s: ANSI missing in %q", name, got)
		}
		if !strings.HasSuffix(got, ansiReset) {
			t.Errorf("%s: reset missing in %q", name, got)
		}
		if stripANSI(got) != "x" {
			t.Errorf("%s: stripANSI(%q) != %q", name, got, "x")
		}
	}
}

func TestServiceStatus(t *testing.T) {
	cleanEnv(t, "NO_COLOR", "FORCE_COLOR", "TERM")
	t.Setenv("NO_COLOR", "1")
	ResetColorStateForTest()
	InitColor(false)

	if got := ServiceStatus(true, 42); got != "запущен  (pid 42)" {
		t.Errorf("running: %q", got)
	}
	if got := ServiceStatus(false, 0); got != "остановлен" {
		t.Errorf("stopped: %q", got)
	}
}

func TestPreParseNoColor(t *testing.T) {
	tests := []struct {
		in       []string
		wantArgs []string
		wantFlag bool
	}{
		{[]string{"--status"}, []string{"--status"}, false},
		{[]string{"--no-color", "--status"}, []string{"--status"}, true},
		{[]string{"--status", "--no-color"}, []string{"--status"}, true},
		{[]string{"-s", "--no-color", "extra"}, []string{"-s", "extra"}, true},
	}
	for _, tc := range tests {
		gotArgs, gotFlag := PreParseNoColor(tc.in)
		if gotFlag != tc.wantFlag {
			t.Errorf("flag: in=%v got=%v want=%v", tc.in, gotFlag, tc.wantFlag)
		}
		if !equalStrings(gotArgs, tc.wantArgs) {
			t.Errorf("args: in=%v got=%v want=%v", tc.in, gotArgs, tc.wantArgs)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

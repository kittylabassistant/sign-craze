package version

import (
	"strings"
	"testing"
)

func TestVersion_NotEmpty(t *testing.T) {
	if Version == "" {
		t.Fatal("Version пустой")
	}
}

func TestVersion_NoWhitespace(t *testing.T) {
	if strings.ContainsAny(Version, " \t\n\r") {
		t.Errorf("Version содержит пробельные символы: %q", Version)
	}
}

func TestString_ContainsVersion(t *testing.T) {
	s := String()
	core := strings.TrimPrefix(Version, "v")
	if !strings.Contains(s, core) {
		t.Errorf("String() = %q не содержит версию %q", s, core)
	}
	if !strings.HasPrefix(s, "v") {
		t.Errorf("String() = %q должен начинаться с 'v'", s)
	}
	if strings.HasPrefix(s, "vv") {
		t.Errorf("String() = %q содержит двойной префикс 'vv'", s)
	}
}

func TestShort_NoDoubleV(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"0.2.2", "v0.2.2"},
		{"v0.2.2", "v0.2.2"},
		{"vv0.2.2", "v0.2.2"},
	}
	orig := Version
	defer func() { Version = orig }()
	for _, c := range cases {
		Version = c.in
		got := Short()
		if got != c.want {
			t.Errorf("Short() при Version=%q = %q, ожидалось %q", c.in, got, c.want)
		}
	}
}

func TestGet_GoVersionNotEmpty(t *testing.T) {
	bi := Get()
	if bi.GoVersion == "" {
		t.Error("BuildInfo.GoVersion пустой")
	}
}

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
	if !strings.Contains(s, Version) {
		t.Errorf("String() = %q не содержит версию %q", s, Version)
	}
	if !strings.HasPrefix(s, "v") {
		t.Errorf("String() = %q должен начинаться с 'v'", s)
	}
}

func TestGet_GoVersionNotEmpty(t *testing.T) {
	bi := Get()
	if bi.GoVersion == "" {
		t.Error("BuildInfo.GoVersion пустой")
	}
}

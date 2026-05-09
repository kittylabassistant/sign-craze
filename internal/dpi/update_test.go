package dpi

import (
	"strings"
	"testing"
	"time"
)

func TestParseHostLine(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"example.com", "example.com"},
		{"  example.com  ", "example.com"},
		{"# comment", ""},
		{"example.com # inline", "example.com"},
		{"", ""},
		{"  ", ""},
		{"0.0.0.0 example.com", "example.com"},
		{"127.0.0.1 example.com  # block", "example.com"},
		{"||example.com^", "example.com"},
		{"||example.com", "example.com"},
		{"||example.com$third-party", "example.com"},
		{"!comment-adblock", ""},
		{"EXAMPLE.COM", "example.com"},
		{"plainword", ""},   // no dot → не hostname
		{"<html>", ""},      // мусор
		{"foo bar baz", ""}, // первое поле "foo" не хост (нет точки)
		{"foo.com bar baz", "foo.com"},
	}
	for _, c := range cases {
		got := parseHostLine(c.in)
		if got != c.want {
			t.Errorf("parseHostLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLooksLikeHostname(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"a.b", true},
		{"plain", false},
		{"two words.com", false},
		{"имя.рф", false}, // unicode не ASCII — на slow MIPS избегаем IDN
		{"<html>", false},
	}
	for _, c := range cases {
		got := looksLikeHostname(c.in)
		if got != c.want {
			t.Errorf("looksLikeHostname(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestShouldUpdate_Disabled(t *testing.T) {
	if ShouldUpdate("", 0) {
		t.Error("ShouldUpdate(_, 0) должно быть false (auto-update выключен)")
	}
	if ShouldUpdate("2026-05-09T12:00:00Z", -1) {
		t.Error("ShouldUpdate(_, -1) должно быть false")
	}
}

func TestShouldUpdate_NeverUpdated(t *testing.T) {
	if !ShouldUpdate("", 24) {
		t.Error("ShouldUpdate(пустая, 24) должно быть true (никогда не обновлялся)")
	}
	if !ShouldUpdate("garbage", 24) {
		t.Error("ShouldUpdate(некорректный ts, 24) должно быть true")
	}
}

func TestShouldUpdate_Threshold(t *testing.T) {
	now := time.Now().UTC()
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)
	old := now.Add(-25 * time.Hour).Format(time.RFC3339)
	if ShouldUpdate(recent, 24) {
		t.Error("ShouldUpdate(recent=1h, interval=24h) должно быть false")
	}
	if !ShouldUpdate(old, 24) {
		t.Error("ShouldUpdate(old=25h, interval=24h) должно быть true")
	}
}

func TestResilientResolver_NotNil(t *testing.T) {
	r := resilientResolver()
	if r == nil {
		t.Fatal("resilientResolver() = nil")
	}
	if !r.PreferGo {
		t.Error("resilientResolver: PreferGo должен быть true для использования custom Dial")
	}
	if r.Dial == nil {
		t.Error("resilientResolver: Dial callback должен быть установлен")
	}
}

func TestFallbackDNSServers_NotEmpty(t *testing.T) {
	if len(fallbackDNSServers) == 0 {
		t.Fatal("fallbackDNSServers не должен быть пустым (без fallback Auto-update сломается на Keenetic+DNSCrypt)")
	}
	for i, s := range fallbackDNSServers {
		if s == "" {
			t.Errorf("fallbackDNSServers[%d] пустая строка", i)
		}
		if !strings.HasSuffix(s, ":53") {
			t.Errorf("fallbackDNSServers[%d]=%q должен оканчиваться на :53", i, s)
		}
	}
}

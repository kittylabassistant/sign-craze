package modes

import (
	"strings"
	"testing"
)

func TestExcludeRules_Structure(t *testing.T) {
	rules := ExcludeRules()
	if len(rules) != 1 {
		t.Fatalf("ожидалось 1 правило, получено %d", len(rules))
	}
	r := rules[0]
	if r.Table != "mangle" {
		t.Errorf("Table = %q, ожидалось mangle", r.Table)
	}
	if r.Chain != "signcraze" {
		t.Errorf("Chain = %q, ожидалось signcraze", r.Chain)
	}
	args := strings.Join(r.Args, " ")
	if !strings.Contains(args, "--match-set signcraze_excludes dst") {
		t.Errorf("отсутствует match-set: %s", args)
	}
	if !strings.Contains(args, "-j RETURN") {
		t.Errorf("должно быть RETURN: %s", args)
	}
}

// TestAdminPortBypassRule защищает SSH/web admin от попадания в TPROXY:
// пакеты на admin порт получают RETURN раньше mark-правил (safety-fixes #2).
func TestAdminPortBypassRule_Structure(t *testing.T) {
	rules := AdminPortBypassRule(22)
	if len(rules) != 2 {
		t.Fatalf("ожидалось 2 правила (TCP+UDP), получено %d", len(rules))
	}
	for _, r := range rules {
		if r.Table != "mangle" || r.Chain != "signcraze" {
			t.Errorf("неверная позиция: %s/%s", r.Table, r.Chain)
		}
		args := strings.Join(r.Args, " ")
		if !strings.Contains(args, "--dport 22") {
			t.Errorf("отсутствует --dport 22: %s", args)
		}
		if !strings.Contains(args, "-j RETURN") {
			t.Errorf("должно быть RETURN: %s", args)
		}
	}
}

func TestAdminPortBypassRule_ZeroPort(t *testing.T) {
	// port=0 — пропустить (admin bypass отключён).
	if rules := AdminPortBypassRule(0); len(rules) != 0 {
		t.Errorf("port=0 должен дать пустой slice, получено %d правил", len(rules))
	}
}

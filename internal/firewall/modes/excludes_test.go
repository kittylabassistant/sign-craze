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

// TestAdminPortsBypassRules защищает SSH/web admin от попадания в TPROXY:
// пакеты на admin порты получают RETURN раньше mark-правил (safety-fixes #2).
func TestAdminPortsBypassRules_Structure(t *testing.T) {
	rules := AdminPortsBypassRules([]uint16{22, 222})
	if len(rules) != 4 {
		t.Fatalf("ожидалось 4 правила (TCP+UDP × 2 порта), получено %d", len(rules))
	}
	for _, r := range rules {
		if r.Table != "mangle" || r.Chain != "signcraze" {
			t.Errorf("неверная позиция: %s/%s", r.Table, r.Chain)
		}
		args := strings.Join(r.Args, " ")
		if !strings.Contains(args, "-j RETURN") {
			t.Errorf("должно быть RETURN: %s", args)
		}
	}
	all := strings.Join(flattenArgs(rules), " ")
	if !strings.Contains(all, "--dport 22 ") || !strings.Contains(all, "--dport 222") {
		t.Errorf("отсутствует один из портов 22/222: %s", all)
	}
}

func TestAdminPortsBypassRules_Empty(t *testing.T) {
	if rules := AdminPortsBypassRules(nil); len(rules) != 0 {
		t.Errorf("nil должен дать пустой slice, получено %d", len(rules))
	}
	if rules := AdminPortsBypassRules([]uint16{}); len(rules) != 0 {
		t.Errorf("[] должен дать пустой slice, получено %d", len(rules))
	}
	if rules := AdminPortsBypassRules([]uint16{0, 0}); len(rules) != 0 {
		t.Errorf("[0,0] должен дать пустой slice, получено %d", len(rules))
	}
}

func flattenArgs(rules []RuleSpec) []string {
	var out []string
	for _, r := range rules {
		out = append(out, r.Args...)
	}
	return out
}

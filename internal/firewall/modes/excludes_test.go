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

func TestLocalBypassRules_Structure(t *testing.T) {
	rules := LocalBypassRules("mangle", "signcraze_policy", []string{"172.16.0.1", "10.1.30.1"})
	if len(rules) != 2 {
		t.Fatalf("ожидалось 2 правила, получено %d", len(rules))
	}
	for i, r := range rules {
		if r.Table != "mangle" || r.Chain != "signcraze_policy" {
			t.Errorf("[%d] table/chain = %s/%s, ожидалось mangle/signcraze_policy", i, r.Table, r.Chain)
		}
		joined := strings.Join(r.Args, " ")
		if !strings.Contains(joined, "-j RETURN") {
			t.Errorf("[%d] должно быть RETURN: %s", i, joined)
		}
	}
}

func TestLocalBypassRules_EmptyOrNilIPs(t *testing.T) {
	if rules := LocalBypassRules("mangle", "signcraze_policy", nil); len(rules) != 0 {
		t.Errorf("nil → ожидался пустой slice, получено %d", len(rules))
	}
	if rules := LocalBypassRules("mangle", "signcraze_policy", []string{"", ""}); len(rules) != 0 {
		t.Errorf("пустые строки должны игнорироваться, получено %d", len(rules))
	}
}

func TestLocalBypassRules_NATTable(t *testing.T) {
	rules := LocalBypassRules("nat", "signcraze_policy", []string{"172.16.0.1"})
	if len(rules) != 1 || rules[0].Table != "nat" {
		t.Errorf("ожидался 1 правило в таблице nat, получено %+v", rules)
	}
}

func TestAdminPortsBypassRulesForChain_PolicyChainMangle(t *testing.T) {
	rules := AdminPortsBypassRulesForChain([]uint16{22, 222}, "mangle", "signcraze_policy")
	if len(rules) != 4 {
		t.Fatalf("ожидалось 4 правила (2 порта * tcp+udp), получено %d", len(rules))
	}
	for _, r := range rules {
		if r.Table != "mangle" || r.Chain != "signcraze_policy" {
			t.Errorf("table/chain = %s/%s", r.Table, r.Chain)
		}
	}
}

func TestAdminPortsBypassRulesForChain_NAT(t *testing.T) {
	rules := AdminPortsBypassRulesForChain([]uint16{22}, "nat", "signcraze_policy")
	if len(rules) != 2 || rules[0].Table != "nat" {
		t.Errorf("ожидалось 2 правила nat, получено %+v", rules)
	}
}

func TestAdminPortsBypassRules_BackwardCompat(t *testing.T) {
	old := AdminPortsBypassRules([]uint16{22, 222})
	newRules := AdminPortsBypassRulesForChain([]uint16{22, 222}, "mangle", "signcraze")
	if len(old) != len(newRules) {
		t.Errorf("back-compat нарушена: %d != %d", len(old), len(newRules))
	}
	for i := range old {
		if old[i].Table != newRules[i].Table || old[i].Chain != newRules[i].Chain {
			t.Errorf("[%d] %+v != %+v", i, old[i], newRules[i])
		}
	}
}

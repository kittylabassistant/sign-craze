package modes

import (
	"strings"
	"testing"
)

func TestPolicyDPIRules_БезWANInterface_JumpБезФильтра(t *testing.T) {
	rules := PolicyDPIRules(0xaab, 300, "", nil)
	if len(rules) == 0 {
		t.Fatal("PolicyDPIRules: пустой результат")
	}
	jump := lastPostroutingJump(rules)
	if jump == nil {
		t.Fatal("не найдено POSTROUTING jump-правило")
	}
	if argsContains(jump.Args, "-o") {
		t.Errorf("POSTROUTING jump с пустым wanIface не должно содержать `-o`: %v", jump.Args)
	}
}

func TestPolicyDPIRules_СWANInterface_JumpСодержитO(t *testing.T) {
	rules := PolicyDPIRules(0xaab, 300, "eth3", nil)
	jump := lastPostroutingJump(rules)
	if jump == nil {
		t.Fatal("не найдено POSTROUTING jump-правило")
	}
	if !argsHasPair(jump.Args, "-o", "eth3") {
		t.Errorf("POSTROUTING jump должен содержать `-o eth3`: %v", jump.Args)
	}
	if !argsHasPair(jump.Args, "-j", PolicyDPIChainName) {
		t.Errorf("POSTROUTING jump должен содержать `-j %s`: %v", PolicyDPIChainName, jump.Args)
	}
}

func TestPolicyDPIRules_VPNExcludeIPs_RETURNПередNFQUEUE(t *testing.T) {
	excludes := []string{"167.17.177.54", "1.2.3.4"}
	rules := PolicyDPIRules(0xaab, 300, "eth3", excludes)

	// Первые len(excludes) правил в signcraze_policy_dpi должны быть RETURN с -d
	for i, ip := range excludes {
		if i >= len(rules) {
			t.Fatalf("ожидалось %d RETURN-правил, получено %d правил всего", len(excludes), len(rules))
		}
		r := rules[i]
		if r.Chain != PolicyDPIChainName {
			t.Errorf("RETURN-правило #%d должно быть в %s, получено %s", i, PolicyDPIChainName, r.Chain)
		}
		if !argsHasPair(r.Args, "-d", ip) {
			t.Errorf("RETURN-правило #%d должно содержать `-d %s`: %v", i, ip, r.Args)
		}
		if !argsContains(r.Args, "RETURN") {
			t.Errorf("RETURN-правило #%d должно содержать target RETURN: %v", i, r.Args)
		}
	}
	// Следующие правила — NFQUEUE-портовые
	if len(rules) < len(excludes)+1 {
		t.Fatal("не найдены NFQUEUE-портовые правила после RETURN-исключений")
	}
	nextRule := rules[len(excludes)]
	if !argsContains(nextRule.Args, "NFQUEUE") {
		t.Errorf("первое правило после RETURN должно быть NFQUEUE: %v", nextRule.Args)
	}
}

func TestPolicyDPIRules_VPNExcludeIPs_ПустыеСтрокиИгнорируются(t *testing.T) {
	rules := PolicyDPIRules(0xaab, 300, "eth3", []string{"", "167.17.177.54", ""})
	first := rules[0]
	if !argsHasPair(first.Args, "-d", "167.17.177.54") {
		t.Errorf("пустые IP должны игнорироваться, ожидалось первое правило с `-d 167.17.177.54`: %v", first.Args)
	}
}

func TestPolicyDPIRules_СчётчикПравил(t *testing.T) {
	excludes := []string{"1.1.1.1", "2.2.2.2"}
	rules := PolicyDPIRules(0xaab, 300, "eth3", excludes)
	wantCount := len(excludes) + len(PolicyDPITCPPorts) + len(PolicyDPIUDPPorts) + 1 // +1 — POSTROUTING jump
	if len(rules) != wantCount {
		t.Errorf("ожидалось %d правил (%d excludes + %d tcp + %d udp + 1 jump), получено %d",
			wantCount, len(excludes), len(PolicyDPITCPPorts), len(PolicyDPIUDPPorts), len(rules))
	}
}

// lastPostroutingJump возвращает последнее правило в mangle/POSTROUTING (jump
// в signcraze_policy_dpi). Используется для проверки -o $WAN-фильтра.
func lastPostroutingJump(rules []RuleSpec) *RuleSpec {
	for i := len(rules) - 1; i >= 0; i-- {
		r := rules[i]
		if r.Table == "mangle" && r.Chain == "POSTROUTING" {
			return &r
		}
	}
	return nil
}

// argsContains true если any arg равен needle.
func argsContains(args []string, needle string) bool {
	for _, a := range args {
		if a == needle {
			return true
		}
	}
	return false
}

// argsHasPair true если args содержит две последовательные строки key и value.
func argsHasPair(args []string, key, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}

// rulesContain — общая утилита для проверки наличия правил с substr-набором.
// Дублируется из hybrid_test.go чтобы не вводить shared helpers (тесты per-file).
var _ = strings.Contains // silence linter unused import (used in other tests)

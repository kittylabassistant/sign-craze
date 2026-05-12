package modes

import (
	"strings"
	"testing"
)

func TestDPIForwardRules_БезWANInterface_JumpБезФильтра(t *testing.T) {
	rules := DPIForwardRules(0x53, 300, "", nil)
	if len(rules) == 0 {
		t.Fatal("DPIForwardRules: пустой результат")
	}
	jump := lastForwardJump(rules)
	if jump == nil {
		t.Fatal("не найдено FORWARD jump-правило")
	}
	if argsContains(jump.Args, "-o") {
		t.Errorf("FORWARD jump с пустым wanIface не должно содержать `-o`: %v", jump.Args)
	}
	if !argsHasPair(jump.Args, "-j", DPIForwardChainName) {
		t.Errorf("FORWARD jump должен содержать `-j %s`: %v", DPIForwardChainName, jump.Args)
	}
}

func TestDPIForwardRules_СWANInterface_JumpСодержитO(t *testing.T) {
	rules := DPIForwardRules(0x53, 300, "eth3", nil)
	jump := lastForwardJump(rules)
	if jump == nil {
		t.Fatal("не найдено FORWARD jump-правило")
	}
	if !argsHasPair(jump.Args, "-o", "eth3") {
		t.Errorf("FORWARD jump должен содержать `-o eth3`: %v", jump.Args)
	}
	if !argsHasPair(jump.Args, "-j", DPIForwardChainName) {
		t.Errorf("FORWARD jump должен содержать `-j %s`: %v", DPIForwardChainName, jump.Args)
	}
}

func TestDPIForwardRules_JumpСодержитMarkФильтр(t *testing.T) {
	rules := DPIForwardRules(0x53, 300, "eth3", nil)
	jump := lastForwardJump(rules)
	if jump == nil {
		t.Fatal("не найдено FORWARD jump-правило")
	}
	argsStr := strings.Join(jump.Args, " ")
	if !strings.Contains(argsStr, "! --mark 0x53") {
		t.Errorf("FORWARD jump должен содержать фильтр `! --mark 0x53` для loop-prevention: %v", jump.Args)
	}
}

func TestDPIForwardRules_VPNExcludeIPs_RETURNПередNFQUEUE(t *testing.T) {
	excludes := []string{"167.17.177.54", "1.2.3.4"}
	rules := DPIForwardRules(0x53, 300, "eth3", excludes)

	// Первые len(excludes) правил в signcraze_dpi_fwd должны быть RETURN с -d
	for i, ip := range excludes {
		if i >= len(rules) {
			t.Fatalf("ожидалось %d RETURN-правил, получено %d правил всего", len(excludes), len(rules))
		}
		r := rules[i]
		if r.Chain != DPIForwardChainName {
			t.Errorf("RETURN-правило #%d должно быть в %s, получено %s", i, DPIForwardChainName, r.Chain)
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

func TestDPIForwardRules_VPNExcludeIPs_ПустыеСтрокиИгнорируются(t *testing.T) {
	rules := DPIForwardRules(0x53, 300, "eth3", []string{"", "167.17.177.54", ""})
	first := rules[0]
	if !argsHasPair(first.Args, "-d", "167.17.177.54") {
		t.Errorf("пустые IP должны игнорироваться, ожидалось первое правило с `-d 167.17.177.54`: %v", first.Args)
	}
}

func TestDPIForwardRules_СчётчикПравил(t *testing.T) {
	excludes := []string{"1.1.1.1", "2.2.2.2"}
	rules := DPIForwardRules(0x53, 300, "eth3", excludes)
	wantCount := len(excludes) + len(PolicyDPITCPPorts) + len(PolicyDPIUDPPorts) + 1 // +1 — FORWARD jump
	if len(rules) != wantCount {
		t.Errorf("ожидалось %d правил (%d excludes + %d tcp + %d udp + 1 jump), получено %d",
			wantCount, len(excludes), len(PolicyDPITCPPorts), len(PolicyDPIUDPPorts), len(rules))
	}
}

func TestDPIForwardRules_ВсеПортыПокрыты(t *testing.T) {
	rules := DPIForwardRules(0x53, 300, "eth3", nil)

	for _, port := range PolicyDPITCPPorts {
		found := false
		for _, r := range rules {
			if r.Chain != DPIForwardChainName {
				continue
			}
			if argsHasPair(r.Args, "--dport", port) && argsHasPair(r.Args, "-p", "tcp") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("TCP-порт %s не покрыт NFQUEUE-правилом", port)
		}
	}
	for _, port := range PolicyDPIUDPPorts {
		found := false
		for _, r := range rules {
			if r.Chain != DPIForwardChainName {
				continue
			}
			if argsHasPair(r.Args, "--dport", port) && argsHasPair(r.Args, "-p", "udp") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("UDP-порт %s не покрыт NFQUEUE-правилом", port)
		}
	}
}

func TestDPIForwardRules_ВсеВТаблицеMangle(t *testing.T) {
	rules := DPIForwardRules(0x53, 300, "eth3", []string{"1.1.1.1"})
	for _, r := range rules {
		if r.Table != "mangle" {
			t.Errorf("правило в %s/%s в таблице %q, ожидалась mangle", r.Table, r.Chain, r.Table)
		}
	}
}

// lastForwardJump возвращает последнее правило в mangle/FORWARD (jump в
// signcraze_dpi_fwd). Используется для проверки -o $WAN-фильтра и mark-фильтра.
func lastForwardJump(rules []RuleSpec) *RuleSpec {
	for i := len(rules) - 1; i >= 0; i-- {
		r := rules[i]
		if r.Table == "mangle" && r.Chain == "FORWARD" {
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

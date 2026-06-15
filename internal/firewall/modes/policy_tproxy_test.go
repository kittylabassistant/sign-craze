package modes

import (
	"strings"
	"testing"
)

// PolicyTProxyRules — REDIRECT-режим (xt_TPROXY отсутствует в kernel Keenetic 4.9).
// Имя функции оставлено для совместимости с applier.go; механизм — REDIRECT
// (DNAT в local stack), потому что TPROXY недоступен.

func TestPolicyTProxyRules_НулевойMark_ВозвращаетNil(t *testing.T) {
	rules := PolicyTProxyRules(0, 7895)
	if rules != nil {
		t.Errorf("PolicyTProxyRules(keenMark=0) должна возвращать nil, получено %v", rules)
	}
}

func TestPolicyTProxyRules_СодержитREDIRECTTarget(t *testing.T) {
	rules := PolicyTProxyRules(0xaab, 7895)
	if len(rules) == 0 {
		t.Fatal("PolicyTProxyRules: пустой результат")
	}

	found := false
	for _, r := range rules {
		for _, arg := range r.Args {
			if arg == "REDIRECT" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("PolicyTProxyRules: не найден REDIRECT target в правилах")
	}
}

func TestPolicyTProxyRules_TCP(t *testing.T) {
	rules := PolicyTProxyRules(0xaab, 7895)

	checks := []struct {
		desc    string
		chain   string
		needles []string
	}{
		{"tcp redirect", PolicyChainName, []string{"-p", "tcp", "REDIRECT"}},
		{"prerouting jump", "PREROUTING", []string{"-j", PolicyChainName}},
	}

	for _, c := range checks {
		if !rulesContain(rules, c.chain, c.needles...) {
			t.Errorf("правило %q в цепочке %s не найдено (искались substr: %v)", c.desc, c.chain, c.needles)
		}
	}
}

func TestPolicyTProxyRules_КорректныйПорт(t *testing.T) {
	rules := PolicyTProxyRules(0xaab, 7895)
	for _, r := range rules {
		joined := strings.Join(r.Args, " ")
		if strings.Contains(joined, "REDIRECT") && !strings.Contains(joined, "7895") {
			t.Errorf("REDIRECT правило не содержит порт 7895: %v", r.Args)
		}
	}
}

func TestPolicyTProxyRules_ВсеВNAT(t *testing.T) {
	rules := PolicyTProxyRules(0xaab, 7895)
	for _, r := range rules {
		if r.Table != "nat" {
			t.Errorf("REDIRECT правила должны быть в nat-таблице, получено %s: %v", r.Table, r.Args)
		}
	}
}

func TestPolicyTProxyRules_БезFORWARDACCEPT(t *testing.T) {
	rules := PolicyTProxyRules(0xaab, 7895)
	for _, r := range rules {
		if r.Table == "filter" && r.Chain == "FORWARD" {
			t.Errorf("REDIRECT-режим не должен содержать filter/FORWARD правил: %v", r.Args)
		}
	}
}

func TestPolicyTProxyRules_БезTCPMSS(t *testing.T) {
	rules := PolicyTProxyRules(0xaab, 7895)
	for _, r := range rules {
		for _, arg := range r.Args {
			if arg == "TCPMSS" {
				t.Errorf("REDIRECT-режим не нуждается в TCPMSS clamp: %v", r.Args)
			}
		}
	}
}

func TestPolicyTProxyRules_КастомныйПорт(t *testing.T) {
	rules := PolicyTProxyRules(0x1234, 9999)
	for _, r := range rules {
		joined := strings.Join(r.Args, " ")
		if strings.Contains(joined, "REDIRECT") && !strings.Contains(joined, "9999") {
			t.Errorf("REDIRECT правило не содержит кастомный порт 9999: %v", r.Args)
		}
	}
}

// --- PolicyTProxyRulesReal (реальный xt_TPROXY) ---

func TestPolicyTProxyRulesReal_НулевойMark_ВозвращаетNil(t *testing.T) {
	rules := PolicyTProxyRulesReal(0, 0x53, 7895)
	if rules != nil {
		t.Errorf("PolicyTProxyRulesReal(keenMark=0) должна возвращать nil, получено %v", rules)
	}
}

func TestPolicyTProxyRulesReal_СодержитTPROXYTarget(t *testing.T) {
	rules := PolicyTProxyRulesReal(0xaab, 0x53, 7895)
	if len(rules) == 0 {
		t.Fatal("PolicyTProxyRulesReal: пустой результат")
	}
	found := false
	for _, r := range rules {
		for _, arg := range r.Args {
			if arg == "TPROXY" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("PolicyTProxyRulesReal: не найден TPROXY target в правилах")
	}
}

func TestPolicyTProxyRulesReal_TCPиUDP(t *testing.T) {
	rules := PolicyTProxyRulesReal(0xaab, 0x53, 7895)

	checks := []struct {
		desc    string
		chain   string
		needles []string
	}{
		{"tcp tproxy", PolicyChainName, []string{"-p", "tcp", "TPROXY"}},
		{"udp tproxy", PolicyChainName, []string{"-p", "udp", "TPROXY"}},
		{"prerouting jump", "PREROUTING", []string{"-j", PolicyChainName}},
	}

	for _, c := range checks {
		if !rulesContain(rules, c.chain, c.needles...) {
			t.Errorf("правило %q в цепочке %s не найдено (искались: %v)", c.desc, c.chain, c.needles)
		}
	}
}

func TestPolicyTProxyRulesReal_КорректныйПорт(t *testing.T) {
	rules := PolicyTProxyRulesReal(0xaab, 0x53, 7895)
	for _, r := range rules {
		joined := strings.Join(r.Args, " ")
		if strings.Contains(joined, "TPROXY") && !strings.Contains(joined, "7895") {
			t.Errorf("TPROXY правило не содержит порт 7895: %v", r.Args)
		}
	}
}

func TestPolicyTProxyRulesReal_ВсеВMangle(t *testing.T) {
	rules := PolicyTProxyRulesReal(0xaab, 0x53, 7895)
	for _, r := range rules {
		if r.Table != "mangle" {
			t.Errorf("TPROXY правила должны быть в mangle, получено %s: %v", r.Table, r.Args)
		}
	}
}

func TestPolicyTProxyRulesReal_TProxyMark(t *testing.T) {
	// tproxy-mark должен содержать 0x53/0xffffffff
	rules := PolicyTProxyRulesReal(0xaab, 0x53, 7895)
	found := false
	for _, r := range rules {
		joined := strings.Join(r.Args, " ")
		if strings.Contains(joined, "0x53/0xffffffff") {
			found = true
			break
		}
	}
	if !found {
		t.Error("PolicyTProxyRulesReal: не найден --tproxy-mark 0x53/0xffffffff")
	}
}

func TestPolicyTProxyRulesReal_БезREDIRECT(t *testing.T) {
	rules := PolicyTProxyRulesReal(0xaab, 0x53, 7895)
	for _, r := range rules {
		for _, arg := range r.Args {
			if arg == "REDIRECT" {
				t.Errorf("TPROXY-режим не должен содержать REDIRECT: %v", r.Args)
			}
		}
	}
}

func TestPolicyTProxyRulesReal_БезTCPMSS(t *testing.T) {
	rules := PolicyTProxyRulesReal(0xaab, 0x53, 7895)
	for _, r := range rules {
		for _, arg := range r.Args {
			if arg == "TCPMSS" {
				t.Errorf("TPROXY-режим не нуждается в TCPMSS clamp: %v", r.Args)
			}
		}
	}
}

func TestPolicyTProxyRulesReal_БезFORWARDACCEPT(t *testing.T) {
	rules := PolicyTProxyRulesReal(0xaab, 0x53, 7895)
	for _, r := range rules {
		if r.Table == "filter" && r.Chain == "FORWARD" {
			t.Errorf("TPROXY-режим не должен содержать filter/FORWARD правил: %v", r.Args)
		}
	}
}

func TestPolicyTProxyRulesReal_OnIPLoopback(t *testing.T) {
	rules := PolicyTProxyRulesReal(0xaab, 0x53, 7895)
	for _, r := range rules {
		joined := strings.Join(r.Args, " ")
		if strings.Contains(joined, "TPROXY") && !strings.Contains(joined, "127.0.0.1") {
			t.Errorf("TPROXY правило должно содержать --on-ip 127.0.0.1: %v", r.Args)
		}
	}
}

func TestPolicyTProxyRulesReal_КастомныйПорт(t *testing.T) {
	rules := PolicyTProxyRulesReal(0x1234, 0x53, 9999)
	for _, r := range rules {
		joined := strings.Join(r.Args, " ")
		if strings.Contains(joined, "TPROXY") && !strings.Contains(joined, "9999") {
			t.Errorf("TPROXY правило не содержит кастомный порт 9999: %v", r.Args)
		}
	}
}

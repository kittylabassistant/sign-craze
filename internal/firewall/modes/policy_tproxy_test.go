package modes

import (
	"strings"
	"testing"
)

// PolicyTProxyRules — REDIRECT-режим (xt_TPROXY отсутствует в kernel Keenetic 4.9).
// Имя функции оставлено для совместимости с applier.go; механизм — REDIRECT
// (DNAT в local stack), потому что TPROXY недоступен.

func TestPolicyTProxyRules_НулевойMark_ВозвращаетNil(t *testing.T) {
	rules := PolicyTProxyRules(0, 0x53, 7895)
	if rules != nil {
		t.Errorf("PolicyTProxyRules(keenMark=0) должна возвращать nil, получено %v", rules)
	}
}

func TestPolicyTProxyRules_СодержитREDIRECTTarget(t *testing.T) {
	rules := PolicyTProxyRules(0xaab, 0x53, 7895)
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
	rules := PolicyTProxyRules(0xaab, 0x53, 7895)

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
	rules := PolicyTProxyRules(0xaab, 0x53, 7895)
	for _, r := range rules {
		joined := strings.Join(r.Args, " ")
		if strings.Contains(joined, "REDIRECT") && !strings.Contains(joined, "7895") {
			t.Errorf("REDIRECT правило не содержит порт 7895: %v", r.Args)
		}
	}
}

func TestPolicyTProxyRules_ВсеВNAT(t *testing.T) {
	rules := PolicyTProxyRules(0xaab, 0x53, 7895)
	for _, r := range rules {
		if r.Table != "nat" {
			t.Errorf("REDIRECT правила должны быть в nat-таблице, получено %s: %v", r.Table, r.Args)
		}
	}
}

func TestPolicyTProxyRules_БезFORWARDACCEPT(t *testing.T) {
	rules := PolicyTProxyRules(0xaab, 0x53, 7895)
	for _, r := range rules {
		if r.Table == "filter" && r.Chain == "FORWARD" {
			t.Errorf("REDIRECT-режим не должен содержать filter/FORWARD правил: %v", r.Args)
		}
	}
}

func TestPolicyTProxyRules_БезTCPMSS(t *testing.T) {
	rules := PolicyTProxyRules(0xaab, 0x53, 7895)
	for _, r := range rules {
		for _, arg := range r.Args {
			if arg == "TCPMSS" {
				t.Errorf("REDIRECT-режим не нуждается в TCPMSS clamp: %v", r.Args)
			}
		}
	}
}

func TestPolicyTProxyRules_КастомныйПорт(t *testing.T) {
	rules := PolicyTProxyRules(0x1234, 0x53, 9999)
	for _, r := range rules {
		joined := strings.Join(r.Args, " ")
		if strings.Contains(joined, "REDIRECT") && !strings.Contains(joined, "9999") {
			t.Errorf("REDIRECT правило не содержит кастомный порт 9999: %v", r.Args)
		}
	}
}


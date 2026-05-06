package modes

import (
	"strings"
	"testing"
)

// rulesContain — true, если хотя бы одно правило с заданным chain содержит
// все substr (для проверки наличия конкретных match'ей/target'ов в args).
func rulesContain(rules []RuleSpec, chain string, substrs ...string) bool {
nextRule:
	for _, r := range rules {
		if r.Chain != chain {
			continue
		}
		joined := strings.Join(r.Args, " ")
		for _, s := range substrs {
			if !strings.Contains(joined, s) {
				continue nextRule
			}
		}
		return true
	}
	return false
}

func TestTProxyRules_СодержитОбязательныеПравила(t *testing.T) {
	rules := TProxyRules(0x53)

	checks := []struct {
		desc    string
		chain   string
		needles []string
	}{
		{"mark-ipv4", "signcraze", []string{"--match-set signcraze_ipv4 dst", "-j MARK", "0x53"}},
		{"prerouting-dpi", "PREROUTING", []string{"-j signcraze_dpi"}},
		{"prerouting-mark", "PREROUTING", []string{"-j signcraze"}},
	}
	for _, c := range checks {
		if !rulesContain(rules, c.chain, c.needles...) {
			t.Errorf("правило %s в %s не найдено (нужны substr: %v)", c.desc, c.chain, c.needles)
		}
	}
}

func TestTProxyRules_ИспользуетКорректныйFWMark(t *testing.T) {
	rules := TProxyRules(0x53)
	for _, r := range rules {
		for i, arg := range r.Args {
			if (arg == "--set-mark" || arg == "--mark") && i+1 < len(r.Args) {
				if r.Args[i+1] != "0x53" {
					t.Errorf("неверный fwmark %q в правиле %v, ожидался 0x53", r.Args[i+1], r.Args)
				}
			}
		}
	}
}

func TestTProxyRules_БезTPROXYTarget(t *testing.T) {
	// TUN-mode: TPROXY-target недоступен на стоковом ядре Keenetic, не должен
	// появляться в правилах ни при каких условиях.
	rules := TProxyRules(0x53)
	for _, r := range rules {
		joined := strings.Join(r.Args, " ")
		if strings.Contains(joined, "TPROXY") || strings.Contains(joined, "--tproxy-port") {
			t.Errorf("правило содержит TPROXY (запрещено в TUN-mode): %v", r.Args)
		}
	}
}

func TestTProxyRules_ВсеВТаблицеMangle(t *testing.T) {
	rules := TProxyRules(0x53)
	for _, r := range rules {
		if r.Table != "mangle" {
			t.Errorf("правило в %s/%s в таблице %q, ожидалась mangle",
				r.Table, r.Chain, r.Table)
		}
	}
}

// TestTProxyRules_БезКомментариев — busybox iptables 1.4.21 на Keenetic
// часто без xt_comment. Контракт: правила пишутся без `-m comment`.
func TestTProxyRules_БезКомментариев(t *testing.T) {
	rules := TProxyRules(0x53)
	for _, r := range rules {
		for _, arg := range r.Args {
			if arg == "--comment" {
				t.Errorf("правило %s/%s содержит --comment: %v", r.Table, r.Chain, r.Args)
				break
			}
		}
	}
}

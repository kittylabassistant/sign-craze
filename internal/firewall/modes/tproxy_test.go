package modes

import (
	"strings"
	"testing"
)

// rulesContain — true, если хотя бы одно правило с заданными chain содержит
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
	rules := TProxyRules(7895, 0x53)

	checks := []struct {
		desc    string
		chain   string
		needles []string
	}{
		{"mark-ipv4", "signcraze", []string{"--match-set signcraze_ipv4 dst", "-j MARK"}},
		{"mark-ipv6", "signcraze", []string{"--match-set signcraze_ipv6 dst", "-j MARK"}},
		{"tproxy-tcp", "signcraze_full", []string{"-p tcp", "-j TPROXY"}},
		{"tproxy-udp", "signcraze_full", []string{"-p udp", "-j TPROXY"}},
		{"prerouting-dpi", "PREROUTING", []string{"-j signcraze_dpi"}},
		{"prerouting-mark", "PREROUTING", []string{"-j signcraze"}},
		{"prerouting-tproxy", "PREROUTING", []string{"-j signcraze_full"}},
	}
	for _, c := range checks {
		if !rulesContain(rules, c.chain, c.needles...) {
			t.Errorf("правило %s в %s не найдено (нужны substr: %v)", c.desc, c.chain, c.needles)
		}
	}
}

func TestTProxyRules_ИспользуетКорректныйFWMark(t *testing.T) {
	rules := TProxyRules(7895, 0x53)
	for _, r := range rules {
		for i, arg := range r.Args {
			if (arg == "--set-mark" || arg == "--mark" || arg == "--tproxy-mark") && i+1 < len(r.Args) {
				if r.Args[i+1] != "0x53" {
					t.Errorf("неверный fwmark %q в правиле %v, ожидался 0x53", r.Args[i+1], r.Args)
				}
			}
		}
	}
}

func TestTProxyRules_ИспользуетКорректныйПорт(t *testing.T) {
	rules := TProxyRules(7895, 0x53)
	portFound := false
	for _, r := range rules {
		for i, arg := range r.Args {
			if arg == "--tproxy-port" && i+1 < len(r.Args) {
				if r.Args[i+1] == "7895" {
					portFound = true
				}
			}
		}
	}
	if !portFound {
		t.Error("порт 7895 не найден в tproxy-правилах")
	}
}

func TestTProxyRules_ВсеВТаблицеMangle(t *testing.T) {
	rules := TProxyRules(7895, 0x53)
	for _, r := range rules {
		if r.Table != "mangle" {
			t.Errorf("правило в %s/%s в таблице %q, ожидалась mangle",
				r.Table, r.Chain, r.Table)
		}
	}
}

// TestTProxyRules_BypassLoopback проверяет наличие RETURN-bypass правил
// для loopback (127.0.0.0/8), link-local (169.254.0.0/16) и интерфейса lo
// перед TPROXY-правилами — иначе TCP/UDP-стек роутера ломается
// (safety-fixes #6). Раздельные правила вместо `! -s X ! -s Y` в одном
// rule из-за iptables 1.4.21 (Keenetic) не принимающего multiple -s.
func TestTProxyRules_BypassLoopback(t *testing.T) {
	rules := TProxyRules(7895, 0x53)
	wantBypass := []struct {
		desc string
		need string
	}{
		{"bypass-loopback-src", "-s 127.0.0.0/8"},
		{"bypass-linklocal-src", "-s 169.254.0.0/16"},
		{"bypass-lo-iface", "-i lo"},
	}
	for _, w := range wantBypass {
		if !rulesContain(rules, "signcraze_full", w.need, "-j RETURN") {
			t.Errorf("отсутствует bypass-правило %s (нужно %q + -j RETURN в signcraze_full)", w.desc, w.need)
		}
	}
}

// TestTProxyRules_БезКомментариев — busybox iptables 1.4.21 на Keenetic
// часто без xt_comment. Контракт: правила пишутся без `-m comment`.
func TestTProxyRules_БезКомментариев(t *testing.T) {
	rules := TProxyRules(7895, 0x53)
	for _, r := range rules {
		for _, arg := range r.Args {
			if arg == "--comment" {
				t.Errorf("правило %s/%s содержит --comment: %v", r.Table, r.Chain, r.Args)
				break
			}
		}
	}
}

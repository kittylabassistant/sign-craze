package modes

import (
	"strings"
	"testing"
)

func TestHybridRules_СодержитMARKПравила(t *testing.T) {
	rules := HybridRules(0x53, 200)

	checks := []struct {
		desc    string
		chain   string
		needles []string
	}{
		{"mark-ipv4", "signcraze", []string{"--match-set signcraze_ipv4 dst", "-j MARK"}},
		{"prerouting-dpi", "PREROUTING", []string{"-j signcraze_dpi"}},
		{"prerouting-mark", "PREROUTING", []string{"-j signcraze"}},
	}
	for _, c := range checks {
		if !rulesContain(rules, c.chain, c.needles...) {
			t.Errorf("правило %s в %s не найдено (нужны substr: %v)", c.desc, c.chain, c.needles)
		}
	}
}

func TestHybridRules_СодержитNFQUEUE(t *testing.T) {
	rules := HybridRules(0x53, 200)

	if !rulesContain(rules, "signcraze_dpi", "-p tcp", "-j NFQUEUE") {
		t.Error("NFQUEUE TCP в signcraze_dpi отсутствует")
	}
	if !rulesContain(rules, "signcraze_dpi", "-p udp", "-j NFQUEUE") {
		t.Error("NFQUEUE UDP в signcraze_dpi отсутствует")
	}
}

func TestHybridRules_NFQUEUEИспользуетКорректныйНомерОчереди(t *testing.T) {
	rules := HybridRules(0x53, 200)
	found := false
	for _, r := range rules {
		if r.Chain != "signcraze_dpi" {
			continue
		}
		for i, arg := range r.Args {
			if arg == "--queue-num" && i+1 < len(r.Args) && r.Args[i+1] == "200" {
				found = true
			}
		}
	}
	if !found {
		t.Error("--queue-num 200 не найден в signcraze_dpi правилах")
	}
}

func TestHybridRules_DPIПравилаИдутПервыми(t *testing.T) {
	rules := HybridRules(0x53, 200)
	if len(rules) == 0 {
		t.Fatal("нет правил")
	}
	if rules[0].Chain != "signcraze_dpi" {
		t.Errorf("первое правило в цепочке %q, ожидалось signcraze_dpi", rules[0].Chain)
	}
}

func TestHybridRules_DPIПравилаПроверяютОтсутствиеМетки(t *testing.T) {
	rules := HybridRules(0x53, 200)
	for _, r := range rules {
		if r.Chain != "signcraze_dpi" {
			continue
		}
		argsStr := strings.Join(r.Args, " ")
		if !strings.Contains(argsStr, "! --mark 0x53") {
			t.Errorf("DPI-правило %v не проверяет отсутствие метки 0x53", r.Args)
		}
	}
}

func TestHybridRules_БезTPROXY(t *testing.T) {
	// TUN-mode: TPROXY-target недоступен на стоковом ядре Keenetic.
	rules := HybridRules(0x53, 200)
	for _, r := range rules {
		joined := strings.Join(r.Args, " ")
		if strings.Contains(joined, "TPROXY") {
			t.Errorf("правило содержит TPROXY (запрещено в TUN-mode): %v", r.Args)
		}
	}
}

func TestHybridRules_ВсеВТаблицеMangle(t *testing.T) {
	rules := HybridRules(0x53, 200)
	for _, r := range rules {
		if r.Table != "mangle" {
			t.Errorf("правило в %s/%s в таблице %q, ожидалась mangle",
				r.Table, r.Chain, r.Table)
		}
	}
}

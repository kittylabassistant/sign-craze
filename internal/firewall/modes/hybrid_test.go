package modes

import (
	"strings"
	"testing"
)

func TestHybridRules_СодержитTProxyПравила(t *testing.T) {
	rules := HybridRules(7895, 0x53, 200)

	// Все tproxy-правила должны присутствовать
	tproxyComments := []string{
		"signcraze:mark-ipv4",
		"signcraze:mark-ipv6",
		"signcraze:tproxy-tcp",
		"signcraze:tproxy-udp",
		"signcraze:prerouting-dpi",
		"signcraze:prerouting-mark",
	}
	for _, c := range tproxyComments {
		if !rulesContainComment(rules, c) {
			t.Errorf("правило с комментарием %q отсутствует в hybrid-наборе", c)
		}
	}
}

func TestHybridRules_СодержитNFQUEUE(t *testing.T) {
	rules := HybridRules(7895, 0x53, 200)

	// NFQUEUE-правила в signcraze_dpi
	dpiComments := []string{"signcraze:dpi-tcp", "signcraze:dpi-udp"}
	for _, c := range dpiComments {
		if !rulesContainComment(rules, c) {
			t.Errorf("NFQUEUE-правило с комментарием %q отсутствует", c)
		}
	}
}

func TestHybridRules_NFQUEUEИспользуетКорректныйНомерОчереди(t *testing.T) {
	rules := HybridRules(7895, 0x53, 200)
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
	rules := HybridRules(7895, 0x53, 200)
	if len(rules) == 0 {
		t.Fatal("нет правил")
	}
	// Первые правила должны быть в signcraze_dpi
	if rules[0].Chain != "signcraze_dpi" {
		t.Errorf("первое правило в цепочке %q, ожидалось signcraze_dpi", rules[0].Chain)
	}
}

func TestHybridRules_DPIПравилаПроверяютОтсутствиеМетки(t *testing.T) {
	rules := HybridRules(7895, 0x53, 200)
	for _, r := range rules {
		if r.Chain != "signcraze_dpi" {
			continue
		}
		// NFQUEUE применяется только к трафику без метки: "!" "--mark" "0x53"
		argsStr := strings.Join(r.Args, " ")
		if !strings.Contains(argsStr, "! --mark 0x53") {
			t.Errorf("DPI-правило %v не проверяет отсутствие метки 0x53", r.Args)
		}
	}
}

func TestHybridRules_ВсеВТаблицеMangle(t *testing.T) {
	rules := HybridRules(7895, 0x53, 200)
	for _, r := range rules {
		if r.Table != "mangle" {
			t.Errorf("правило %s/%s в таблице %q, ожидалась mangle",
				r.Chain, findComment(r.Args), r.Table)
		}
	}
}

func rulesContainComment(rules []RuleSpec, comment string) bool {
	for _, r := range rules {
		if findComment(r.Args) == comment {
			return true
		}
	}
	return false
}

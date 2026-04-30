package modes

import "strconv"

// ExcludeRules возвращает правило bypass: пакеты с dst в ipset signcraze_excludes
// получают RETURN из цепочки signcraze ДО достижения MARK-правил.
// Должно применяться через iptables -I signcraze 1 — Applier использует InsertRule,
// гарантируя порядок независимо от прошлого состояния цепочки (safety-fixes #1).
func ExcludeRules() []RuleSpec {
	return []RuleSpec{
		{
			Table: "mangle", Chain: "signcraze",
			Args: []string{
				"-m", "set", "--match-set", "signcraze_excludes", "dst",
				"-j", "RETURN",
			},
		},
	}
}

// AdminPortsBypassRules возвращает RETURN-правила для трафика на admin порты
// (TCP+UDP на каждый порт), защищая SSH и web admin от случайного попадания
// в TPROXY (safety-fixes #2). Порты со значением 0 пропускаются.
// Пустой/nil slice → возвращает nil.
func AdminPortsBypassRules(ports []uint16) []RuleSpec {
	var rules []RuleSpec
	for _, port := range ports {
		if port == 0 {
			continue
		}
		portStr := strconv.Itoa(int(port))
		rules = append(rules,
			RuleSpec{
				Table: "mangle", Chain: "signcraze",
				Args: []string{"-p", "tcp", "--dport", portStr, "-j", "RETURN"},
			},
			RuleSpec{
				Table: "mangle", Chain: "signcraze",
				Args: []string{"-p", "udp", "--dport", portStr, "-j", "RETURN"},
			},
		)
	}
	return rules
}

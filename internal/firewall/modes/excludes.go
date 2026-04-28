package modes

import "fmt"

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
				"-m", "comment", "--comment", "signcraze:exclude-bypass",
			},
		},
	}
}

// AdminPortBypassRule возвращает RETURN-правила для трафика на admin port (TCP+UDP),
// защищая SSH и web admin от случайного попадания в TPROXY (safety-fixes #2).
// При port=0 возвращает пустой slice.
func AdminPortBypassRule(port uint16) []RuleSpec {
	if port == 0 {
		return nil
	}
	portStr := fmt.Sprintf("%d", port)
	return []RuleSpec{
		{
			Table: "mangle", Chain: "signcraze",
			Args: []string{
				"-p", "tcp", "--dport", portStr,
				"-j", "RETURN",
				"-m", "comment", "--comment", "signcraze:admin-port-bypass-tcp",
			},
		},
		{
			Table: "mangle", Chain: "signcraze",
			Args: []string{
				"-p", "udp", "--dport", portStr,
				"-j", "RETURN",
				"-m", "comment", "--comment", "signcraze:admin-port-bypass-udp",
			},
		},
	}
}

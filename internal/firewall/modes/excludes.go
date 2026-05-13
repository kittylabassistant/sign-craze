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

// AdminPortsBypassRulesForChain — обобщённая версия AdminPortsBypassRules
// с явным table+chainName. Позволяет переиспользовать в policy mode
// (table="mangle"/"nat", chain="signcraze_policy") в дополнение к
// full mode (table="mangle", chain="signcraze").
func AdminPortsBypassRulesForChain(ports []uint16, table, chainName string) []RuleSpec {
	var rules []RuleSpec
	for _, port := range ports {
		if port == 0 {
			continue
		}
		portStr := strconv.Itoa(int(port))
		rules = append(rules,
			RuleSpec{
				Table: table, Chain: chainName,
				Args: []string{"-p", "tcp", "--dport", portStr, "-j", "RETURN"},
			},
			RuleSpec{
				Table: table, Chain: chainName,
				Args: []string{"-p", "udp", "--dport", portStr, "-j", "RETURN"},
			},
		)
	}
	return rules
}

// AdminPortsBypassRules — back-compat обёртка для full-mode "signcraze" chain.
func AdminPortsBypassRules(ports []uint16) []RuleSpec {
	return AdminPortsBypassRulesForChain(ports, "mangle", "signcraze")
}

// LocalBypassRules возвращает RETURN-правила для трафика с dst = LAN-IP роутера.
// Защита SSH/web-admin от перехвата в TPROXY в режиме policy: Keenetic метит
// mark всех policy-устройств, включая пакеты с dst=LAN_IP роутера. Без bypass
// эти пакеты уходят в sing-box, который их не обслуживает → коннект виснет.
// Вставлять в pos=1 ДО TPROXY/MARK правил через InsertRule. Пустой lanIPs → nil.
func LocalBypassRules(table, chainName string, lanIPs []string) []RuleSpec {
	var rules []RuleSpec
	for _, ip := range lanIPs {
		if ip == "" {
			continue
		}
		rules = append(rules, RuleSpec{
			Table: table, Chain: chainName,
			Args: []string{"-d", ip, "-j", "RETURN"},
		})
	}
	return rules
}

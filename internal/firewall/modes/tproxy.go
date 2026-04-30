package modes

import "fmt"

// RuleSpec описывает одно правило iptables для передачи в IPTables.EnsureRule / DeleteRule.
type RuleSpec struct {
	Table string
	Chain string
	Args  []string
}

// TProxyRules возвращает набор правил iptables для режима proxy (tproxy).
// Все правила помечены комментариями вида "signcraze:*" для идентификации при откате.
func TProxyRules(port uint16, fwmark uint32) []RuleSpec {
	mark := fmt.Sprintf("0x%x", fwmark)
	portStr := fmt.Sprintf("%d", port)

	return []RuleSpec{
		// mark-маршрутизация по ipset signcraze_ipv4
		{
			Table: "mangle", Chain: "signcraze",
			Args: []string{
				"-m", "set", "--match-set", "signcraze_ipv4", "dst",
				"-j", "MARK", "--set-mark", mark,
				"-m", "comment", "--comment", "signcraze:mark-ipv4",
			},
		},
		// mark-маршрутизация по ipset signcraze_ipv6
		{
			Table: "mangle", Chain: "signcraze",
			Args: []string{
				"-m", "set", "--match-set", "signcraze_ipv6", "dst",
				"-j", "MARK", "--set-mark", mark,
				"-m", "comment", "--comment", "signcraze:mark-ipv6",
			},
		},
		// Bypass-RETURN внутри signcraze_full: разделяем -s фильтры на
		// отдельные правила (iptables 1.4.21 не принимает несколько -s
		// флагов в одном rule).
		{
			Table: "mangle", Chain: "signcraze_full",
			Args: []string{
				"-s", "127.0.0.0/8",
				"-j", "RETURN",
				"-m", "comment", "--comment", "signcraze:bypass-loopback-src",
			},
		},
		{
			Table: "mangle", Chain: "signcraze_full",
			Args: []string{
				"-s", "169.254.0.0/16",
				"-j", "RETURN",
				"-m", "comment", "--comment", "signcraze:bypass-linklocal-src",
			},
		},
		{
			Table: "mangle", Chain: "signcraze_full",
			Args: []string{
				"-i", "lo",
				"-j", "RETURN",
				"-m", "comment", "--comment", "signcraze:bypass-lo-iface",
			},
		},
		// tproxy TCP для помеченных пакетов; bypass-правила выше не дают
		// loopback/link-local дойти сюда (safety-fixes #6).
		{
			Table: "mangle", Chain: "signcraze_full",
			Args: []string{
				"-m", "mark", "--mark", mark,
				"-p", "tcp",
				"-j", "TPROXY", "--tproxy-port", portStr, "--tproxy-mark", mark,
				"-m", "comment", "--comment", "signcraze:tproxy-tcp",
			},
		},
		// tproxy UDP для помеченных пакетов.
		{
			Table: "mangle", Chain: "signcraze_full",
			Args: []string{
				"-m", "mark", "--mark", mark,
				"-p", "udp",
				"-j", "TPROXY", "--tproxy-port", portStr, "--tproxy-mark", mark,
				"-m", "comment", "--comment", "signcraze:tproxy-udp",
			},
		},
		// переход в signcraze_dpi (пустая в proxy-режиме, заполняется в hybrid)
		{
			Table: "mangle", Chain: "PREROUTING",
			Args: []string{
				"-j", "signcraze_dpi",
				"-m", "comment", "--comment", "signcraze:prerouting-dpi",
			},
		},
		// переход в signcraze для mark-маршрутизации
		{
			Table: "mangle", Chain: "PREROUTING",
			Args: []string{
				"-j", "signcraze",
				"-m", "comment", "--comment", "signcraze:prerouting-mark",
			},
		},
		// переход в signcraze_full для TPROXY (после mark-маршрутизации).
		{
			Table: "mangle", Chain: "PREROUTING",
			Args: []string{
				"-j", "signcraze_full",
				"-m", "comment", "--comment", "signcraze:prerouting-tproxy",
			},
		},
	}
}

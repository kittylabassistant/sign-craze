package modes

import "fmt"

// RuleSpec описывает одно правило iptables для передачи в IPTables.EnsureRule / DeleteRule.
type RuleSpec struct {
	Table string
	Chain string
	Args  []string
}

// TProxyRules возвращает набор правил iptables для режима ModeFull (TUN-mode).
//
// Маркирует пакеты с dst в ipset signcraze_ipv4/v6 нашим fwmark, чтобы ip rule
// направил их в таблицу с default через signbox-tun. Имя функции сохранено для
// совместимости с существующими caller'ами и тестами; реальная семантика —
// MARK-маршрутизация в TUN, без зависимости от kernel xt_TPROXY (которого нет
// на стоковом ядре Keenetic).
func TProxyRules(fwmark uint32) []RuleSpec {
	mark := fmt.Sprintf("0x%x", fwmark)

	return []RuleSpec{
		// Маркировка по ipset signcraze_ipv4: dst-host в списке → MARK.
		{
			Table: "mangle", Chain: "signcraze",
			Args: []string{
				"-m", "set", "--match-set", "signcraze_ipv4", "dst",
				"-j", "MARK", "--set-mark", mark,
			},
		},
		// Маркировка по ipset signcraze_ipv6.
		{
			Table: "mangle", Chain: "signcraze",
			Args: []string{
				"-m", "set", "--match-set", "signcraze_ipv6", "dst",
				"-j", "MARK", "--set-mark", mark,
			},
		},
		// Переход в signcraze_dpi (пустая в proxy-режиме, заполняется в hybrid).
		{
			Table: "mangle", Chain: "PREROUTING",
			Args: []string{"-j", "signcraze_dpi"},
		},
		// Переход в signcraze для mark-маршрутизации.
		{
			Table: "mangle", Chain: "PREROUTING",
			Args: []string{"-j", "signcraze"},
		},
	}
}

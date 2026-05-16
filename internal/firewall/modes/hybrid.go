package modes

import "fmt"

// HybridRules возвращает набор правил iptables для режима hybrid (TUN + nfqws2).
// Включает все правила TProxyRules плюс правила NFQUEUE в цепочке signcraze_dpi.
func HybridRules(fwmark uint32, nfqueueNum int) []RuleSpec {
	rules := TProxyRules(fwmark)
	mark := fmt.Sprintf("0x%x", fwmark)
	queue := fmt.Sprintf("%d", nfqueueNum)

	// NFQUEUE-правила добавляются в signcraze_dpi (до mark-маршрутизации).
	// Comment-теги нужны для cleanup'а и идентификации правил sign-craze
	// при ручной диагностике через `iptables -t mangle -L -nv`.
	dpiRules := []RuleSpec{
		// TCP трафик без нашей метки → NFQUEUE (nfqws2 обрабатывает).
		// Без -m comment: busybox iptables 1.4.21 на стоковом Keenetic
		// может не иметь libxt_comment.so → правило упадёт с ошибкой.
		// Chain-имя signcraze_dpi достаточно для диагностики.
		{
			Table: "mangle", Chain: "signcraze_dpi",
			Args: []string{
				"-m", "mark", "!", "--mark", mark,
				"-p", "tcp",
				"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
			},
		},
		// UDP трафик без нашей метки → NFQUEUE.
		{
			Table: "mangle", Chain: "signcraze_dpi",
			Args: []string{
				"-m", "mark", "!", "--mark", mark,
				"-p", "udp",
				"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
			},
		},
	}

	// DPI-правила идут первыми (до PREROUTING-правил).
	return append(dpiRules, rules...)
}

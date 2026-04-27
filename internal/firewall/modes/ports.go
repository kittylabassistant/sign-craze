package modes

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// MaxPortsPerRule — лимит iptables `multiport` (15 портов на правило).
const MaxPortsPerRule = 15

// PortRules возвращает правила port-based маркировки в цепочке signcraze_ports.
// Каждое правило батчит до MaxPortsPerRule портов через iptables multiport.
// Также добавляет переход PREROUTING -> signcraze_ports (до signcraze).
func PortRules(ports []uint16, fwmark uint32) []RuleSpec {
	if len(ports) == 0 {
		return nil
	}

	mark := fmt.Sprintf("0x%x", fwmark)

	// Сортируем для стабильного вывода (golden-тесты).
	sorted := append([]uint16(nil), ports...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var rules []RuleSpec
	for batchIdx, batch := range chunk(sorted, MaxPortsPerRule) {
		portList := joinPorts(batch)
		comment := fmt.Sprintf("signcraze:port-mark-tcp-%d", batchIdx)
		rules = append(rules, RuleSpec{
			Table: "mangle", Chain: "signcraze_ports",
			Args: []string{
				"-p", "tcp",
				"-m", "multiport", "--dports", portList,
				"-j", "MARK", "--set-mark", mark,
				"-m", "comment", "--comment", comment,
			},
		})
		comment = fmt.Sprintf("signcraze:port-mark-udp-%d", batchIdx)
		rules = append(rules, RuleSpec{
			Table: "mangle", Chain: "signcraze_ports",
			Args: []string{
				"-p", "udp",
				"-m", "multiport", "--dports", portList,
				"-j", "MARK", "--set-mark", mark,
				"-m", "comment", "--comment", comment,
			},
		})
	}

	// Переход PREROUTING -> signcraze_ports.
	rules = append(rules, RuleSpec{
		Table: "mangle", Chain: "PREROUTING",
		Args: []string{
			"-j", "signcraze_ports",
			"-m", "comment", "--comment", "signcraze:prerouting-ports",
		},
	})

	return rules
}

func chunk(items []uint16, size int) [][]uint16 {
	var out [][]uint16
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		out = append(out, items[i:end])
	}
	return out
}

func joinPorts(ports []uint16) string {
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = strconv.Itoa(int(p))
	}
	return strings.Join(parts, ",")
}

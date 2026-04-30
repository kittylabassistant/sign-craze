package modes

import "fmt"

// PolicyChainName — имя цепочки mangle, в которой sign-craze ставит TPROXY-правила
// для трафика, помеченного fwmark IP-policy Keenetic.
const PolicyChainName = "signcraze_policy"

// PolicyDPIChainName — имя цепочки mangle для NFQUEUE-хука в режиме policy.
// Отдельная цепочка от signcraze_dpi (используется в режиме full).
const PolicyDPIChainName = "signcraze_policy_dpi"

// PolicyRules возвращает правила для режима ModePolicy.
//
// keenMark — fwmark, присвоенный Keenetic'ом IP-policy через RCI (читается из
// /rci/show/ip/policy → mark, hex-строка). loopMark — собственный fwmark
// sign-craze (0x53), которым sing-box помечает свои исходящие пакеты для
// предотвращения петли (TPROXY ставит его автоматически через --tproxy-mark).
//
// Особенность: пакеты от sing-box (loopMark) НЕ должны повторно попадать в
// signcraze_policy. Достигается тем, что Keenetic policy mark на пакетах
// sing-box отсутствует — фильтр "-m mark --mark keenMark" просто не
// срабатывает. Дополнительно фильтруем loopback и link-local.
func PolicyRules(port uint16, keenMark, loopMark uint32) []RuleSpec {
	keen := fmt.Sprintf("0x%x", keenMark)
	loop := fmt.Sprintf("0x%x", loopMark)
	portStr := fmt.Sprintf("%d", port)

	// Защита: если RCI ещё не присвоил mark, не возвращаем правила
	// (TPROXY с mark=0 сматчит весь трафик и сломает роутер).
	if keenMark == 0 {
		return nil
	}
	_ = loop // зарезервировано для будущей опциональной anti-loop-проверки

	return []RuleSpec{
		// TPROXY TCP: помеченные Keenetic'ом пакеты идут в sing-box.
		{
			Table: "mangle", Chain: PolicyChainName,
			Args: []string{
				"!", "-s", "127.0.0.0/8",
				"!", "-s", "169.254.0.0/16",
				"!", "-i", "lo",
				"-m", "mark", "--mark", keen,
				"-p", "tcp",
				"-j", "TPROXY", "--tproxy-port", portStr, "--tproxy-mark", loop,
				"-m", "comment", "--comment", "signcraze:tproxy-tcp",
			},
		},
		// TPROXY UDP.
		{
			Table: "mangle", Chain: PolicyChainName,
			Args: []string{
				"!", "-s", "127.0.0.0/8",
				"!", "-s", "169.254.0.0/16",
				"!", "-i", "lo",
				"-m", "mark", "--mark", keen,
				"-p", "udp",
				"-j", "TPROXY", "--tproxy-port", portStr, "--tproxy-mark", loop,
				"-m", "comment", "--comment", "signcraze:tproxy-udp",
			},
		},
		// Переход PREROUTING → signcraze_policy.
		{
			Table: "mangle", Chain: "PREROUTING",
			Args: []string{
				"-j", PolicyChainName,
				"-m", "comment", "--comment", "signcraze:prerouting-policy",
			},
		},
	}
}

// PolicyDPIRules возвращает правила NFQUEUE для режима policy.
// Применяются только когда DPIEnabled=true.
//
// Фильтр "-m mark --mark keenMark" гарантирует, что NFQUEUE срабатывает
// ТОЛЬКО на трафике устройств, привязанных к policy в Keenetic.
func PolicyDPIRules(keenMark uint32, nfqueueNum int) []RuleSpec {
	if keenMark == 0 {
		return nil
	}
	keen := fmt.Sprintf("0x%x", keenMark)
	queue := fmt.Sprintf("%d", nfqueueNum)

	return []RuleSpec{
		{
			Table: "mangle", Chain: PolicyDPIChainName,
			Args: []string{
				"-m", "mark", "--mark", keen,
				"-p", "tcp",
				"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
				"-m", "comment", "--comment", "signcraze:dpi-tcp",
			},
		},
		{
			Table: "mangle", Chain: PolicyDPIChainName,
			Args: []string{
				"-m", "mark", "--mark", keen,
				"-p", "udp",
				"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
				"-m", "comment", "--comment", "signcraze:dpi-udp",
			},
		},
		// Переход PREROUTING → signcraze_policy_dpi (вставляется ПЕРЕД signcraze_policy).
		{
			Table: "mangle", Chain: "PREROUTING",
			Args: []string{
				"-j", PolicyDPIChainName,
				"-m", "comment", "--comment", "signcraze:prerouting-policy-dpi",
			},
		},
	}
}

package modes

import "fmt"

// PolicyChainName — имя цепочки mangle, в которой sign-craze ставит правила
// маркировки трафика, помеченного fwmark IP-policy Keenetic, для перенаправления
// в TUN-интерфейс sing-box.
const PolicyChainName = "signcraze_policy"

// PolicyDPIChainName — имя цепочки mangle для NFQUEUE-хука в режиме policy.
// Отдельная цепочка от signcraze_dpi (используется в режиме full).
const PolicyDPIChainName = "signcraze_policy_dpi"

// PolicyRules возвращает правила для режима ModePolicy (TUN-mode).
//
// keenMark — fwmark, присвоенный Keenetic'ом IP-policy через RCI (читается из
// /rci/show/ip/policy → mark). loopMark — собственный fwmark sign-craze (0x53),
// которым мы помечаем пакеты для подъёма в нашу таблицу маршрутизации
// (ip rule fwmark loopMark → table → default dev signbox-tun).
//
// Правила работают только на ingress (PREROUTING): локально сгенерированные
// пакеты sing-box (исходящие к upstream) идут через OUTPUT и не попадают в
// signcraze_policy → loop-prevention обеспечен автоматически без bypass-фильтров.
func PolicyRules(keenMark, loopMark uint32) []RuleSpec {
	if keenMark == 0 {
		// Без mark от Keenetic policy сматчит весь трафик и сломает роутер.
		return nil
	}
	keen := fmt.Sprintf("0x%x", keenMark)
	loop := fmt.Sprintf("0x%x", loopMark)

	return []RuleSpec{
		// Перемаркировать пакеты Keenetic-policy в наш fwmark, чтобы ip rule
		// направил их в нашу таблицу с default через TUN.
		{
			Table: "mangle", Chain: PolicyChainName,
			Args: []string{
				"-m", "mark", "--mark", keen,
				"-j", "MARK", "--set-mark", loop,
			},
		},
		// Переход PREROUTING → signcraze_policy.
		{
			Table: "mangle", Chain: "PREROUTING",
			Args: []string{"-j", PolicyChainName},
		},
		// Keenetic FORWARD policy = DROP, NDM ACL chains не делают ACCEPT для
		// нерегистрированных интерфейсов. Без этих правил пакеты с mark 0x53
		// после ip rule lookup (table 83 → dev signbox-tun) дропаются default
		// policy DROP до того, как kernel запишет в TUN fd.
		{
			Table: "filter", Chain: "FORWARD",
			Args: []string{"-o", PolicyTUNDeviceName, "-j", "ACCEPT"},
		},
		{
			Table: "filter", Chain: "FORWARD",
			Args: []string{"-i", PolicyTUNDeviceName, "-j", "ACCEPT"},
		},
	}
}

// PolicyTUNDeviceName — имя TUN-интерфейса в правилах filter/FORWARD.
// Должно совпадать с firewall.TUNDeviceName / singbox.DefaultTUNInterfaceName.
const PolicyTUNDeviceName = "signbox-tun"

// PolicyDPIRules возвращает правила NFQUEUE для режима policy.
// Применяются только когда DPIEnabled=true.
//
// **Архитектурно важно:** правила висят в POSTROUTING, НЕ в PREROUTING.
// Причина — в режиме `policy` весь LAN-трафик с keenetic-mark переотмечается
// в наш fwmark 0x53 → ip rule lookup → table 83 → signbox-tun. Sing-box
// получает соединение и сам открывает исходящий ClientHello к ISP.
// PREROUTING NFQUEUE десинхронизирует пакет от LAN-клиента, но sing-box
// потом этот пакет НЕ пробрасывает — он формирует свой собственный поток
// к ISP. nfqws2-десинхронизация теряется, ISP видит чистый ClientHello и
// блокирует SNI=youtube.com / discord.com.
//
// Правильное место — POSTROUTING на исходящем интерфейсе, где пакет
// уже после sing-box и реально уходит на провайдера. NFQUEUE здесь
// перехватит ClientHello sing-box → nfqws2 desync → выпуск на ISP.
//
// Фильтр `--dport 443` ограничивает обработку TLS+QUIC трафика — основной
// канал DPI-блокировок на mtgs/Москва. HTTP (80) обычно не блокируется
// и не нуждается в desync.
//
// keenMark передаётся для совместимости с прежним signature, но в
// POSTROUTING-схеме не используется (mark ставится в PREROUTING до того,
// как пакет дойдёт до POSTROUTING; sing-box свои пакеты помечает 0x53).
func PolicyDPIRules(keenMark uint32, nfqueueNum int) []RuleSpec {
	queue := fmt.Sprintf("%d", nfqueueNum)
	_ = keenMark // зарезервирован для будущего фильтра по mark

	return []RuleSpec{
		{
			Table: "mangle", Chain: PolicyDPIChainName,
			Args: []string{
				"-p", "tcp", "--dport", "443",
				"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
			},
		},
		{
			Table: "mangle", Chain: PolicyDPIChainName,
			Args: []string{
				"-p", "udp", "--dport", "443",
				"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
			},
		},
		// Переход POSTROUTING → signcraze_policy_dpi.
		{
			Table: "mangle", Chain: "POSTROUTING",
			Args: []string{"-j", PolicyDPIChainName},
		},
	}
}

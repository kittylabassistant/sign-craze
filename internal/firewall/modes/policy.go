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

// PolicyDPITCPPorts — TCP-порты, обрабатываемые nfqws2 в режиме policy.
// Каждый элемент — single port или range (`80` / `2053:2096`), на одно
// правило iptables. xt_multiport отсутствует в стоковом ядре Keenetic 4.9,
// поэтому используется multiple `--dport` правил вместо `-m multiport`.
//
//   - 80          — HTTP (некоторые ISP блокируют по Host: header)
//   - 443         — HTTPS/TLS (основной канал YT/Discord/Google блокировок)
//   - 2053:2096   — Cloudflare альтернативные TLS-порты (Discord media, CDN)
//   - 8443        — альтернативный HTTPS, используется Discord и рядом
//     сервисов для обхода базовых блокировок 443
var PolicyDPITCPPorts = []string{"80", "443", "2053:2096", "8443"}

// PolicyDPIUDPPorts — UDP-порты, обрабатываемые nfqws2 в режиме policy.
//
//   - 443           — QUIC/HTTP3 (YouTube, Google CDN)
//   - 19200:19400   — Discord RTP (голосовые каналы)
//   - 50000:50100   — Discord voice P2P / WebRTC ICE
var PolicyDPIUDPPorts = []string{"443", "19200:19400", "50000:50100"}

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
// Списки портов (PolicyDPITCPPorts/UDPPorts) покрывают основные каналы
// DPI-блокировок (TLS+QUIC) и Discord voice. На каждый элемент — отдельное
// правило `--dport` (xt_multiport не загружен в Keenetic 4.9 ядре).
//
// keenMark передаётся для совместимости с прежним signature, но в
// POSTROUTING-схеме не используется (sing-box свои пакеты помечает 0x53,
// LAN-mark теряется при TPROXY-перехвате до POSTROUTING).
func PolicyDPIRules(keenMark uint32, nfqueueNum int) []RuleSpec {
	queue := fmt.Sprintf("%d", nfqueueNum)
	_ = keenMark // зарезервирован для будущего фильтра по mark

	rules := make([]RuleSpec, 0, len(PolicyDPITCPPorts)+len(PolicyDPIUDPPorts)+1)
	for _, port := range PolicyDPITCPPorts {
		rules = append(rules, RuleSpec{
			Table: "mangle", Chain: PolicyDPIChainName,
			Args: []string{
				"-p", "tcp", "--dport", port,
				"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
			},
		})
	}
	for _, port := range PolicyDPIUDPPorts {
		rules = append(rules, RuleSpec{
			Table: "mangle", Chain: PolicyDPIChainName,
			Args: []string{
				"-p", "udp", "--dport", port,
				"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
			},
		})
	}
	// Переход POSTROUTING → signcraze_policy_dpi (один раз, в конце).
	rules = append(rules, RuleSpec{
		Table: "mangle", Chain: "POSTROUTING",
		Args: []string{"-j", PolicyDPIChainName},
	})
	return rules
}

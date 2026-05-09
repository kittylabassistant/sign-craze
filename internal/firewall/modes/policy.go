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
		// MSS clamping: TUN-интерфейс sing-box имеет MTU 1280 (см.
		// singbox.template.json), а LAN-клиенты согласовывают MSS 1460 (на
		// MTU 1500). Большие сегменты от удалённого сервера (TLS Server Hello
		// + Certificate chain) не пролезают в TUN — пакеты дропаются, клиенту
		// видно "TLS handshake timeout" сразу после Client Hello. Path MTU
		// Discovery через ICMP редко работает на LAN-сегменте Keenetic,
		// поэтому требуется явный TCPMSS clamp на FORWARD к/от TUN.
		// Симметрично -i и -o: clamp применяется к SYN от LAN→TUN и от TUN→LAN.
		{
			Table: "mangle", Chain: "FORWARD",
			Args: []string{
				"-o", PolicyTUNDeviceName,
				"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
				"-j", "TCPMSS", "--clamp-mss-to-pmtu",
			},
		},
		{
			Table: "mangle", Chain: "FORWARD",
			Args: []string{
				"-i", PolicyTUNDeviceName,
				"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
				"-j", "TCPMSS", "--clamp-mss-to-pmtu",
			},
		},
	}
}

// PolicyTUNDeviceName — имя TUN-интерфейса в правилах filter/FORWARD.
// Должно совпадать с firewall.TUNDeviceName / singbox.DefaultTUNInterfaceName.
const PolicyTUNDeviceName = "signbox-tun"

// PolicyTProxyRules возвращает правила iptables для режима ModePolicy +
// REDIRECT inbound. Имя оставлено для совместимости с applier.go, но
// механизм — REDIRECT (DNAT в local stack), потому что xt_TPROXY отсутствует
// в kernel Keenetic 4.9.
//
// Пакеты Keenetic-policy → REDIRECT → 127.0.0.1:tproxyPort → sing-box
// `redirect` inbound принимает соединение, через SO_ORIGINAL_DST извлекает
// оригинальный dst клиента. Src LAN-клиента сохраняется (DNAT меняет dst).
//
// REDIRECT работает только для TCP. UDP трафик идёт напрямую (sing-box
// `redirect` inbound TCP-only). Для UDP-проксирования нужен другой механизм
// (TUN или TPROXY — последний недоступен на Keenetic 4.9).
//
// Правила в `nat:PREROUTING` (DNAT — не mangle).
func PolicyTProxyRules(keenMark, loopMark uint32, tproxyPort uint16) []RuleSpec {
	if keenMark == 0 {
		return nil
	}
	keen := fmt.Sprintf("0x%x", keenMark)
	port := fmt.Sprintf("%d", tproxyPort)
	_ = loopMark // REDIRECT не использует mark; зарезервирован для loop-prevention

	return []RuleSpec{
		// TCP REDIRECT: DNAT keenetic-policy TCP в local 127.0.0.1:port.
		{
			Table: "nat", Chain: PolicyChainName,
			Args: []string{
				"-m", "mark", "--mark", keen,
				"-p", "tcp",
				"-j", "REDIRECT", "--to-port", port,
			},
		},
		// Переход nat:PREROUTING → signcraze_policy.
		{
			Table: "nat", Chain: "PREROUTING",
			Args: []string{"-j", PolicyChainName},
		},
	}
}

// PolicyTProxyRulesReal — настоящие TPROXY правила (mangle + xt_TPROXY).
// Загрузка модулей xt_socket+xt_TPROXY обеспечивается applier.EnsureKernelModule.
// TCP+UDP оба покрыты, оригинальный src/dst клиента сохраняется через TPROXY.
//
// Требует EnsureLocalRoute (ip route local 0.0.0.0/0 dev lo table N) для
// доставки TPROXY-пакетов в сокет sing-box.
func PolicyTProxyRulesReal(keenMark, loopMark uint32, tproxyPort uint16) []RuleSpec {
	if keenMark == 0 {
		return nil
	}
	keen := fmt.Sprintf("0x%x", keenMark)
	loop := fmt.Sprintf("0x%x", loopMark)
	port := fmt.Sprintf("%d", tproxyPort)

	return []RuleSpec{
		// TCP TPROXY
		{
			Table: "mangle", Chain: PolicyChainName,
			Args: []string{
				"-m", "mark", "--mark", keen,
				"-p", "tcp",
				"-j", "TPROXY", "--on-port", port, "--on-ip", "127.0.0.1",
				"--tproxy-mark", loop + "/0xffffffff",
			},
		},
		// UDP TPROXY
		{
			Table: "mangle", Chain: PolicyChainName,
			Args: []string{
				"-m", "mark", "--mark", keen,
				"-p", "udp",
				"-j", "TPROXY", "--on-port", port, "--on-ip", "127.0.0.1",
				"--tproxy-mark", loop + "/0xffffffff",
			},
		},
		// PREROUTING jump
		{
			Table: "mangle", Chain: "PREROUTING",
			Args: []string{"-j", PolicyChainName},
		},
	}
}

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
// wanIface — Linux-имя WAN (eth3, ppp0). Если задан, jump-правило в
// POSTROUTING ограничивается `-o <wanIface>` — это исключает loopback,
// внутренние интерфейсы и трафик-на-TUN от попадания в NFQUEUE (важно
// для производительности на slow MIPS: без фильтра nfqws2 десинхронизирует
// и пакеты к VPN-серверу, что бесполезно). Пустая строка → fallback на
// безусловный jump (back-compat).
//
// vpnExcludeIPs — список IP-адресов VPN-эндпоинтов, которые не должны
// проходить DPI-десинхронизацию. Reality VPN маскируется под TLS реального
// хоста, и nfqws2-desync на ClientHello к VPN-серверу либо сломает
// маскировку, либо бесполезен. Каждый IP вставляется как RETURN-правило
// в начало signcraze_policy_dpi.
//
// keenMark передаётся для совместимости с прежним signature, но в
// POSTROUTING-схеме не используется (sing-box свои пакеты помечает 0x53,
// LAN-mark теряется при TPROXY-перехвате до POSTROUTING).
func PolicyDPIRules(keenMark uint32, nfqueueNum int, wanIface string, vpnExcludeIPs []string) []RuleSpec {
	queue := fmt.Sprintf("%d", nfqueueNum)
	_ = keenMark // зарезервирован для будущего фильтра по mark

	rules := make([]RuleSpec, 0, len(vpnExcludeIPs)+len(PolicyDPITCPPorts)+len(PolicyDPIUDPPorts)+1)
	// Exclude VPN-эндпоинты: трафик sing-box к Reality-серверу должен идти
	// без модификаций (десинхронизация ClientHello ломает маскировку).
	for _, ip := range vpnExcludeIPs {
		if ip == "" {
			continue
		}
		rules = append(rules, RuleSpec{
			Table: "mangle", Chain: PolicyDPIChainName,
			Args: []string{"-d", ip, "-j", "RETURN"},
		})
	}
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
	jumpArgs := []string{"-j", PolicyDPIChainName}
	if wanIface != "" {
		jumpArgs = append([]string{"-o", wanIface}, jumpArgs...)
	}
	rules = append(rules, RuleSpec{
		Table: "mangle", Chain: "POSTROUTING",
		Args: jumpArgs,
	})
	return rules
}

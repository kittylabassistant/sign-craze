package firewall

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall/modes"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/netif"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// Applier управляет полным жизненным циклом брандмауэра: применение и откат правил.
type Applier interface {
	// Apply применяет правила iptables/ipset/ip rule для заданного режима.
	// НЕ устанавливает TUN-route — он требует уже запущенный sing-box
	// (TUN-интерфейс существует только когда sing-box активен). После старта
	// sing-box вызывайте AttachTUN.
	// При ошибке на любом шаге автоматически вызывает Remove для отката.
	Apply(ctx context.Context, mode types.Mode) error

	// AttachTUN ждёт появления TUN-интерфейса dev и устанавливает default-route
	// в нашу таблицу маршрутизации. Вызывается из CLI (--start/--restart) после
	// успешного старта ядра. Для xray и mihomo (TProxy mode) AttachTUN пропускается
	// через SkipTUNCheck. Идемпотентна (ip route replace).
	AttachTUN(ctx context.Context, dev string) error

	// Remove удаляет все правила sign-craze (включая TUN-route). Идемпотентно.
	Remove(ctx context.Context) error

	// Reconcile повторно применяет все правила без pre-flights и без
	// auto-rollback. Используется watchdog'ом для восстановления правил после
	// внешнего вмешательства (Keenetic ndm reconciliation стирает наши
	// FORWARD ACCEPT правила через несколько часов после `--start`).
	//
	// Все шаги используют EnsureRule/EnsureChain (idempotent), поэтому
	// повторный вызов на корректно настроенной системе — no-op.
	// AttachTUN не вызывается (TUN route ставится отдельно после старта sing-box).
	Reconcile(ctx context.Context, mode types.Mode) error
}

// Config содержит параметры брандмауэра. Все значения из BEHAVIOR_SPEC §3.
type Config struct {
	FWMark     uint32         // 0x53 (83) — собственный mark sign-craze (loop-prevention)
	Table      int            // 83
	Priority   int            // 32765
	Port       uint16         // 7895
	NFQueueNum int            // 300 (совпадает с upstream nfqws2-keenetic)
	Ports      []uint16       // дополнительные порты для маркировки в signcraze_ports
	Excludes   []netip.Prefix // CIDR-исключения (RETURN из signcraze)
	AdminPorts []uint16       // SSH/web admin порты для bypass (пусто = выкл)

	// PolicyMark — fwmark, присвоенный Keenetic'ом IP-policy через RCI.
	// Используется только в режиме ModePolicy. Читается через ndm.GetPolicy()
	// и передаётся в Config перед каждым Apply (mark может смениться при reboot).
	PolicyMark uint32
	// DPIEnabled управляет добавлением NFQUEUE-правил в режиме ModePolicy.
	// В режиме ModeFull DPI-правила являются частью HybridRules.
	DPIEnabled bool

	// VPNExcludeIPs — IP-адреса VPN-эндпоинтов, исключаемые из NFQUEUE-десинка.
	// Резолвится из state.Outbounds[].Server при build firewall config.
	// В PolicyDPIRules вставляется как RETURN-правила перед NFQUEUE-портами,
	// чтобы не ломать Reality-маскировку sing-box к собственному VPN-серверу.
	VPNExcludeIPs []string

	// WANIface — имя WAN-интерфейса в Linux (eth0, ppp0 и т.п. — НЕ Keenetic-имя).
	// Apply добавляет INPUT DROP TCP/UDP для всех UI-портов (9090, 9091, 9092)
	// с WAN_IF — единственный сетевой барьер от внешнего доступа к Web UI,
	// который намеренно работает без auth/TLS в LAN-trust-модели.
	// Remove удаляет эти правила.
	// Пустая строка → ApplyInternal возвращает ошибку (fail-secure).
	WANIface string

	// SkipTUNCheck отключает pre-flight CheckTUNAvailable. Используется
	// для xray/mihomo, которые работают через TProxy + fwmark и не создают
	// TUN-устройство. Sing-box оставляет SkipTUNCheck=false (default).
	SkipTUNCheck bool

	// UseTProxy переключает режим sing-box с TUN-inbound на TPROXY-inbound.
	// Когда true:
	//   - pre-flight CheckTUNAvailable пропускается;
	//   - applyPolicyMode пробует загрузить xt_TPROXY (real TPROXY) и при успехе
	//     использует PolicyTProxyRulesReal + EnsureLocalRoute;
	//   - при недоступности xt_TPROXY — fallback на PolicyTProxyRules (REDIRECT, TCP-only);
	//   - AttachTUN становится no-op;
	//   - Remove вызывает DeleteLocalRoute вместо DeleteTUNRoute.
	UseTProxy bool

	// TProxyKernelOK заполняется applier'ом при Apply в режиме UseTProxy=true.
	// true — xt_TPROXY загружен, используется реальный TPROXY (TCP+UDP).
	// false — fallback на REDIRECT (TCP-only).
	// Читается из config render для выбора inbound type sing-box.
	TProxyKernelOK bool

	// TUNDeviceName — переопределение имени TUN-интерфейса. Пусто → default
	// (TUNDeviceName const, "signbox-tun"). Заполняется когда активное ядро
	// требует отличное имя TUN: xray/mihomo сейчас работают через TProxy и
	// TUN не создают; поле зарезервировано на случай будущей поддержки
	// xray-TUN/mihomo-TUN с собственным именем интерфейса.
	TUNDeviceNameOverride string
}

// TUNDevice возвращает имя TUN-интерфейса для текущей конфигурации.
// Fallback на TUNDeviceName const, если override не задан.
func (c Config) TUNDevice() string {
	if c.TUNDeviceNameOverride != "" {
		return c.TUNDeviceNameOverride
	}
	return TUNDeviceName
}

// IPSetExcludes — имя ipset для bypass-исключений.
const IPSetExcludes = "signcraze_excludes"

// TUNDeviceName — имя TUN-интерфейса, создаваемого активным ядром (sing-box или
// mihomo в TUN-режиме). Должно совпадать с singbox.DefaultTUNInterfaceName
// (декларируется здесь для использования в firewall-слое без зависимости от пакета singbox).
const TUNDeviceName = "signbox-tun"

// TUNAttachTimeout — потолок ожидания TUN-интерфейса. На slow MIPS softfloat
// sing-box cold-start может занимать 12-20s в обычной нагрузке. После cold
// reboot Keenetic (cache не прогрет, NDM поднимает интерфейсы параллельно,
// kernel ещё подгружает netfilter-модули) реальное время до создания
// signbox-tun наблюдается до 60s — наблюдалось 30s+ на reboot v0.5.13
// 2026-05-07 → "интерфейс signbox-tun не появился за 30s". 90s — запас
// для cold boot, на warm-start 0.6-2s — fast-path сработает мгновенно.
const TUNAttachTimeout = 90 * time.Second

// DefaultConfig возвращает конфигурацию по умолчанию согласно BEHAVIOR_SPEC §3.
func DefaultConfig() Config {
	return Config{
		FWMark:     0x53,
		Table:      83,
		Priority:   32765,
		Port:       7895,
		NFQueueNum: 300,
	}
}

// NewApplier создаёт Applier с заданным runner и конфигурацией.
func NewApplier(runner exectx.Runner, cfg Config) Applier {
	return &applierImpl{
		runner: runner,
		cfg:    cfg,
		ipt:    New(runner),
		ipset:  NewIPSet(runner),
	}
}

type applierImpl struct {
	runner exectx.Runner
	cfg    Config
	ipt    *IPTables
	ipset  *IPSet
}

// Apply применяет правила для заданного режима. При ошибке откатывается через Remove.
func (a *applierImpl) Apply(ctx context.Context, mode types.Mode) error {
	log.L().Info("firewall: применение правил", "mode", mode)

	// Pre-flight: kernel TUN — без него sing-box `tun` inbound не запустится.
	// Для xray/mihomo (TProxy mode) и sing-box TPROXY-mode проверка пропускается.
	if !a.cfg.SkipTUNCheck && !a.cfg.UseTProxy {
		if err := CheckTUNAvailable(); err != nil {
			return fmt.Errorf("firewall: pre-flight: %w", err)
		}
	}

	// Pre-flight: проверить наличие match `set` в iptables (для ipset-маршрутизации
	// в режимах full/hybrid). Также подсветит отсутствие libxt_set.so на busybox.
	if err := CheckRequiredIptablesModules(ctx, a.runner); err != nil {
		return fmt.Errorf("firewall: pre-flight: %w", err)
	}

	// Pre-flight: убедиться что fwmark не занят другим инструментом.
	if err := CheckFWMarkAvailable(ctx, a.runner, a.cfg.FWMark, a.cfg.Table); err != nil {
		return fmt.Errorf("firewall: pre-flight: %w", err)
	}

	// Отключить Keenetic FASTNAT/FASTROUTE: иначе RTCACHE кэширует первую пару
	// пакетов flow и обходит mangle для последующих ACK/PSH, отправляя их в
	// default WAN мимо signbox-tun. На не-Keenetic-системах no-op.
	if err := DisableFastPath(); err != nil {
		log.L().Warn("firewall: не удалось отключить FastPath, трафик может частично идти мимо TUN", "err", err)
	}

	if err := a.applyInternal(ctx, mode); err != nil {
		log.L().Warn("firewall: ошибка применения, откат", "err", err)
		if removeErr := a.Remove(ctx); removeErr != nil {
			return fmt.Errorf("firewall: ошибка применения: %w; ошибка отката: %w", err, removeErr)
		}
		return err
	}
	log.L().Info("firewall: правила применены", "mode", mode)
	return nil
}

func (a *applierImpl) applyInternal(ctx context.Context, mode types.Mode) error {
	// WAN автодетект — нужен ДО applyPolicyMode для PolicyDPIRules `-o $WAN`-фильтра,
	// чтобы NFQUEUE не ловил трафик-к-TUN/loopback. ensureWANUIDrop ниже использует
	// уже заполненный a.cfg.WANIface.
	if a.cfg.WANIface == "" {
		iface, err := netif.DetectWANIface(ctx, a.runner)
		if err != nil {
			return fmt.Errorf("firewall: WANIface не задан и автодетект не удался: %w", err)
		}
		a.cfg.WANIface = iface
		log.L().Info("firewall: WAN автодетект", "iface", iface)
	}
	switch mode {
	case types.ModePolicy:
		if err := a.applyPolicyMode(ctx); err != nil {
			return err
		}
	case types.ModeFull:
		if err := a.applyFullMode(ctx); err != nil {
			return err
		}
	default:
		return fmt.Errorf("firewall: неподдерживаемый режим %q", mode)
	}
	// Защита Web UI (9090 Zashboard, 9091 admin REST, 9092 routing UI) от
	// доступа через WAN: DROP на входящем WAN-интерфейсе. UI работает в
	// LAN-trust-модели без auth/TLS, WAN-доступность недопустима.
	if err := a.ensureWANUIDrop(ctx); err != nil {
		return err
	}
	return nil
}

// uiPorts — порты Web UI, защищаемые INPUT DROP на WAN.
// 9090 — Zashboard (Clash-API), 9091 — admin REST, 9092 — routing editor.
var uiPorts = []string{"9090", "9091", "9092"}

// ensureWANUIDrop добавляет INPUT DROP TCP/UDP на UI-порты для WAN-интерфейса.
// applyInternal заполняет a.cfg.WANIface через автодетект ДО вызова этой функции,
// поэтому здесь ожидается уже непустой WANIface. Дополнительный fallback оставлен
// на случай прямого вызова из Reconcile или тестов.
func (a *applierImpl) ensureWANUIDrop(ctx context.Context) error {
	if a.cfg.WANIface == "" {
		iface, err := netif.DetectWANIface(ctx, a.runner)
		if err != nil {
			return fmt.Errorf("firewall: WANIface не задан и автодетект не удался: %w", err)
		}
		a.cfg.WANIface = iface
		log.L().Info("firewall: WAN автодетект", "iface", iface)
	}
	for _, port := range uiPorts {
		for _, proto := range []string{"tcp", "udp"} {
			if err := a.ipt.EnsureRule(ctx, "filter", "INPUT",
				"-i", a.cfg.WANIface, "-p", proto, "--dport", port, "-j", "DROP",
			); err != nil {
				return fmt.Errorf("firewall: WAN UI DROP (port %s/%s): %w", port, proto, err)
			}
		}
	}
	return nil
}

// deleteWANUIDrop удаляет INPUT DROP TCP/UDP на UI-порты для WAN-интерфейса.
// Идемпотентно: пустой WANIface — no-op.
func (a *applierImpl) deleteWANUIDrop(ctx context.Context) {
	if a.cfg.WANIface == "" {
		return
	}
	for _, port := range uiPorts {
		for _, proto := range []string{"tcp", "udp"} {
			if err := a.ipt.DeleteRule(ctx, "filter", "INPUT",
				"-i", a.cfg.WANIface, "-p", proto, "--dport", port, "-j", "DROP",
			); err != nil {
				log.L().Warn("firewall: ошибка удаления WAN UI DROP", "port", port, "proto", proto, "err", err)
			}
		}
	}
}

// applyPolicyMode — режим интеграции с Keenetic IP Policy.
//
// Не создаёт ipset, не использует собственный fwmark для маркировки трафика.
// Keenetic сам помечает пакеты устройств своим mark и создаёт ip rule.
// Sign-craze добавляет TPROXY-правила с фильтром по этому mark и собственное
// ip rule для loop-prevention исходящих от активного ядра (SO_MARK=0x53).
func (a *applierImpl) applyPolicyMode(ctx context.Context) error {
	if a.cfg.PolicyMark == 0 {
		return fmt.Errorf("firewall: ModePolicy требует PolicyMark != 0 (читать через ndm.GetPolicy)")
	}

	// 1. ip rule fwmark → table. Default-route в TUN установится позже через
	// AttachTUN (после старта sing-box, когда TUN-интерфейс появится).
	if err := EnsureIPRule(ctx, a.runner, a.cfg.FWMark, a.cfg.Table, a.cfg.Priority); err != nil {
		return err
	}

	// 2. DPI-цепочка signcraze_policy_dpi всегда в mangle (NFQUEUE для nfqws2).
	if a.cfg.DPIEnabled {
		if err := a.ipt.EnsureChain(ctx, "mangle", modes.PolicyDPIChainName); err != nil {
			return err
		}
	}

	// 3. DPI-правила (если включено) — добавляются ПЕРЕД policy-правилами,
	// чтобы NFQUEUE сработал до перемаркировки и подъёма в TUN-таблицу.
	// WAN-фильтр (`-o $WAN`) исключает трафик-в-TUN/loopback от NFQUEUE.
	// VPNExcludeIPs защищает Reality-маскировку sing-box к собственному
	// VPN-серверу: десинк на этих IP не применяется (RETURN перед NFQUEUE).
	if a.cfg.DPIEnabled {
		dpiRules := modes.PolicyDPIRules(a.cfg.PolicyMark, a.cfg.NFQueueNum, a.cfg.WANIface, a.cfg.VPNExcludeIPs)
		for _, spec := range dpiRules {
			if err := a.ipt.EnsureRule(ctx, spec.Table, spec.Chain, spec.Args...); err != nil {
				return err
			}
		}
	}

	// 4. Правила маркировки / перенаправления трафика Keenetic-policy.
	if a.cfg.UseTProxy {
		// Probe xt_socket + xt_TPROXY: пытаемся загрузить kernel-модули.
		// Если оба загружены — используем реальный TPROXY (TCP+UDP).
		// Иначе — fallback на REDIRECT (TCP-only).
		socketOK := EnsureKernelModule(ctx, a.runner, "xt_socket", KernelModulePathXtSocket) == nil
		tproxyOK := false
		if socketOK {
			tproxyOK = EnsureKernelModule(ctx, a.runner, "xt_TPROXY", KernelModulePathXtTProxy) == nil
		}
		a.cfg.TProxyKernelOK = tproxyOK

		if tproxyOK {
			log.L().Info("firewall: TPROXY режим (xt_TPROXY загружен)")
			// Реальный TPROXY: цепочка в mangle, сохраняет оригинальный src/dst.
			if err := a.ipt.EnsureChain(ctx, "mangle", modes.PolicyChainName); err != nil {
				return err
			}
			for _, spec := range modes.PolicyTProxyRulesReal(a.cfg.PolicyMark, a.cfg.FWMark, a.cfg.Port) {
				if err := a.ipt.EnsureRule(ctx, spec.Table, spec.Chain, spec.Args...); err != nil {
					return err
				}
			}
			if err := EnsureLocalRoute(ctx, a.runner, a.cfg.Table); err != nil {
				return err
			}
		} else {
			log.L().Warn("firewall: xt_TPROXY недоступен, fallback на REDIRECT (TCP-only)")
			// REDIRECT-mode: цепочка в nat, DNAT в local stack.
			if err := a.ipt.EnsureChain(ctx, "nat", modes.PolicyChainName); err != nil {
				return err
			}
			for _, spec := range modes.PolicyTProxyRules(a.cfg.PolicyMark, a.cfg.FWMark, a.cfg.Port) {
				if err := a.ipt.EnsureRule(ctx, spec.Table, spec.Chain, spec.Args...); err != nil {
					return err
				}
			}
		}
		// route_localnet необходим как для REDIRECT (martian-пакеты) так и для
		// TPROXY (local socket delivery через loopback).
		if err := enableRouteLocalnet(); err != nil {
			log.L().Warn("firewall: route_localnet enable", "err", err)
		}
		// INPUT ACCEPT для sing-box listening port: NDM filter INPUT
		// policy=DROP, все non-ESTABLISHED packets без явного ACCEPT
		// дропаются. TCP и UDP (для TPROXY UDP-режима).
		for _, proto := range []string{"tcp", "udp"} {
			if err := a.ipt.EnsureRule(ctx, "filter", "INPUT",
				"-p", proto, "--dport", fmt.Sprintf("%d", a.cfg.Port),
				"-j", "ACCEPT",
			); err != nil {
				return err
			}
		}
	} else {
		// TUN-mode (legacy): цепочка в mangle, помечаем трафик fwmark
		// для подъёма в таблицу через TUN.
		if err := a.ipt.EnsureChain(ctx, "mangle", modes.PolicyChainName); err != nil {
			return err
		}
		for _, spec := range modes.PolicyRules(a.cfg.PolicyMark, a.cfg.FWMark) {
			if err := a.ipt.EnsureRule(ctx, spec.Table, spec.Chain, spec.Args...); err != nil {
				return err
			}
		}
	}

	return nil
}

// applyFullMode — legacy-режим: ipset/fwmark/signcraze chains (бывший hybrid).
func (a *applierImpl) applyFullMode(ctx context.Context) error {
	// 1. Создать ipset-наборы (включая signcraze_excludes).
	if err := a.ipset.EnsureSet(ctx, string(types.IPSetIPv4), "hash:net", "inet"); err != nil {
		return err
	}
	if err := a.ipset.EnsureSet(ctx, string(types.IPSetIPv6), "hash:net", "inet6"); err != nil {
		return err
	}
	if err := a.ipset.EnsureSet(ctx, IPSetExcludes, "hash:net", "inet"); err != nil {
		return err
	}

	// Заполнить signcraze_excludes из cfg.Excludes (atomic replace).
	if len(a.cfg.Excludes) > 0 {
		if err := a.ipset.AtomicReplace(ctx, IPSetExcludes, "hash:net", "inet", a.cfg.Excludes); err != nil {
			return fmt.Errorf("firewall: заполнение excludes: %w", err)
		}
	}

	// 2. ip rule fwmark → table. Default-route в TUN установится через AttachTUN.
	if err := EnsureIPRule(ctx, a.runner, a.cfg.FWMark, a.cfg.Table, a.cfg.Priority); err != nil {
		return err
	}

	// 3. Создать цепочки в mangle (signcraze_ports добавлена для port-based маркировки).
	for _, chain := range []string{"signcraze", "signcraze_full", "signcraze_dpi", "signcraze_ports"} {
		if err := a.ipt.EnsureChain(ctx, "mangle", chain); err != nil {
			return err
		}
	}

	// 4. RETURN-bypass правила должны идти ПЕРЕД mark-правилами в цепочке.
	for _, spec := range modes.AdminPortsBypassRules(a.cfg.AdminPorts) {
		if err := a.ipt.InsertRule(ctx, spec.Table, spec.Chain, 1, spec.Args...); err != nil {
			return err
		}
	}
	if len(a.cfg.Excludes) > 0 {
		for _, spec := range modes.ExcludeRules() {
			if err := a.ipt.InsertRule(ctx, spec.Table, spec.Chain, 1, spec.Args...); err != nil {
				return err
			}
		}
	}

	// 5. Правила режима: MARK по ipset + опц. NFQUEUE (DPIEnabled).
	var ruleSpecs []modes.RuleSpec
	if a.cfg.DPIEnabled {
		ruleSpecs = modes.HybridRules(a.cfg.FWMark, a.cfg.NFQueueNum)
	} else {
		ruleSpecs = modes.TProxyRules(a.cfg.FWMark)
	}
	for _, spec := range ruleSpecs {
		if err := a.ipt.EnsureRule(ctx, spec.Table, spec.Chain, spec.Args...); err != nil {
			return err
		}
	}

	// 6. Port-based маркировка (signcraze_ports).
	for _, spec := range modes.PortRules(a.cfg.Ports, a.cfg.FWMark) {
		if err := a.ipt.EnsureRule(ctx, spec.Table, spec.Chain, spec.Args...); err != nil {
			return err
		}
	}

	return nil
}

// Reconcile вызывает applyInternal без pre-flights и без auto-rollback.
// EnsureRule/EnsureChain делают восстановление идемпотентным. Используется
// watchdog'ом для борьбы с ndm reconciliation, которая стирает FORWARD ACCEPT
// для signbox-tun через несколько часов после `--start`.
func (a *applierImpl) Reconcile(ctx context.Context, mode types.Mode) error {
	return a.applyInternal(ctx, mode)
}

// AttachTUN ждёт появления TUN-интерфейса (созданного sing-box) и устанавливает
// default-route в нашу таблицу маршрутизации. Должна вызываться из CLI после
// успешного старта sing-box.
// В режиме TPROXY (UseTProxy=true) — no-op: sing-box не создаёт TUN-интерфейс,
// маршрутизация построена на local route + TPROXY правилах.
func (a *applierImpl) AttachTUN(ctx context.Context, dev string) error {
	if a.cfg.UseTProxy {
		log.L().Debug("firewall: AttachTUN пропущен (TPROXY-режим)")
		return nil
	}
	if err := WaitForInterface(ctx, a.runner, dev, TUNAttachTimeout); err != nil {
		return fmt.Errorf("firewall: ожидание TUN-интерфейса: %w", err)
	}
	// Поднять link явно: WaitForInterface ловит netdev по имени даже когда
	// он ещё в state DOWN (race на slow MIPS между TUNSETIFF и IFF_UP).
	// Без этого следующий EnsureTUNRoute падает с "Network is down".
	if err := EnsureLinkUp(ctx, a.runner, dev); err != nil {
		return err
	}
	if err := EnsureTUNRoute(ctx, a.runner, dev, a.cfg.Table); err != nil {
		return err
	}
	log.L().Info("firewall: TUN подключён", "dev", dev, "table", a.cfg.Table)
	return nil
}

// Remove удаляет все правила sign-craze. Идемпотентно — безопасно вызывать на чистом состоянии.
func (a *applierImpl) Remove(ctx context.Context) error {
	log.L().Info("firewall: удаление правил")

	// 1. Удалить filter/FORWARD ACCEPT правила (policy mode). DeleteRule
	// идемпотентна (-C → -D), безопасна на чистом состоянии.
	for _, args := range [][]string{
		{"-o", a.cfg.TUNDevice(), "-j", "ACCEPT"},
		{"-i", a.cfg.TUNDevice(), "-j", "ACCEPT"},
	} {
		if err := a.ipt.DeleteRule(ctx, "filter", "FORWARD", args...); err != nil {
			log.L().Warn("firewall: ошибка удаления filter/FORWARD ACCEPT", "args", args, "err", err)
		}
	}

	// 2. Удалить prerouting jumps на наши user-chains. На стоковой
	// Keenetic-прошивке нет xt_comment → раньше использовался
	// DeleteRulesByComment теперь не сработает (правила пишутся без
	// --comment). Все наши правила в системной PREROUTING — это
	// jumps на signcraze_*; перечислены явно.
	preroutingJumps := []string{
		modes.PolicyChainName, // signcraze_policy
		"signcraze_dpi",
		"signcraze_ports",
		"signcraze_full",
		"signcraze",
	}
	for _, target := range preroutingJumps {
		if err := a.ipt.DeleteJumpAll(ctx, "mangle", "PREROUTING", target); err != nil {
			log.L().Warn("firewall: ошибка удаления PREROUTING jump", "target", target, "err", err)
		}
	}

	// signcraze_policy в REDIRECT-режиме висит в nat:PREROUTING (DNAT).
	// Удаляем jump из nat независимо от UseTProxy — idempotent.
	if err := a.ipt.DeleteJumpAll(ctx, "nat", "PREROUTING", modes.PolicyChainName); err != nil {
		log.L().Warn("firewall: ошибка удаления nat PREROUTING jump", "target", modes.PolicyChainName, "err", err)
	}

	// signcraze_policy_dpi висит в POSTROUTING (см. PolicyDPIRules комментарий).
	// Cleanup отдельно — иначе DeleteChain ниже падает на «chain referenced».
	if err := a.ipt.DeleteJumpAll(ctx, "mangle", "POSTROUTING", modes.PolicyDPIChainName); err != nil {
		log.L().Warn("firewall: ошибка удаления POSTROUTING jump", "target", modes.PolicyDPIChainName, "err", err)
	}

	// 2. Удалить наши user-chains (flush очищает их содержимое — все наши
	// правила, поскольку в системные цепочки мы кроме jumps ничего не
	// клали).
	allChains := []string{
		modes.PolicyDPIChainName,
		modes.PolicyChainName,
		"signcraze_dpi",
		"signcraze_ports",
		"signcraze_full",
		"signcraze",
	}
	for _, chain := range allChains {
		if err := a.ipt.FlushAndDeleteChain(ctx, "mangle", chain); err != nil {
			log.L().Warn("firewall: ошибка удаления цепочки", "chain", chain, "err", err)
		}
	}
	// signcraze_policy в nat (REDIRECT-режим) — удаляем отдельно.
	if err := a.ipt.FlushAndDeleteChain(ctx, "nat", modes.PolicyChainName); err != nil {
		log.L().Warn("firewall: ошибка удаления nat цепочки", "chain", modes.PolicyChainName, "err", err)
	}

	// 3. Удалить route и ip rule. В TPROXY-режиме удаляем local route вместо TUN route.
	if a.cfg.UseTProxy {
		if err := DeleteLocalRoute(ctx, a.runner, a.cfg.Table); err != nil {
			log.L().Warn("firewall: ошибка удаления local route", "err", err)
		}
	} else {
		if err := DeleteTUNRoute(ctx, a.runner, a.cfg.TUNDevice(), a.cfg.Table); err != nil {
			log.L().Warn("firewall: ошибка удаления TUN route", "err", err)
		}
	}
	if err := DeleteIPRule(ctx, a.runner, a.cfg.FWMark, a.cfg.Table); err != nil {
		log.L().Warn("firewall: ошибка удаления ip rule", "err", err)
	}

	// 4. Удалить ipset-наборы
	if err := a.ipset.DestroySet(ctx, string(types.IPSetIPv4)); err != nil {
		log.L().Warn("firewall: ошибка удаления ipset", "name", types.IPSetIPv4, "err", err)
	}
	if err := a.ipset.DestroySet(ctx, string(types.IPSetIPv6)); err != nil {
		log.L().Warn("firewall: ошибка удаления ipset", "name", types.IPSetIPv6, "err", err)
	}
	if err := a.ipset.DestroySet(ctx, IPSetExcludes); err != nil {
		log.L().Warn("firewall: ошибка удаления ipset", "name", IPSetExcludes, "err", err)
	}

	// 5. Удалить WAN DROP для UI-портов (9090/9091/9092).
	a.deleteWANUIDrop(ctx)

	// Восстановить исходные значения Keenetic FASTNAT/FASTROUTE.
	if err := RestoreFastPath(); err != nil {
		log.L().Warn("firewall: восстановление FastPath", "err", err)
	}

	log.L().Info("firewall: правила удалены")
	return nil
}

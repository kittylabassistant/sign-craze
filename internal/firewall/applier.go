package firewall

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall/modes"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// Applier управляет полным жизненным циклом брандмауэра: применение и откат правил.
type Applier interface {
	// Apply применяет правила iptables/ipset/ip для заданного режима.
	// При ошибке на любом шаге автоматически вызывает Remove для отката.
	Apply(ctx context.Context, mode types.Mode) error

	// Remove удаляет все правила sign-craze. Идемпотентно.
	Remove(ctx context.Context) error
}

// Config содержит параметры брандмауэра. Все значения из BEHAVIOR_SPEC §3.
type Config struct {
	FWMark     uint32         // 0x53 (83)
	Table      int            // 83
	Priority   int            // 32765
	Port       uint16         // 7895
	NFQueueNum int            // 200
	Ports      []uint16       // дополнительные порты для маркировки в signcraze_ports
	Excludes   []netip.Prefix // CIDR-исключения (RETURN из signcraze)
	AdminPort  uint16         // SSH/web admin port для bypass (0 = выкл)
}

// IPSetExcludes — имя ipset для bypass-исключений.
const IPSetExcludes = "signcraze_excludes"

// DefaultConfig возвращает конфигурацию по умолчанию согласно BEHAVIOR_SPEC §3.
func DefaultConfig() Config {
	return Config{
		FWMark:     0x53,
		Table:      83,
		Priority:   32765,
		Port:       7895,
		NFQueueNum: 200,
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

	// Pre-flight: убедиться что fwmark не занят другим инструментом.
	if err := CheckFWMarkAvailable(ctx, a.runner, a.cfg.FWMark, a.cfg.Table); err != nil {
		return fmt.Errorf("firewall: pre-flight: %w", err)
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

	// 2. ip rule + ip route (маршрутизация помеченного трафика).
	if err := EnsureIPRule(ctx, a.runner, a.cfg.FWMark, a.cfg.Table, a.cfg.Priority); err != nil {
		return err
	}
	if err := EnsureLocalRoute(ctx, a.runner, a.cfg.Table); err != nil {
		return err
	}

	// 3. Создать цепочки в mangle (signcraze_ports добавлена для port-based маркировки).
	for _, chain := range []string{"signcraze", "signcraze_full", "signcraze_dpi", "signcraze_ports"} {
		if err := a.ipt.EnsureChain(ctx, "mangle", chain); err != nil {
			return err
		}
	}

	// 4. RETURN-bypass правила должны идти ПЕРЕД mark-правилами в цепочке.
	// Используем InsertRule (-I 1), чтобы порядок гарантировался независимо от
	// предыдущего состояния цепочки (safety-fixes #1).
	// Admin port bypass добавляется первым (если включён) — самые приоритетные.
	if a.cfg.AdminPort > 0 {
		for _, spec := range modes.AdminPortBypassRule(a.cfg.AdminPort) {
			if err := a.ipt.InsertRule(ctx, spec.Table, spec.Chain, 1, spec.Args...); err != nil {
				return err
			}
		}
	}
	if len(a.cfg.Excludes) > 0 {
		for _, spec := range modes.ExcludeRules() {
			if err := a.ipt.InsertRule(ctx, spec.Table, spec.Chain, 1, spec.Args...); err != nil {
				return err
			}
		}
	}

	// 5. Правила режима (TProxy / Hybrid).
	var ruleSpecs []modes.RuleSpec
	switch mode {
	case types.ModeProxy:
		ruleSpecs = modes.TProxyRules(a.cfg.Port, a.cfg.FWMark)
	case types.ModeHybrid:
		ruleSpecs = modes.HybridRules(a.cfg.Port, a.cfg.FWMark, a.cfg.NFQueueNum)
	default:
		return fmt.Errorf("firewall: неподдерживаемый режим %q", mode)
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

// Remove удаляет все правила sign-craze. Идемпотентно — безопасно вызывать на чистом состоянии.
func (a *applierImpl) Remove(ctx context.Context) error {
	log.L().Info("firewall: удаление правил")

	// 1. Удалить правила с комментариями signcraze: из таблицы mangle
	if err := a.ipt.DeleteRulesByComment(ctx, "mangle", "signcraze:"); err != nil {
		log.L().Warn("firewall: ошибка при удалении правил по комментарию", "err", err)
	}

	// 2. Удалить цепочки (включая signcraze_ports).
	for _, chain := range []string{"signcraze_dpi", "signcraze_ports", "signcraze_full", "signcraze"} {
		if err := a.ipt.FlushAndDeleteChain(ctx, "mangle", chain); err != nil {
			log.L().Warn("firewall: ошибка удаления цепочки", "chain", chain, "err", err)
		}
	}

	// 3. Удалить ip rule и ip route
	if err := DeleteIPRule(ctx, a.runner, a.cfg.FWMark, a.cfg.Table); err != nil {
		log.L().Warn("firewall: ошибка удаления ip rule", "err", err)
	}
	if err := DeleteLocalRoute(ctx, a.runner, a.cfg.Table); err != nil {
		log.L().Warn("firewall: ошибка удаления local route", "err", err)
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

	log.L().Info("firewall: правила удалены")
	return nil
}

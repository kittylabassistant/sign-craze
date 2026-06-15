package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/locks"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/netif"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// reconcileFirewall — коллбэк для firewall.Watchdog.
//
// Шаги:
//  1. Load state (возможно state.PolicyMark обновлён CLI/web между тиками).
//  2. Если ModePolicy — ensureKeeneticPolicy: обновляет mark в state, если
//     Keenetic пересоздал policy (типичный триггер — WAN reconnect, ndmc save).
//  3. Pre-check: если все критичные правила на месте — fast-path return nil.
//     Pre-check дешёвый (3-4 вызова iptables -C), Reconcile полный — 15-30
//     вызовов. На быстрых роутерах разница незаметна, на KN-1410 — заметна.
//  4. Если правила пропали — создаём applier (DNS-резолв только здесь) и
//     Reconcile (idempotent re-apply без auto-rollback).
func reconcileFirewall(ctx context.Context) error {
	// Skip-and-return-nil при занятом flock: CLI команда (--stop, --restart,
	// --update-geo и т.п.) держит lock в этот момент, watchdog не должен
	// конкурировать с ней — повторное Reconcile при занятом lock пересоздаст
	// правила, которые удаляет --stop. Следующий тик watchdog'а попробует
	// снова. ErrLocked не фатален.
	lk, err := locks.TryAcquire(locks.DefaultPath)
	if err != nil {
		if errors.Is(err, locks.ErrLocked) {
			log.L().Debug("watchdog: flock занят CLI, пропуск тика")
			return nil
		}
		return fmt.Errorf("watchdog: захват flock: %w", err)
	}
	defer func() {
		if relErr := lk.Release(); relErr != nil {
			log.L().Warn("watchdog: ошибка снятия flock", "err", relErr)
		}
	}()

	st, err := loadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	// Mode policy требует свежего mark от Keenetic — он может смениться при
	// WAN reconnect или manual policy edit в Keenetic web UI.
	if st.Mode == types.ModePolicy {
		if pErr := ensureKeeneticPolicy(ctx, st); pErr != nil {
			// Не фатально: продолжаем со старым mark из state. Если mark
			// действительно сменился, следующая попытка попадёт в эту ветку
			// снова, и в какой-то момент RCI ответит.
			log.L().Warn("watchdog: ensureKeeneticPolicy failed", "err", pErr)
		}
	}

	// Pre-check критичных правил: быстрая ветка идемпотентности.
	ipt := firewall.New(exectx.OS)
	critCfg := &firewall.CriticalRulesConfig{
		UseTProxy:  st.Inbound == "tproxy",
		DPIEnabled: st.DPIEnabled,
		FWMark:     0x53,
	}
	// TProxyKernelOK: best-effort probe без side-effects.
	if critCfg.UseTProxy {
		critCfg.TProxyKernelOK = firewall.EnsureKernelModule(
			ctx, exectx.OS, "xt_TPROXY", firewall.KernelModulePathXtTProxy) == nil
	}
	// WANIface: best-effort, при ошибке check работает без -o фильтра.
	if critCfg.DPIEnabled {
		if iface, wanErr := netif.DetectWANIface(ctx, exectx.OS); wanErr == nil {
			critCfg.WANIface = iface
		}
	}
	missing := ipt.CheckCriticalRules(ctx, st.Mode, st.PolicyMark, critCfg)
	if len(missing) == 0 {
		return nil
	}

	// DNS-резолв (collectVPNExcludeIPs внутри) — только если правила пропали.
	applier, err := newFirewallApplier(st)
	if err != nil {
		return fmt.Errorf("applier: %w", err)
	}

	log.L().Warn("watchdog: критичные правила пропали, реапплай",
		"missing", missing, "mode", st.Mode)

	if err := applier.Reconcile(ctx, st.Mode); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}

	log.L().Info("watchdog: правила восстановлены", "count", len(missing))
	return nil
}

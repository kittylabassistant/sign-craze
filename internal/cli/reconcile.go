package cli

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// reconcileFirewall — коллбэк для firewall.Watchdog.
//
// Шаги:
//  1. Load state (возможно state.PolicyMark обновлён CLI/web между тиками).
//  2. Если ModePolicy — ensureKeeneticPolicy: обновляет mark в state, если
//     Keenetic пересоздал policy (типичный триггер — WAN reconnect, ndmc save).
//  3. Pre-check: если все критичные правила на месте — ничего не делаем.
//     Pre-check дешёвый (3-4 вызова iptables -C), Reconcile полный — 15-30
//     вызовов. На быстрых роутерах разница незаметна, на KN-1410 — заметна.
//  4. Если правила пропали — Reconcile (idempotent re-apply без auto-rollback).
func reconcileFirewall(ctx context.Context) error {
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

	applier, err := newFirewallApplier(st)
	if err != nil {
		return fmt.Errorf("applier: %w", err)
	}

	// Pre-check критичных правил: быстрая ветка идемпотентности.
	ipt := firewall.New(newRunner())
	missing := ipt.CheckCriticalRules(ctx, st.Mode, st.PolicyMark)
	if len(missing) == 0 {
		return nil
	}

	log.L().Warn("watchdog: критичные правила пропали, реапплай",
		"missing", missing, "mode", st.Mode)

	if err := applier.Reconcile(ctx, st.Mode); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}

	log.L().Info("watchdog: правила восстановлены", "count", len(missing))
	return nil
}

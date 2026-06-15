package firewall

import (
	"context"
	"fmt"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/firewall/modes"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// DefaultWatchdogInterval — период проверки/реапплая правил по умолчанию.
//
// 30 секунд — компромисс между скоростью реакции на ndm reconciliation
// (Keenetic стирает FORWARD ACCEPT для signbox-tun через несколько часов
// после --start) и нагрузкой на медленные MIPS роутеры (каждый цикл — 4-8
// вызовов iptables, ~50ms суммарно на KN-1410).
const DefaultWatchdogInterval = 30 * time.Second

// ReconcileFunc — пользовательский колбэк реапплая. Вызывается каждый тик.
// Должен быть идемпотентным: на корректно настроенной системе — no-op.
//
// Типичная реализация:
//  1. Load state.
//  2. Если ModePolicy — ensureKeeneticPolicy (обновляет mark в state если
//     Keenetic пересоздал policy после WAN reconnect).
//  3. applier.Reconcile(ctx, mode).
type ReconcileFunc func(ctx context.Context) error

// Watchdog периодически вызывает Reconcile для восстановления правил
// после внешнего вмешательства (ndm reconciliation, manual flush).
//
// Не использует goroutine sync — caller отвечает за управление lifecycle
// через контекст.
type Watchdog struct {
	interval  time.Duration
	reconcile ReconcileFunc
}

// NewWatchdog создаёт Watchdog с заданным интервалом и колбэком.
// interval <= 0 → DefaultWatchdogInterval.
func NewWatchdog(interval time.Duration, reconcile ReconcileFunc) *Watchdog {
	if interval <= 0 {
		interval = DefaultWatchdogInterval
	}
	return &Watchdog{interval: interval, reconcile: reconcile}
}

// Run блокируется и периодически вызывает reconcile до отмены контекста.
// Первый тик — через interval (не сразу): startUI только что вызвал Apply.
//
// Ошибки reconcile логируются как WARN и не прерывают цикл — watchdog должен
// продолжать пытаться даже при временных сбоях (sing-box рестарт, network down).
func (w *Watchdog) Run(ctx context.Context) {
	if w.reconcile == nil {
		log.L().Warn("watchdog: reconcile callback не задан, watchdog не активен")
		return
	}
	t := time.NewTicker(w.interval)
	defer t.Stop()

	log.L().Info("firewall watchdog запущен", "interval", w.interval)
	for {
		select {
		case <-ctx.Done():
			log.L().Info("firewall watchdog остановлен")
			return
		case <-t.C:
			if err := w.reconcile(ctx); err != nil {
				log.L().Warn("firewall watchdog: reconcile failed", "err", err)
			}
		}
	}
}

// CriticalRulesConfig содержит параметры для CheckCriticalRules.
// Позволяет проверять дополнительные правила в зависимости от конфигурации.
type CriticalRulesConfig struct {
	// UseTProxy=false → режим REDIRECT (nat PREROUTING); =true → mangle PREROUTING.
	UseTProxy bool
	// TProxyKernelOK=true → реальный TPROXY (mangle); =false → REDIRECT (nat).
	TProxyKernelOK bool
	// DPIEnabled=true → проверять mangle/FORWARD -j signcraze_dpi_fwd.
	DPIEnabled bool
	// WANIface — Linux-имя WAN для фильтра -o в DPI jump-правиле.
	// Если пустое — jump проверяется без -o фильтра (back-compat).
	WANIface string
	// FWMark — собственный mark sign-craze (0x53) для DPI jump фильтра ! --mark.
	FWMark uint32
}

// CheckCriticalRules проверяет наличие критичных правил в текущей таблице
// iptables. Возвращает список отсутствующих описаний для diagnostics/logs.
//
// Critical = те правила, отсутствие которых ломает прокси:
//  1. filter/FORWARD -o signbox-tun -j ACCEPT — без него Keenetic FORWARD
//     policy DROP уничтожает все пакеты к TUN.
//  2. filter/FORWARD -i signbox-tun -j ACCEPT — обратное направление.
//  3. mangle/PREROUTING -j signcraze_policy (или signcraze_full) — без jump
//     наша цепочка не вызывается, mark не выставляется.
//     Для ModePolicy+REDIRECT: nat/PREROUTING -j signcraze_policy.
//  4. (DPIEnabled=true) mangle/FORWARD -j signcraze_dpi_fwd — без jump DPI
//     не работает для LAN-устройств без VPN-policy.
//
// keenMark > 0 → также проверить mangle/signcraze_policy MARK rule.
// Если все правила на месте — возвращается nil/пустой slice.
//
// Параметр cfg опционален (nil → использовать defaults: TProxy=false,
// DPI=false). Метод сохраняет обратную совместимость для тестов без cfg.
func (t *IPTables) CheckCriticalRules(ctx context.Context, mode types.Mode, keenMark uint32, cfg ...*CriticalRulesConfig) []string {
	var ccfg CriticalRulesConfig
	if len(cfg) > 0 && cfg[0] != nil {
		ccfg = *cfg[0]
	}

	var missing []string

	check := func(table, chain string, args ...string) bool {
		fullArgs := append([]string{"-t", table, "-C", chain}, args...)
		_, err := t.runner.Run(ctx, "iptables", fullArgs...)
		return err == nil
	}

	if !check("filter", "FORWARD", "-o", TUNDeviceName, "-j", "ACCEPT") {
		missing = append(missing, "FORWARD -o signbox-tun ACCEPT")
	}
	if !check("filter", "FORWARD", "-i", TUNDeviceName, "-j", "ACCEPT") {
		missing = append(missing, "FORWARD -i signbox-tun ACCEPT")
	}

	switch mode {
	case types.ModePolicy:
		// ModePolicy+UseTProxy=true → реальный TPROXY (mangle) или REDIRECT (nat).
		// ModePolicy+UseTProxy=false → TUN-mode через mangle mark.
		if ccfg.UseTProxy && !ccfg.TProxyKernelOK {
			// REDIRECT-mode: jump в nat PREROUTING.
			if !check("nat", "PREROUTING", "-j", modes.PolicyChainName) {
				missing = append(missing, "nat/PREROUTING -j signcraze_policy (REDIRECT)")
			}
		} else {
			// TUN-mode или реальный TPROXY: jump в mangle PREROUTING.
			if !check("mangle", "PREROUTING", "-j", modes.PolicyChainName) {
				missing = append(missing, "mangle/PREROUTING -j signcraze_policy")
			}
		}
		// DPI forward chain: обязателен когда DPIEnabled=true.
		if ccfg.DPIEnabled {
			var dpiArgs []string
			if ccfg.WANIface != "" {
				dpiArgs = append(dpiArgs, "-o", ccfg.WANIface)
			}
			mark := fmt.Sprintf("0x%x", ccfg.FWMark)
			dpiArgs = append(dpiArgs, "-m", "mark", "!", "--mark", mark, "-j", modes.DPIForwardChainName)
			if !check("mangle", "FORWARD", dpiArgs...) {
				missing = append(missing, "mangle/FORWARD -j signcraze_dpi_fwd")
			}
		}
	case types.ModeFull:
		if !check("mangle", "PREROUTING", "-j", "signcraze") {
			missing = append(missing, "PREROUTING -j signcraze")
		}
	}

	// keenMark>0 → проверить наличие MARK-правила в mangle/signcraze_policy.
	// Форма правила соответствует PolicyRules() в modes/policy.go.
	if keenMark > 0 && mode == types.ModePolicy {
		keen := fmt.Sprintf("0x%x", keenMark)
		loop := fmt.Sprintf("0x%x", ccfg.FWMark)
		if !check("mangle", modes.PolicyChainName,
			"-m", "mark", "--mark", keen,
			"-j", "MARK", "--set-mark", loop,
		) {
			missing = append(missing, fmt.Sprintf("mangle/signcraze_policy MARK rule (keenMark=%s)", keen))
		}
	}

	return missing
}

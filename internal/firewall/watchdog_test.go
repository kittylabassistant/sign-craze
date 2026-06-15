package firewall

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestWatchdog_ВызываетReconcileПоТику(t *testing.T) {
	var calls atomic.Int32
	w := NewWatchdog(20*time.Millisecond, func(_ context.Context) error {
		calls.Add(1)
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	got := calls.Load()
	if got < 3 || got > 6 {
		t.Errorf("ожидалось 3-6 вызовов за 100ms (interval 20ms), получено %d", got)
	}
}

func TestWatchdog_ОшибкиReconcileНеПрерываютЦикл(t *testing.T) {
	var calls atomic.Int32
	w := NewWatchdog(15*time.Millisecond, func(_ context.Context) error {
		calls.Add(1)
		return errors.New("симулированная ошибка")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	got := calls.Load()
	if got < 3 {
		t.Errorf("ожидалось >=3 вызовов даже при ошибках, получено %d", got)
	}
}

func TestWatchdog_NilCallbackВыходитСразу(t *testing.T) {
	w := NewWatchdog(10*time.Millisecond, nil)
	done := make(chan struct{})
	go func() {
		w.Run(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Run должен вернуться сразу при nil callback")
	}
}

func TestWatchdog_ОстанавливаетсяПоCtxCancel(t *testing.T) {
	w := NewWatchdog(10*time.Millisecond, func(_ context.Context) error {
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	time.Sleep(25 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Run не остановился после cancel")
	}
}

func TestNewWatchdog_ZeroIntervalUseDefault(t *testing.T) {
	w := NewWatchdog(0, func(_ context.Context) error { return nil })
	if w.interval != DefaultWatchdogInterval {
		t.Errorf("interval = %v, ожидался DefaultWatchdogInterval %v", w.interval, DefaultWatchdogInterval)
	}
}

func TestNewWatchdog_NegativeIntervalUseDefault(t *testing.T) {
	w := NewWatchdog(-1*time.Second, func(_ context.Context) error { return nil })
	if w.interval != DefaultWatchdogInterval {
		t.Errorf("interval = %v, ожидался default", w.interval)
	}
}

// --- Task C: CheckCriticalRules ---

// ruleSet — mock runner, который матчит ключи по "iptables -t TABLE -C CHAIN ARGS".
// Правила из set возвращают успех, всё остальное — ошибку (правило отсутствует).
type ruleSet struct {
	present map[string]bool
}

func (r *ruleSet) Run(_ context.Context, name string, args ...string) (exectx.Result, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	if r.present[key] {
		return exectx.Result{ExitCode: 0}, nil
	}
	return exectx.Result{ExitCode: 1}, errors.New("no rule")
}

// tunRules возвращает базовый набор ключей для filter/FORWARD ACCEPT на TUN.
func tunRules() map[string]bool {
	return map[string]bool{
		"iptables -t filter -C FORWARD -o signbox-tun -j ACCEPT": true,
		"iptables -t filter -C FORWARD -i signbox-tun -j ACCEPT": true,
	}
}

// TestCheckCriticalRules_Policy_TProxy_OK — mangle PREROUTING -j signcraze_policy есть,
// TUN FORWARD ACCEPT есть → пусто.
func TestCheckCriticalRules_Policy_TProxy_OK(t *testing.T) {
	rules := tunRules()
	rules["iptables -t mangle -C PREROUTING -j signcraze_policy"] = true
	ipt := New(&ruleSet{rules})
	missing := ipt.CheckCriticalRules(context.Background(), types.ModePolicy, 0)
	if len(missing) != 0 {
		t.Errorf("ожидалось 0 missing, получено: %v", missing)
	}
}

// TestCheckCriticalRules_Policy_TProxy_Missing — mangle PREROUTING отсутствует.
func TestCheckCriticalRules_Policy_TProxy_Missing(t *testing.T) {
	ipt := New(&ruleSet{tunRules()})
	missing := ipt.CheckCriticalRules(context.Background(), types.ModePolicy, 0)
	found := false
	for _, m := range missing {
		if m == "mangle/PREROUTING -j signcraze_policy" {
			found = true
		}
	}
	if !found {
		t.Errorf("ожидался 'mangle/PREROUTING -j signcraze_policy' в missing, получено: %v", missing)
	}
}

// TestCheckCriticalRules_Policy_Redirect_OK — REDIRECT-mode (UseTProxy+!TProxyKernelOK),
// nat PREROUTING -j signcraze_policy есть.
func TestCheckCriticalRules_Policy_Redirect_OK(t *testing.T) {
	rules := tunRules()
	rules["iptables -t nat -C PREROUTING -j signcraze_policy"] = true
	ipt := New(&ruleSet{rules})
	cfg := &CriticalRulesConfig{UseTProxy: true, TProxyKernelOK: false}
	missing := ipt.CheckCriticalRules(context.Background(), types.ModePolicy, 0, cfg)
	if len(missing) != 0 {
		t.Errorf("ожидалось 0 missing, получено: %v", missing)
	}
}

// TestCheckCriticalRules_Policy_Redirect_Missing — REDIRECT-mode, nat/PREROUTING отсутствует.
func TestCheckCriticalRules_Policy_Redirect_Missing(t *testing.T) {
	ipt := New(&ruleSet{tunRules()})
	cfg := &CriticalRulesConfig{UseTProxy: true, TProxyKernelOK: false}
	missing := ipt.CheckCriticalRules(context.Background(), types.ModePolicy, 0, cfg)
	found := false
	for _, m := range missing {
		if m == "nat/PREROUTING -j signcraze_policy (REDIRECT)" {
			found = true
		}
	}
	if !found {
		t.Errorf("ожидался 'nat/PREROUTING -j signcraze_policy (REDIRECT)' в missing, получено: %v", missing)
	}
}

// TestCheckCriticalRules_Policy_DPI_OK — DPIEnabled=true, WANIface=eth3, fwmark=0x53,
// jump в mangle FORWARD присутствует.
func TestCheckCriticalRules_Policy_DPI_OK(t *testing.T) {
	rules := tunRules()
	rules["iptables -t mangle -C PREROUTING -j signcraze_policy"] = true
	rules["iptables -t mangle -C FORWARD -o eth3 -m mark ! --mark 0x53 -j signcraze_dpi_fwd"] = true
	ipt := New(&ruleSet{rules})
	cfg := &CriticalRulesConfig{DPIEnabled: true, WANIface: "eth3", FWMark: 0x53}
	missing := ipt.CheckCriticalRules(context.Background(), types.ModePolicy, 0, cfg)
	if len(missing) != 0 {
		t.Errorf("ожидалось 0 missing при DPI OK, получено: %v", missing)
	}
}

// TestCheckCriticalRules_Policy_DPI_Missing — DPIEnabled=true, DPI jump отсутствует.
func TestCheckCriticalRules_Policy_DPI_Missing(t *testing.T) {
	rules := tunRules()
	rules["iptables -t mangle -C PREROUTING -j signcraze_policy"] = true
	ipt := New(&ruleSet{rules})
	cfg := &CriticalRulesConfig{DPIEnabled: true, WANIface: "eth3", FWMark: 0x53}
	missing := ipt.CheckCriticalRules(context.Background(), types.ModePolicy, 0, cfg)
	found := false
	for _, m := range missing {
		if m == "mangle/FORWARD -j signcraze_dpi_fwd" {
			found = true
		}
	}
	if !found {
		t.Errorf("ожидался 'mangle/FORWARD -j signcraze_dpi_fwd' в missing, получено: %v", missing)
	}
}

// TestCheckCriticalRules_Full_OK — ModeFull, mangle PREROUTING -j signcraze есть.
func TestCheckCriticalRules_Full_OK(t *testing.T) {
	rules := tunRules()
	rules["iptables -t mangle -C PREROUTING -j signcraze"] = true
	ipt := New(&ruleSet{rules})
	missing := ipt.CheckCriticalRules(context.Background(), types.ModeFull, 0)
	if len(missing) != 0 {
		t.Errorf("ожидалось 0 missing, получено: %v", missing)
	}
}

// TestCheckCriticalRules_KeenMark_Zero — keenMark=0, проверка MARK не выполняется.
func TestCheckCriticalRules_KeenMark_Zero(t *testing.T) {
	rules := tunRules()
	rules["iptables -t mangle -C PREROUTING -j signcraze_policy"] = true
	ipt := New(&ruleSet{rules})
	missing := ipt.CheckCriticalRules(context.Background(), types.ModePolicy, 0)
	if len(missing) != 0 {
		t.Errorf("keenMark=0: ожидалось 0 missing, получено: %v", missing)
	}
}

// TestCheckCriticalRules_KeenMark_Present — keenMark>0, MARK-правило есть → missing пустой.
func TestCheckCriticalRules_KeenMark_Present(t *testing.T) {
	rules := tunRules()
	rules["iptables -t mangle -C PREROUTING -j signcraze_policy"] = true
	rules["iptables -t mangle -C signcraze_policy -m mark --mark 0xaabb -j MARK --set-mark 0x53"] = true
	ipt := New(&ruleSet{rules})
	cfg := &CriticalRulesConfig{FWMark: 0x53}
	missing := ipt.CheckCriticalRules(context.Background(), types.ModePolicy, 0xaabb, cfg)
	if len(missing) != 0 {
		t.Errorf("keenMark present: ожидалось 0 missing, получено: %v", missing)
	}
}

// TestCheckCriticalRules_KeenMark_Missing — keenMark>0, MARK-правило отсутствует → в missing.
func TestCheckCriticalRules_KeenMark_Missing(t *testing.T) {
	rules := tunRules()
	rules["iptables -t mangle -C PREROUTING -j signcraze_policy"] = true
	// MARK-правило не добавляем
	ipt := New(&ruleSet{rules})
	cfg := &CriticalRulesConfig{FWMark: 0x53}
	missing := ipt.CheckCriticalRules(context.Background(), types.ModePolicy, 0xaabb, cfg)
	found := false
	for _, m := range missing {
		if strings.Contains(m, "MARK rule") {
			found = true
		}
	}
	if !found {
		t.Errorf("keenMark missing: ожидался 'MARK rule' в missing, получено: %v", missing)
	}
}

package firewall

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// autoRunner записывает вызовы и возвращает разумные ответы по умолчанию.
// -C → ошибка (правило не найдено), ipset list → ошибка (нет набора),
// ip rule show / ip route show → пустой вывод, всё остальное → успех.
type autoRunner struct {
	calls []string
}

func (r *autoRunner) Run(_ context.Context, name string, args ...string) (exectx.Result, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.calls = append(r.calls, key)

	switch {
	// iptables -C → правило не найдено
	case name == "iptables" && len(args) >= 3 && args[2] == "-C":
		return exectx.Result{ExitCode: 1, Stderr: []byte("Bad rule")}, fmt.Errorf("не найдено")
	// ipset list → набор не существует
	case name == "ipset" && len(args) >= 1 && args[0] == "list":
		return exectx.Result{ExitCode: 1, Stderr: []byte("The set with the given name does not exist")}, fmt.Errorf("не существует")
	// ip rule show → пусто (правил нет)
	case name == "ip" && len(args) >= 2 && args[0] == "rule" && args[1] == "show":
		return exectx.Result{ExitCode: 0, Stdout: []byte("")}, nil
	// ip route show → пусто (маршрутов нет)
	case name == "ip" && len(args) >= 2 && args[0] == "route" && args[1] == "show":
		return exectx.Result{ExitCode: 0, Stdout: []byte("")}, nil
	// iptables -t table -S → пусто (правил нет)
	case name == "iptables" && len(args) >= 3 && args[2] == "-S" && len(args) == 3:
		return exectx.Result{ExitCode: 0, Stdout: []byte("")}, nil
	// ip rule show для DeleteRulesByComment: также пусто
	default:
		return exectx.Result{ExitCode: 0}, nil
	}
}

func (r *autoRunner) hasCall(prefix string) bool {
	for _, c := range r.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func TestApplier_Apply_Proxy_НетОшибок(t *testing.T) {
	r := &autoRunner{}
	a := NewApplier(r, DefaultConfig())
	if err := a.Apply(context.Background(), types.ModeProxy); err != nil {
		t.Fatalf("Apply(proxy) вернул ошибку: %v", err)
	}
	// ipsets созданы
	if !r.hasCall("ipset create signcraze_ipv4") {
		t.Error("ipset signcraze_ipv4 не был создан")
	}
	if !r.hasCall("ipset create signcraze_ipv6") {
		t.Error("ipset signcraze_ipv6 не был создан")
	}
	// ip rule добавлен
	if !r.hasCall("ip rule add fwmark") {
		t.Error("ip rule add не вызван")
	}
	// цепочки созданы
	if !r.hasCall("iptables -t mangle -N signcraze") {
		t.Error("цепочка signcraze не создана")
	}
	if !r.hasCall("iptables -t mangle -N signcraze_dpi") {
		t.Error("цепочка signcraze_dpi не создана")
	}
}

func TestApplier_Apply_Hybrid_ДобавляетNFQUEUE(t *testing.T) {
	r := &autoRunner{}
	a := NewApplier(r, DefaultConfig())
	if err := a.Apply(context.Background(), types.ModeHybrid); err != nil {
		t.Fatalf("Apply(hybrid) вернул ошибку: %v", err)
	}
	// NFQUEUE-правило добавлено
	if !r.hasCall("iptables -t mangle -A signcraze_dpi") {
		t.Error("правило NFQUEUE в signcraze_dpi не добавлено")
	}
}

func TestApplier_Remove_Идемпотентен(t *testing.T) {
	// Remove на чистом состоянии не должен возвращать ошибку
	r := exectx.Mock(map[string]exectx.Result{
		// ip rule show: пусто
		"ip rule show": {ExitCode: 0, Stdout: []byte("")},
		// ip route show: пусто
		"ip route show table 83": {ExitCode: 0, Stdout: []byte("")},
		// iptables -S: пусто
		"iptables -t mangle -S": {ExitCode: 0, Stdout: []byte("")},
		// FlushAndDeleteChain: цепочек нет → -F возвращает ошибку → идемпотентно
		"iptables -t mangle -F signcraze":      {ExitCode: 1, Stderr: []byte("No chain")},
		"iptables -t mangle -F signcraze_full": {ExitCode: 1, Stderr: []byte("No chain")},
		"iptables -t mangle -F signcraze_dpi":  {ExitCode: 1, Stderr: []byte("No chain")},
		// ipset destroy: наборов нет → ошибка → идемпотентно
		"ipset destroy signcraze_ipv4": {ExitCode: 1, Stderr: []byte("The set with the given name does not exist")},
		"ipset destroy signcraze_ipv6": {ExitCode: 1, Stderr: []byte("The set with the given name does not exist")},
	})
	a := NewApplier(r, DefaultConfig())
	if err := a.Remove(context.Background()); err != nil {
		t.Fatalf("Remove на чистом состоянии вернул ошибку: %v", err)
	}
}

func TestApplier_DefaultConfig_ЗначенияИзСпецификации(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.FWMark != 0x53 {
		t.Errorf("FWMark = 0x%x, ожидался 0x53", cfg.FWMark)
	}
	if cfg.Table != 83 {
		t.Errorf("Table = %d, ожидался 83", cfg.Table)
	}
	if cfg.Priority != 32765 {
		t.Errorf("Priority = %d, ожидался 32765", cfg.Priority)
	}
	if cfg.Port != 7895 {
		t.Errorf("Port = %d, ожидался 7895", cfg.Port)
	}
	if cfg.NFQueueNum != 200 {
		t.Errorf("NFQueueNum = %d, ожидался 200", cfg.NFQueueNum)
	}
}

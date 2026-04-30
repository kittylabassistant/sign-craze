package firewall

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// init отключает реальную проверку /dev/net/tun на CI/Docker, где TUN-устройство
// может отсутствовать. Тесты Apply покрывают логику, не системную доступность TUN.
func init() {
	tunAvailableCheck = func() error { return nil }
}

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

func TestApplier_Apply_Full_НетОшибок(t *testing.T) {
	r := &autoRunner{}
	a := NewApplier(r, DefaultConfig())
	if err := a.Apply(context.Background(), types.ModeFull); err != nil {
		t.Fatalf("Apply(full) вернул ошибку: %v", err)
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

func TestApplier_Apply_Full_ДобавляетNFQUEUE(t *testing.T) {
	r := &autoRunner{}
	cfg := DefaultConfig()
	cfg.DPIEnabled = true
	a := NewApplier(r, cfg)
	if err := a.Apply(context.Background(), types.ModeFull); err != nil {
		t.Fatalf("Apply(full) с DPI вернул ошибку: %v", err)
	}
	// NFQUEUE-правило добавлено
	if !r.hasCall("iptables -t mangle -A signcraze_dpi") {
		t.Error("правило NFQUEUE в signcraze_dpi не добавлено")
	}
}

func TestApplier_Apply_Policy_СоздаётСвоюЦепочку(t *testing.T) {
	r := &autoRunner{}
	cfg := DefaultConfig()
	cfg.PolicyMark = 0xffffaab // имитация значения, присвоенного Keenetic
	a := NewApplier(r, cfg)
	if err := a.Apply(context.Background(), types.ModePolicy); err != nil {
		t.Fatalf("Apply(policy) вернул ошибку: %v", err)
	}
	// Цепочка signcraze_policy создана.
	if !r.hasCall("iptables -t mangle -N signcraze_policy") {
		t.Error("цепочка signcraze_policy не создана")
	}
	// ipset НЕ создаётся в policy-режиме.
	if r.hasCall("ipset create signcraze_ipv4") {
		t.Error("ipset signcraze_ipv4 не должен создаваться в режиме policy")
	}
	// signcraze chain (legacy) НЕ создаётся.
	if r.hasCall("iptables -t mangle -N signcraze ") || r.hasCall("iptables -t mangle -N signcraze\n") {
		t.Error("цепочка signcraze (legacy) не должна создаваться в режиме policy")
	}
}

func TestApplier_Apply_Policy_НулевойMarkОшибка(t *testing.T) {
	r := &autoRunner{}
	cfg := DefaultConfig()
	// PolicyMark не задан → должна быть ошибка.
	a := NewApplier(r, cfg)
	if err := a.Apply(context.Background(), types.ModePolicy); err == nil {
		t.Fatal("Apply(policy) с PolicyMark=0 должен вернуть ошибку")
	}
}

func TestApplier_Remove_Идемпотентен(t *testing.T) {
	// Remove на чистом состоянии: все iptables/ipset/ip команды для несуществующих
	// цепочек/наборов/правил возвращают ошибку — Remove должен это игнорировать.
	// Используем autoRunner: -F цепочек → default OK (FlushAndDeleteChain трактует
	// успешный -F как «была цепочка, удаляем»; в реальном busybox -F на отсутствующей
	// цепочке возвращает err, что тоже OK по семантике FlushAndDeleteChain).
	r := &autoRunner{}
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

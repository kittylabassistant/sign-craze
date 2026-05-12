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
//
// override (опционально) перехватывает вызов до дефолтной логики:
// возвращает (Result, true) для матчей, (_, false) для пропуска.
type autoRunner struct {
	calls    []string
	override func(ctx context.Context, name string, args ...string) (exectx.Result, bool)
}

func (r *autoRunner) Run(ctx context.Context, name string, args ...string) (exectx.Result, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.calls = append(r.calls, key)

	if r.override != nil {
		if res, ok := r.override(ctx, name, args...); ok {
			return res, nil
		}
	}

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
	// ip route show default → дефолтный маршрут на eth0 (для автодетекта WAN
	// в ensureWANUIDrop). TestApply_FailsSecure_БезWANIface переопределяет
	// через override, чтобы получить пустой stdout и проверить fail-secure.
	case name == "ip" && len(args) >= 3 && args[0] == "route" && args[1] == "show" && args[2] == "default":
		return exectx.Result{ExitCode: 0, Stdout: []byte("default via 10.0.0.1 dev eth0 proto static\n")}, nil
	// ip route show без default → пусто
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

// TestApplier_Apply_Policy_РазрешаетForwardНаTUN — sign-craze должна добавить
// ACCEPT правила в filter/FORWARD для signbox-tun, иначе на Keenetic с
// FORWARD policy=DROP пакеты с mark 0x53 после ip rule lookup → table 83 →
// signbox-tun дропаются default policy до того как kernel запишет в TUN fd.
func TestApplier_Apply_Policy_РазрешаетForwardНаTUN(t *testing.T) {
	r := &autoRunner{}
	cfg := DefaultConfig()
	cfg.PolicyMark = 0xffffaab
	a := NewApplier(r, cfg)
	if err := a.Apply(context.Background(), types.ModePolicy); err != nil {
		t.Fatalf("Apply(policy) вернул ошибку: %v", err)
	}
	if !r.hasCall("iptables -t filter -A FORWARD -o signbox-tun -j ACCEPT") {
		t.Error("filter/FORWARD ACCEPT для -o signbox-tun не добавлен")
	}
	if !r.hasCall("iptables -t filter -A FORWARD -i signbox-tun -j ACCEPT") {
		t.Error("filter/FORWARD ACCEPT для -i signbox-tun не добавлен")
	}
}

// TestApplier_Apply_Policy_DPI_ChainInForward — refactor 2026-05-12: DPI-цепочка
// signcraze_dpi_fwd создаётся в mangle FORWARD (не POSTROUTING). Применяется ко
// всем LAN-устройствам, идущим через WAN напрямую (не через TPROXY).
func TestApplier_Apply_Policy_DPI_ChainInForward(t *testing.T) {
	r := &autoRunner{}
	cfg := DefaultConfig()
	cfg.PolicyMark = 0xffffaab
	cfg.DPIEnabled = true
	cfg.WANIface = "eth3"
	a := NewApplier(r, cfg)
	if err := a.Apply(context.Background(), types.ModePolicy); err != nil {
		t.Fatalf("Apply(policy, DPI=on) вернул ошибку: %v", err)
	}
	// Новая цепочка создана.
	if !r.hasCall("iptables -t mangle -N signcraze_dpi_fwd") {
		t.Error("цепочка signcraze_dpi_fwd не создана (новая FORWARD-архитектура)")
	}
	// Jump в FORWARD с фильтром по mark.
	if !r.hasCall("iptables -t mangle -A FORWARD -o eth3 -m mark ! --mark 0x53 -j signcraze_dpi_fwd") {
		t.Error("FORWARD jump на signcraze_dpi_fwd с filter `! --mark 0x53` не добавлен")
	}
	// Старая цепочка signcraze_policy_dpi в POSTROUTING НЕ создаётся.
	if r.hasCall("iptables -t mangle -N signcraze_policy_dpi") {
		t.Error("legacy signcraze_policy_dpi не должна создаваться в новой архитектуре")
	}
}

// TestApplier_Remove_CleansLegacyPolicyDPIChain — refactor 2026-05-12: Remove
// должен очистить legacy цепочку signcraze_policy_dpi в POSTROUTING для
// апгрейд-инсталляций (старый бинарь оставил правила, новый бинарь делает Remove).
func TestApplier_Remove_CleansLegacyPolicyDPIChain(t *testing.T) {
	r := &autoRunner{}
	r2 := &seedRunner{
		autoRunner: r,
		existing: map[string]bool{
			"iptables -t mangle -C POSTROUTING -j signcraze_policy_dpi":         true,
			"iptables -t mangle -C POSTROUTING -o eth3 -j signcraze_policy_dpi": true,
			"iptables -t mangle -S POSTROUTING":                                 true,
			"iptables -t mangle -L POSTROUTING --line-numbers -n":               true,
		},
	}
	a := NewApplier(r2, DefaultConfig())
	if err := a.Remove(context.Background()); err != nil {
		t.Fatalf("Remove вернул ошибку: %v", err)
	}
	// Проверяем что попытка удалить jump на legacy chain была сделана
	// (фактическое удаление зависит от DeleteJumpAll внутренностей).
	foundLegacyJumpAttempt := false
	for _, c := range r.calls {
		if strings.Contains(c, "POSTROUTING") && strings.Contains(c, "signcraze_policy_dpi") {
			foundLegacyJumpAttempt = true
			break
		}
	}
	if !foundLegacyJumpAttempt {
		t.Error("Remove не пытался удалить legacy POSTROUTING jump для signcraze_policy_dpi")
	}
	// Проверяем что FlushAndDeleteChain вызывается для legacy цепочки.
	foundLegacyFlush := false
	for _, c := range r.calls {
		if strings.Contains(c, "signcraze_policy_dpi") && (strings.Contains(c, "-F") || strings.Contains(c, "-X")) {
			foundLegacyFlush = true
			break
		}
	}
	if !foundLegacyFlush {
		t.Error("Remove не пытался flush/delete legacy цепочку signcraze_policy_dpi")
	}
}

// TestApplier_Remove_УдаляетForwardНаTUN — Remove должен снять filter/FORWARD
// ACCEPT правила, иначе они остаются после --uninstall.
func TestApplier_Remove_УдаляетForwardНаTUN(t *testing.T) {
	r := &autoRunner{}
	// autoRunner возвращает error на iptables -C → DeleteRule фактически skip'нет.
	// Используем seedRunner, который имитирует наличие правил.
	r2 := &seedRunner{
		autoRunner: r,
		existing: map[string]bool{
			"iptables -t filter -C FORWARD -o signbox-tun -j ACCEPT": true,
			"iptables -t filter -C FORWARD -i signbox-tun -j ACCEPT": true,
		},
	}
	a := NewApplier(r2, DefaultConfig())
	if err := a.Remove(context.Background()); err != nil {
		t.Fatalf("Remove вернул ошибку: %v", err)
	}
	if !r.hasCall("iptables -t filter -D FORWARD -o signbox-tun -j ACCEPT") {
		t.Error("filter/FORWARD -D для -o signbox-tun не вызван")
	}
	if !r.hasCall("iptables -t filter -D FORWARD -i signbox-tun -j ACCEPT") {
		t.Error("filter/FORWARD -D для -i signbox-tun не вызван")
	}
}

// seedRunner — autoRunner с предзаданным набором "существующих" правил.
// Позволяет тестировать DeleteRule (idempotent -C → -D), которая делает -D только
// если -C сообщает что правило есть.
type seedRunner struct {
	*autoRunner
	existing map[string]bool
}

func (s *seedRunner) Run(ctx context.Context, name string, args ...string) (exectx.Result, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	if s.existing[key] {
		s.calls = append(s.calls, key)
		return exectx.Result{ExitCode: 0}, nil
	}
	return s.autoRunner.Run(ctx, name, args...)
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

// TestApply_DropsWANUIPorts — Apply должен добавлять INPUT DROP TCP и UDP
// на все UI-порты (9090/9091/9092) на WAN-интерфейсе. Это единственный
// сетевой барьер от внешнего доступа к Web UI, который работает без auth/TLS
// в LAN-trust-модели.
func TestApply_DropsWANUIPorts(t *testing.T) {
	r := &autoRunner{}
	cfg := DefaultConfig()
	cfg.WANIface = "eth0"
	a := NewApplier(r, cfg)
	if err := a.Apply(context.Background(), types.ModeFull); err != nil {
		t.Fatalf("Apply(full) вернул ошибку: %v", err)
	}
	for _, port := range []string{"9090", "9091", "9092"} {
		for _, proto := range []string{"tcp", "udp"} {
			call := "iptables -t filter -A INPUT -i eth0 -p " + proto + " --dport " + port + " -j DROP"
			if !r.hasCall(call) {
				t.Errorf("INPUT DROP %s %s на WAN не добавлен", proto, port)
			}
		}
	}
}

// TestApply_FailsSecure_БезWANIface — fail-secure: при пустом WANIface
// и невозможности автодетекта Apply возвращает ошибку. Без сетевого
// барьера UI без auth доступен из интернета — нельзя продолжать.
func TestApply_FailsSecure_БезWANIface(t *testing.T) {
	r := &autoRunner{}
	// Override возвращает пустой stdout для "ip route show default" → парсер вернёт error.
	r.override = func(ctx context.Context, name string, args ...string) (exectx.Result, bool) {
		if name == "ip" && len(args) >= 3 && args[0] == "route" && args[1] == "show" && args[2] == "default" {
			return exectx.Result{Stdout: []byte("")}, true
		}
		return exectx.Result{}, false
	}
	cfg := DefaultConfig()
	a := NewApplier(r, cfg)
	err := a.Apply(context.Background(), types.ModeFull)
	if err == nil {
		t.Fatal("ожидалась fail-secure ошибка при пустом WANIface и неудачном автодетекте")
	}
}

// TestApply_АвтодетектWAN — пустой WANIface + рабочий ip route show default
// → автодетект подставляет iface, правила добавляются для него.
func TestApply_АвтодетектWAN(t *testing.T) {
	r := &autoRunner{}
	r.override = func(ctx context.Context, name string, args ...string) (exectx.Result, bool) {
		if name == "ip" && len(args) >= 3 && args[0] == "route" && args[1] == "show" && args[2] == "default" {
			return exectx.Result{Stdout: []byte("default via 1.2.3.4 dev ppp0 proto dhcp\n")}, true
		}
		return exectx.Result{}, false
	}
	cfg := DefaultConfig()
	a := NewApplier(r, cfg)
	if err := a.Apply(context.Background(), types.ModeFull); err != nil {
		t.Fatalf("Apply(full) вернул ошибку: %v", err)
	}
	if !r.hasCall("iptables -t filter -A INPUT -i ppp0 -p tcp --dport 9090 -j DROP") {
		t.Error("INPUT DROP TCP 9090 на автодетектированном WAN (ppp0) не добавлен")
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
	if cfg.NFQueueNum != 300 {
		t.Errorf("NFQueueNum = %d, ожидался 300", cfg.NFQueueNum)
	}
}

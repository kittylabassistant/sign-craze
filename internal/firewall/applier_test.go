package firewall

import (
	"context"
	"fmt"
	"io"
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
// Реализует StdinRunner: RunWithStdin парсит batch-дамп iptables-restore
// и записывает эквивалентные вызовы в r.calls, чтобы тесты проверяли
// содержимое batch так же, как раньше проверяли per-rule вызовы.
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

// RunWithStdin реализует StdinRunner для тестов batch-режима.
// Парсит дамп iptables-restore (формат iptables-save) и записывает
// эквивалентные вызовы iptables в r.calls, сохраняя порядок строк.
//
// Преобразование:
//   - "*table"       → нет вызова (только обновляет текущую таблицу)
//   - ":chain - ..." → "iptables -t TABLE -N chain" (объявление цепочки)
//   - "-F chain"     → "iptables -t TABLE -F chain"
//   - "-A chain ..." → "iptables -t TABLE -A chain ..."
//   - "COMMIT"       → нет вызова
func (r *autoRunner) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) (exectx.Result, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return exectx.Result{ExitCode: 1}, err
	}
	currentTable := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "COMMIT" {
			continue
		}
		if strings.HasPrefix(line, "*") {
			currentTable = line[1:]
			continue
		}
		if strings.HasPrefix(line, ":") {
			// ":chain - [0:0]" → объявление цепочки
			parts := strings.Fields(line[1:])
			if len(parts) > 0 {
				chainName := parts[0]
				key := strings.TrimSpace("iptables -t " + currentTable + " -N " + chainName)
				r.calls = append(r.calls, key)
			}
			continue
		}
		if strings.HasPrefix(line, "-F ") {
			// "-F chainname" → flush
			rest := line[3:]
			key := strings.TrimSpace("iptables -t " + currentTable + " -F " + rest)
			r.calls = append(r.calls, key)
			continue
		}
		if strings.HasPrefix(line, "-A ") || strings.HasPrefix(line, "-I ") {
			key := strings.TrimSpace("iptables -t " + currentTable + " " + line)
			r.calls = append(r.calls, key)
			continue
		}
	}
	return exectx.Result{ExitCode: 0}, nil
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

func (s *seedRunner) RunWithStdin(ctx context.Context, stdin io.Reader, name string, args ...string) (exectx.Result, error) {
	return s.autoRunner.RunWithStdin(ctx, stdin, name, args...)
}

func TestApplier_Apply_Policy_LocalAndAdminBypass(t *testing.T) {
	r := &autoRunner{}
	cfg := DefaultConfig()
	cfg.PolicyMark = 0xffffaab
	cfg.AdminPorts = []uint16{22, 222}
	cfg.LANAddrs = []string{"172.16.0.1"}
	a := NewApplier(r, cfg)
	if err := a.Apply(context.Background(), types.ModePolicy); err != nil {
		t.Fatalf("Apply(policy): %v", err)
	}
	// LOCAL bypass для LAN-IP вставлен через -I в начало signcraze_policy.
	if !r.hasCall("iptables -t mangle -I signcraze_policy 1 -d 172.16.0.1 -j RETURN") {
		t.Error("LOCAL bypass для 172.16.0.1 не вставлен через -I в начало signcraze_policy")
	}
	// Admin-port bypass для порта 222 TCP.
	if !r.hasCall("iptables -t mangle -I signcraze_policy 1 -p tcp --dport 222 -j RETURN") {
		t.Error("admin port 222 TCP bypass не вставлен в signcraze_policy")
	}
	// Admin-port bypass для порта 22 TCP.
	if !r.hasCall("iptables -t mangle -I signcraze_policy 1 -p tcp --dport 22 -j RETURN") {
		t.Error("admin port 22 TCP bypass не вставлен в signcraze_policy")
	}
}

// TestApplier_Apply_Policy_BypassBeforeTProxy — инвариант 2026-05-13:
// LOCAL bypass (-d LAN_IP -j RETURN) и admin-port bypass (--dport N -j RETURN)
// ОБЯЗАНЫ появляться в recorded calls ДО первого TPROXY- или REDIRECT-правила.
// Нарушение: SSH-трафик с пометкой Keenetic-policy попадает в sing-box → SSH виснет.
func TestApplier_Apply_Policy_BypassBeforeTProxy(t *testing.T) {
	r := &autoRunner{}
	cfg := DefaultConfig()
	cfg.PolicyMark = 0xffffaab
	cfg.AdminPorts = []uint16{22, 222, 80, 443}
	cfg.LANAddrs = []string{"172.16.0.1"}
	a := NewApplier(r, cfg)
	if err := a.Apply(context.Background(), types.ModePolicy); err != nil {
		t.Fatalf("Apply(policy): %v", err)
	}

	// Находим индекс первого вызова с -j RETURN (bypass) и
	// первого с -j TPROXY / -j REDIRECT (перенаправление в sing-box).
	firstReturnIdx := -1
	firstTProxyOrRedirectIdx := -1
	for i, call := range r.calls {
		if strings.Contains(call, "-j RETURN") && firstReturnIdx == -1 {
			firstReturnIdx = i
		}
		if (strings.Contains(call, "-j TPROXY") || strings.Contains(call, "-j REDIRECT")) &&
			firstTProxyOrRedirectIdx == -1 {
			firstTProxyOrRedirectIdx = i
		}
	}

	if firstReturnIdx == -1 {
		t.Fatal("ни одного -j RETURN вызова не зафиксировано — bypass не вставлен")
	}
	// Если TPROXY/REDIRECT тоже не появились — режим не добавляет их (TUN-mode),
	// но bypass всё равно должен быть первее всех policy-forward правил.
	if firstTProxyOrRedirectIdx != -1 && firstReturnIdx > firstTProxyOrRedirectIdx {
		t.Errorf(
			"bypass (-j RETURN, idx=%d) вставлен ПОСЛЕ -j TPROXY/REDIRECT (idx=%d): "+
				"SSH-трафик попадёт в sing-box до RETURN-правила (инцидент 2026-05-13)",
			firstReturnIdx, firstTProxyOrRedirectIdx,
		)
	}

	// Дополнительно: обе категории bypass присутствуют.
	hasLANBypass := false
	hasAdminBypass := false
	for _, call := range r.calls {
		if strings.Contains(call, "-d 172.16.0.1") && strings.Contains(call, "-j RETURN") {
			hasLANBypass = true
		}
		if strings.Contains(call, "--dport 22") && strings.Contains(call, "-j RETURN") {
			hasAdminBypass = true
		}
	}
	if !hasLANBypass {
		t.Error("LOCAL bypass -d 172.16.0.1 -j RETURN отсутствует")
	}
	if !hasAdminBypass {
		t.Error("admin-port bypass --dport 22 -j RETURN отсутствует")
	}
}

// TestApply_AllPolicyVariants_HaveDPIPrecleanupAndBypass — regression-фиксация
// поведения ДО рефакторинга R7 (дедупликация триады pre-cleanup DPI-jump /
// DPIForwardRules-в-batch / LAN+admin bypass, общей для applyPolicyTUNMode,
// applyPolicyTProxyReal и applyPolicyRedirect). Для каждой из 3 policy-веток
// проверяет:
//
//	(а) DeleteJumpAll(mangle, FORWARD, signcraze_dpi_fwd) вызван ДО того как
//	    jump на signcraze_dpi_fwd снова появляется в FORWARD (в составе batch);
//	(б) DPIForwardRules реально попали в batch: цепочка signcraze_dpi_fwd
//	    объявлена и хотя бы одно NFQUEUE-правило присутствует;
//	(в) LOCAL bypass и admin-ports bypass вставлены через `-I signcraze_policy 1`
//	    в ожидаемой таблице (mangle для TUN-mode/TProxyReal, nat для Redirect).
//
// applyPolicyTProxyReal недостижима через публичный Apply(): выбор этой ветки
// в applyPolicyMode зависит от EnsureKernelModule(xt_socket/xt_TPROXY), которая
// делает os.Stat на реальный Keenetic-путь (/lib/modules/4.9-ndm-5/...) — вне
// роутера всегда возвращает ошибку, и applyPolicyMode детерминированно уходит
// в Redirect-fallback. Поэтому тест вызывает все 3 приватных метода напрямую
// (white-box, тот же package firewall) через приведение Applier к *applierImpl.
func TestApply_AllPolicyVariants_HaveDPIPrecleanupAndBypass(t *testing.T) {
	tests := []struct {
		name        string
		call        func(a *applierImpl, ctx context.Context) error
		bypassTable string
	}{
		{
			name:        "TUN-mode",
			call:        func(a *applierImpl, ctx context.Context) error { return a.applyPolicyTUNMode(ctx) },
			bypassTable: "mangle",
		},
		{
			name:        "TProxy-real",
			call:        func(a *applierImpl, ctx context.Context) error { return a.applyPolicyTProxyReal(ctx) },
			bypassTable: "mangle",
		},
		{
			name:        "Redirect",
			call:        func(a *applierImpl, ctx context.Context) error { return a.applyPolicyRedirect(ctx) },
			bypassTable: "nat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &autoRunner{}
			cfg := DefaultConfig()
			cfg.PolicyMark = 0xffffaab
			cfg.DPIEnabled = true
			cfg.WANIface = "eth3"
			cfg.AdminPorts = []uint16{22, 222}
			cfg.LANAddrs = []string{"172.16.0.1"}
			ai, ok := NewApplier(r, cfg).(*applierImpl)
			if !ok {
				t.Fatal("NewApplier не вернул *applierImpl")
			}

			if err := tt.call(ai, context.Background()); err != nil {
				t.Fatalf("%s: вызов вернул ошибку: %v", tt.name, err)
			}

			// (а) pre-cleanup ДО повторного добавления FORWARD-jump.
			precleanupIdx := -1
			jumpAddIdx := -1
			for i, c := range r.calls {
				if precleanupIdx == -1 && strings.Contains(c, "-D FORWARD -j signcraze_dpi_fwd") {
					precleanupIdx = i
				}
				if jumpAddIdx == -1 && strings.Contains(c, "-A FORWARD") && strings.Contains(c, "signcraze_dpi_fwd") {
					jumpAddIdx = i
				}
			}
			if precleanupIdx == -1 {
				t.Error("pre-cleanup DeleteJumpAll(mangle, FORWARD, signcraze_dpi_fwd) не вызван")
			}
			if jumpAddIdx == -1 {
				t.Error("FORWARD jump на signcraze_dpi_fwd не добавлен")
			}
			if precleanupIdx != -1 && jumpAddIdx != -1 && precleanupIdx > jumpAddIdx {
				t.Errorf(
					"pre-cleanup (idx=%d) вызван ПОСЛЕ добавления jump (idx=%d): при re-apply jump задублируется",
					precleanupIdx, jumpAddIdx,
				)
			}

			// (б) DPIForwardRules присутствуют.
			if !r.hasCall("iptables -t mangle -N signcraze_dpi_fwd") {
				t.Error("цепочка signcraze_dpi_fwd не создана")
			}
			foundNFQUEUE := false
			for _, c := range r.calls {
				if strings.Contains(c, "signcraze_dpi_fwd") && strings.Contains(c, "NFQUEUE") {
					foundNFQUEUE = true
					break
				}
			}
			if !foundNFQUEUE {
				t.Error("ни одного NFQUEUE-правила в signcraze_dpi_fwd не найдено")
			}

			// (в) LOCAL bypass и admin-ports bypass вставлены с pos=1 в ожидаемой таблице.
			lanBypass := fmt.Sprintf("iptables -t %s -I signcraze_policy 1 -d 172.16.0.1 -j RETURN", tt.bypassTable)
			adminBypass := fmt.Sprintf("iptables -t %s -I signcraze_policy 1 -p tcp --dport 22 -j RETURN", tt.bypassTable)
			if !r.hasCall(lanBypass) {
				t.Errorf("LOCAL bypass не найден: %s", lanBypass)
			}
			if !r.hasCall(adminBypass) {
				t.Errorf("admin-ports bypass не найден: %s", adminBypass)
			}
		})
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

//go:build integration

package firewall

import (
	"context"
	"net/netip"
	"os/exec"
	"testing"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// skipIfNoIPTables пропускает тест если iptables/ipset/ip недоступны.
func skipIfNoIPTables(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"iptables", "ipset", "ip"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("бинарь %s не найден: %v", bin, err)
		}
	}
}

func newRealRunner() exectx.Runner {
	return exectx.OS
}

func TestIntegration_IPTables_EnsureAndDeleteRule(t *testing.T) {
	skipIfNoIPTables(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runner := newRealRunner()
	ipt := New(runner)

	// Создаём тестовую цепочку
	const chain = "signcraze_integration_test"
	if err := ipt.EnsureChain(ctx, "mangle", chain); err != nil {
		t.Fatalf("EnsureChain: %v", err)
	}
	defer func() {
		_ = ipt.FlushAndDeleteChain(ctx, "mangle", chain)
	}()

	// Добавляем правило
	rule := []string{"-j", "MARK", "--set-mark", "0x53", "-m", "comment", "--comment", "signcraze:integration-test"}
	if err := ipt.EnsureRule(ctx, "mangle", chain, rule...); err != nil {
		t.Fatalf("EnsureRule: %v", err)
	}

	// Идемпотентность: второй вызов не должен вернуть ошибку
	if err := ipt.EnsureRule(ctx, "mangle", chain, rule...); err != nil {
		t.Fatalf("EnsureRule (повторный): %v", err)
	}

	// Проверяем наличие правила через -C (идемпотентная проверка присутствия).
	checkArgs := append([]string{"-t", "mangle", "-C", chain}, rule...)
	if _, err := runner.Run(ctx, "iptables", checkArgs...); err != nil {
		t.Fatalf("правило отсутствует после EnsureRule: %v", err)
	}

	// Удаляем правило
	if err := ipt.DeleteRule(ctx, "mangle", chain, rule...); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}

	// Идемпотентность удаления
	if err := ipt.DeleteRule(ctx, "mangle", chain, rule...); err != nil {
		t.Fatalf("DeleteRule (повторный): %v", err)
	}
}

// TestIntegration_RestoreBatch_AppliesRules проверяет полный путь RestoreBatch
// на реальном iptables-restore: проба поддержки --wait + атомарное применение
// батча. На хосте с любой сборкой iptables (с --wait или без) restore должен
// успешно применить правило (регрессия issue #3).
func TestIntegration_RestoreBatch_AppliesRules(t *testing.T) {
	skipIfNoIPTables(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runner := newRealRunner()
	ipt := New(runner)

	const chain = "signcraze_restore_test"
	if err := ipt.EnsureChain(ctx, "mangle", chain); err != nil {
		t.Fatalf("EnsureChain: %v", err)
	}
	defer func() {
		_ = ipt.FlushAndDeleteChain(ctx, "mangle", chain)
	}()

	var b BatchBuilder
	b.Table("mangle").
		Chain(chain).
		Flush(chain).
		Rule(chain, "-j", "MARK", "--set-mark", "0x53").
		Commit()

	if err := ipt.RestoreBatch(ctx, b.Bytes()); err != nil {
		t.Fatalf("RestoreBatch: %v", err)
	}

	// Правило должно присутствовать после атомарного restore.
	if _, err := runner.Run(ctx, "iptables", "-t", "mangle", "-C", chain, "-j", "MARK", "--set-mark", "0x53"); err != nil {
		t.Fatalf("правило отсутствует после RestoreBatch: %v", err)
	}

	// Повторный батч идемпотентен (Flush + повторное добавление).
	if err := ipt.RestoreBatch(ctx, b.Bytes()); err != nil {
		t.Fatalf("RestoreBatch (повторный): %v", err)
	}
}

func TestIntegration_IPSet_AtomicReplace(t *testing.T) {
	skipIfNoIPTables(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runner := newRealRunner()
	s := NewIPSet(runner)

	const setName = "signcraze_integration_v4"
	if err := s.EnsureSet(ctx, setName, "hash:net", "inet"); err != nil {
		t.Fatalf("EnsureSet: %v", err)
	}
	defer func() {
		_ = s.DestroySet(ctx, setName)
	}()

	cidrs := []netip.Prefix{
		netip.MustParsePrefix("1.2.3.0/24"),
		netip.MustParsePrefix("10.0.0.0/8"),
	}
	if err := s.AtomicReplace(ctx, setName, "hash:net", "inet", cidrs); err != nil {
		t.Fatalf("AtomicReplace: %v", err)
	}

	// Повторная замена с другим набором
	cidrs2 := []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16")}
	if err := s.AtomicReplace(ctx, setName, "hash:net", "inet", cidrs2); err != nil {
		t.Fatalf("AtomicReplace (второй): %v", err)
	}
}

func TestIntegration_Applier_ApplyAndRemove_Full(t *testing.T) {
	skipIfNoIPTables(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	runner := newRealRunner()
	cfg := DefaultConfig()
	a := NewApplier(runner, cfg)

	// Применяем правила (legacy режим full)
	if err := a.Apply(ctx, types.ModeFull); err != nil {
		t.Fatalf("Apply(full): %v", err)
	}

	// Откатываем — Remove должен быть идемпотентен
	if err := a.Remove(ctx); err != nil {
		t.Fatalf("Remove после Apply: %v", err)
	}

	// Remove на чистом состоянии не должен давать ошибку
	if err := a.Remove(ctx); err != nil {
		t.Fatalf("Remove повторный: %v", err)
	}
}

func TestIntegration_IPRule_EnsureAndDelete(t *testing.T) {
	skipIfNoIPTables(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runner := newRealRunner()

	// Добавляем ip rule
	if err := EnsureIPRule(ctx, runner, 0x53, 83, 32765); err != nil {
		t.Fatalf("EnsureIPRule: %v", err)
	}
	// Идемпотентность
	if err := EnsureIPRule(ctx, runner, 0x53, 83, 32765); err != nil {
		t.Fatalf("EnsureIPRule (повторный): %v", err)
	}

	// Удаляем
	if err := DeleteIPRule(ctx, runner, 0x53, 83); err != nil {
		t.Fatalf("DeleteIPRule: %v", err)
	}
	// Идемпотентность удаления
	if err := DeleteIPRule(ctx, runner, 0x53, 83); err != nil {
		t.Fatalf("DeleteIPRule (повторный): %v", err)
	}
}

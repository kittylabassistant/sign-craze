package firewall

import (
	"context"
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// IPTables управляет правилами iptables через exectx.Runner.
type IPTables struct {
	runner exectx.Runner
}

// New создаёт IPTables с заданным runner.
func New(runner exectx.Runner) *IPTables {
	return &IPTables{runner: runner}
}

// EnsureRule добавляет правило если оно отсутствует (iptables -C → -A). Идемпотентно.
func (t *IPTables) EnsureRule(ctx context.Context, table, chain string, rule ...string) error {
	checkArgs := append([]string{"-t", table, "-C", chain}, rule...)
	if _, err := t.runner.Run(ctx, "iptables", checkArgs...); err == nil {
		return nil
	}
	addArgs := append([]string{"-t", table, "-A", chain}, rule...)
	if _, err := t.runner.Run(ctx, "iptables", addArgs...); err != nil {
		return fmt.Errorf("firewall: добавление правила в %s/%s: %w", table, chain, err)
	}
	log.L().Debug("firewall: правило добавлено", "table", table, "chain", chain)
	return nil
}

// DeleteRule удаляет правило если оно существует (iptables -C → -D). Идемпотентно.
func (t *IPTables) DeleteRule(ctx context.Context, table, chain string, rule ...string) error {
	checkArgs := append([]string{"-t", table, "-C", chain}, rule...)
	if _, err := t.runner.Run(ctx, "iptables", checkArgs...); err != nil {
		return nil
	}
	deleteArgs := append([]string{"-t", table, "-D", chain}, rule...)
	if _, err := t.runner.Run(ctx, "iptables", deleteArgs...); err != nil {
		return fmt.Errorf("firewall: удаление правила из %s/%s: %w", table, chain, err)
	}
	log.L().Debug("firewall: правило удалено", "table", table, "chain", chain)
	return nil
}

// EnsureChain создаёт цепочку если она отсутствует. Идемпотентно.
func (t *IPTables) EnsureChain(ctx context.Context, table, chain string) error {
	_, err := t.runner.Run(ctx, "iptables", "-t", table, "-N", chain)
	if err == nil {
		return nil
	}
	// iptables -N возвращает ненулевой код если цепочка уже существует — не ошибка
	if strings.Contains(err.Error(), "already exist") || strings.Contains(err.Error(), "Chain already exists") {
		return nil
	}
	return fmt.Errorf("firewall: создание цепочки %s/%s: %w", table, chain, err)
}

// FlushAndDeleteChain очищает и удаляет цепочку. Идемпотентно.
func (t *IPTables) FlushAndDeleteChain(ctx context.Context, table, chain string) error {
	if _, err := t.runner.Run(ctx, "iptables", "-t", table, "-F", chain); err != nil {
		// цепочка отсутствует — ничего не нужно делать
		return nil
	}
	if _, err := t.runner.Run(ctx, "iptables", "-t", table, "-X", chain); err != nil {
		return fmt.Errorf("firewall: удаление цепочки %s/%s: %w", table, chain, err)
	}
	return nil
}

// ListRules возвращает правила цепочки (iptables -S chain).
func (t *IPTables) ListRules(ctx context.Context, table, chain string) ([]string, error) {
	res, err := t.runner.Run(ctx, "iptables", "-t", table, "-S", chain)
	if err != nil {
		return nil, fmt.Errorf("firewall: список правил %s/%s: %w", table, chain, err)
	}
	var rules []string
	for _, line := range strings.Split(strings.TrimSpace(string(res.Stdout)), "\n") {
		if line != "" {
			rules = append(rules, line)
		}
	}
	return rules, nil
}

// DeleteRulesByComment удаляет все правила таблицы, содержащие commentPrefix в комментарии.
// Используется при откате и при --stop для гарантированной очистки.
func (t *IPTables) DeleteRulesByComment(ctx context.Context, table, commentPrefix string) error {
	res, err := t.runner.Run(ctx, "iptables", "-t", table, "-S")
	if err != nil {
		return fmt.Errorf("firewall: получение правил таблицы %s: %w", table, err)
	}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		if !strings.Contains(line, commentPrefix) {
			continue
		}
		// строки вида "-A CHAIN args..." → iptables -t table -D CHAIN args...
		if !strings.HasPrefix(line, "-A ") {
			continue
		}
		rest := line[3:]
		idx := strings.IndexByte(rest, ' ')
		if idx < 0 {
			continue
		}
		chain := rest[:idx]
		ruleStr := rest[idx+1:]
		deleteArgs := append([]string{"-t", table, "-D", chain}, strings.Fields(ruleStr)...)
		if _, delErr := t.runner.Run(ctx, "iptables", deleteArgs...); delErr != nil {
			log.L().Warn("firewall: не удалось удалить правило", "line", line, "err", delErr)
		}
	}
	return nil
}

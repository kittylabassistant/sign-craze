package firewall

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// maxIptablesOutputLine — лимит на одну строку iptables -S при сканировании.
// Защита от raw-output > 256KB, который мог бы исчерпать RAM на 128MB роутере.
const maxIptablesOutputLine = 256 * 1024

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

// InsertRule вставляет правило в позицию position (iptables -I chain pos), если оно
// отсутствует. Используется для exclude-правил, которые должны выполняться ДО
// mark-правил в той же цепочке (safety-fixes #1).
func (t *IPTables) InsertRule(ctx context.Context, table, chain string, position int, rule ...string) error {
	checkArgs := append([]string{"-t", table, "-C", chain}, rule...)
	if _, err := t.runner.Run(ctx, "iptables", checkArgs...); err == nil {
		return nil
	}
	insertArgs := append([]string{"-t", table, "-I", chain, fmt.Sprintf("%d", position)}, rule...)
	if _, err := t.runner.Run(ctx, "iptables", insertArgs...); err != nil {
		return fmt.Errorf("firewall: вставка правила в %s/%s pos=%d: %w", table, chain, position, err)
	}
	log.L().Debug("firewall: правило вставлено", "table", table, "chain", chain, "pos", position)
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

// DeleteJumpAll удаляет все правила вида `-j target` из chain в table.
// Идемпотентно: цикл -D пока exit==0, не более maxIter итераций (защита
// от случайного бесконечного цикла при битом выводе iptables).
//
// Используется в Remove() как замена comment-based DeleteRulesByComment:
// busybox iptables 1.4.21 на стоковой Keenetic-прошивке часто не имеет
// модуля xt_comment, поэтому сами правила пишутся без --comment, а в
// PREROUTING остаются только jumps на наши user-chains, которые легко
// удалить по target-имени.
func (t *IPTables) DeleteJumpAll(ctx context.Context, table, chain, target string) error {
	const maxIter = 16
	for i := 0; i < maxIter; i++ {
		_, err := t.runner.Run(ctx, "iptables", "-t", table, "-D", chain, "-j", target)
		if err != nil {
			return nil
		}
	}
	return nil
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
	rules, scanErr := scanLines(res.Stdout)
	if scanErr != nil {
		return nil, fmt.Errorf("firewall: парсинг %s/%s: %w", table, chain, scanErr)
	}
	return rules, nil
}

// DeleteRulesByComment удаляет все правила таблицы, содержащие commentPrefix в комментарии.
// Используется при откате и при --stop для гарантированной очистки.
//
// Парсинг учитывает iptables-формат с quoted-комментариями
// `--comment "signcraze:xxx"` — strings.Fields() разрезал бы кавычки на отдельные
// токены и получившийся iptables -D не находил бы исходное правило.
func (t *IPTables) DeleteRulesByComment(ctx context.Context, table, commentPrefix string) error {
	res, err := t.runner.Run(ctx, "iptables", "-t", table, "-S")
	if err != nil {
		return fmt.Errorf("firewall: получение правил таблицы %s: %w", table, err)
	}
	lines, scanErr := scanLines(res.Stdout)
	if scanErr != nil {
		return fmt.Errorf("firewall: парсинг таблицы %s: %w", table, scanErr)
	}
	for _, line := range lines {
		if !strings.Contains(line, commentPrefix) {
			continue
		}
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
		args, splitErr := splitIptablesArgs(ruleStr)
		if splitErr != nil {
			log.L().Warn("firewall: не разобрал правило", "line", line, "err", splitErr)
			continue
		}
		deleteArgs := append([]string{"-t", table, "-D", chain}, args...)
		if _, delErr := t.runner.Run(ctx, "iptables", deleteArgs...); delErr != nil {
			log.L().Warn("firewall: не удалось удалить правило", "line", line, "err", delErr)
		}
	}
	return nil
}

// scanLines читает построчно через bufio.Scanner с расширенным буфером, чтобы
// длинные правила iptables не падали с ErrTooLong.
func scanLines(data []byte) ([]string, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), maxIptablesOutputLine)
	var lines []string
	for sc.Scan() {
		line := sc.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// splitIptablesArgs разбивает строку правила на токены с учётом double-quoted
// строк. Кавычки в выводе iptables-save обрамляют значения опций, содержащих
// пробелы (типичный случай: --comment "sign-craze: xxx"). Возвращает токены БЕЗ
// внешних кавычек.
//
// Пример: `-p tcp -m comment --comment "signcraze:foo" -j MARK`
// → ["-p", "tcp", "-m", "comment", "--comment", "signcraze:foo", "-j", "MARK"]
func splitIptablesArgs(s string) ([]string, error) {
	var (
		out      []string
		buf      strings.Builder
		inQuotes bool
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			inQuotes = !inQuotes
		case c == ' ' && !inQuotes:
			if buf.Len() > 0 {
				out = append(out, buf.String())
				buf.Reset()
			}
		default:
			buf.WriteByte(c)
		}
	}
	if inQuotes {
		return nil, fmt.Errorf("несбалансированные кавычки: %q", s)
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out, nil
}

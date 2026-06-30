package firewall

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// BatchBuilder собирает дамп в формате iptables-save для передачи в iptables-restore.
// Методы возвращают *BatchBuilder для цепочек вызовов. Не потокобезопасен.
type BatchBuilder struct {
	buf bytes.Buffer
}

// Table начинает секцию таблицы (например "filter", "mangle", "nat").
// Эквивалентно строке "*table" в формате iptables-save.
func (b *BatchBuilder) Table(name string) *BatchBuilder {
	fmt.Fprintf(&b.buf, "*%s\n", name)
	return b
}

// Chain объявляет цепочку без политики по умолчанию (пользовательская цепочка).
// Эквивалентно ":CHAIN - [0:0]".
func (b *BatchBuilder) Chain(name string) *BatchBuilder {
	fmt.Fprintf(&b.buf, ":%s - [0:0]\n", name)
	return b
}

// Rule добавляет правило в цепочку.
// Эквивалентно "-A chain args...".
func (b *BatchBuilder) Rule(chain string, args ...string) *BatchBuilder {
	fmt.Fprintf(&b.buf, "-A %s %s\n", chain, strings.Join(args, " "))
	return b
}

// Flush добавляет команду сброса цепочки (-F chain).
// Используется перед refill цепочки для идемпотентного Apply:
// flush очищает предыдущие правила sign-craze не затрагивая другие цепочки.
func (b *BatchBuilder) Flush(chain string) *BatchBuilder {
	fmt.Fprintf(&b.buf, "-F %s\n", chain)
	return b
}

// Commit завершает секцию таблицы строкой COMMIT.
func (b *BatchBuilder) Commit() *BatchBuilder {
	b.buf.WriteString("COMMIT\n")
	return b
}

// Bytes возвращает собранный дамп.
func (b *BatchBuilder) Bytes() []byte {
	return b.buf.Bytes()
}

// IPTables управляет правилами iptables через exectx.Runner.
type IPTables struct {
	runner exectx.Runner

	// waitOnce/waitSupported кешируют результат пробы поддержки опции --wait
	// у iptables-restore (см. restoreWaitSupported). Проба выполняется один
	// раз на экземпляр.
	waitOnce      sync.Once
	waitSupported bool
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

// RestoreBatch выполняет iptables-restore --noflush с подготовленным дампом.
// dump в формате iptables-save: *table / правила / COMMIT.
// --noflush: существующие правила НЕ сбрасываются, только добавляются новые.
// --wait 2: таймаут захвата xtables lock (busybox iptables может удерживать
// lock дольше 1s при высокой нагрузке на slow MIPS softfloat). Флаг
// добавляется ТОЛЬКО если iptables-restore его поддерживает — legacy-сборки
// Entware (iptables ≤1.4.x) не знают --wait и трактуют его аргумент как имя
// входного файла (issue #3: "unrecognized option '--wait'" → "Can't open 2").
//
// Аналогично IPSet.AtomicReplace: один subprocess вместо N forkexec.
// На slow MIPS softfloat один iptables fork ≈ 20-50ms, batch из 12 правил
// экономит ≈ 500ms на цикл Reconcile.
//
// Если runner не реализует StdinRunner — fallback с ошибкой (вызывающая
// сторона обязана убедиться что runner поддерживает stdin, или использовать
// поштучный EnsureRule).
func (t *IPTables) RestoreBatch(ctx context.Context, dump []byte) error {
	sr, ok := t.runner.(exectx.StdinRunner)
	if !ok {
		return fmt.Errorf("firewall: runner не поддерживает StdinRunner (RestoreBatch требует StdinRunner)")
	}
	args := []string{"--noflush"}
	if t.restoreWaitSupported(ctx, sr) {
		args = append(args, "--wait", "2")
	}
	if _, err := sr.RunWithStdin(ctx, bytes.NewReader(dump), "iptables-restore", args...); err != nil {
		return fmt.Errorf("firewall: iptables-restore batch: %w", err)
	}
	log.L().Debug("firewall: iptables-restore batch выполнен", "bytes", len(dump))
	return nil
}

// restoreWaitSupported лениво (один раз на экземпляр) определяет, поддерживает
// ли iptables-restore опцию --wait. Проба — функциональный no-op: пустая
// секция filter с --noflush ничего не меняет в системе, но getopt отвергнет
// неизвестный --wait ещё до применения. err==nil ⇒ флаг поддержан.
//
// Любая ошибка пробы трактуется как «нет поддержки»: это безопасное
// направление — без --wait restore работает как поштучные iptables-вызовы
// (они --wait тоже не используют), теряется лишь повторная попытка захвата
// xtables lock.
func (t *IPTables) restoreWaitSupported(ctx context.Context, sr exectx.StdinRunner) bool {
	t.waitOnce.Do(func() {
		probe := []byte("*filter\nCOMMIT\n")
		_, err := sr.RunWithStdin(ctx, bytes.NewReader(probe), "iptables-restore", "--noflush", "--wait", "1")
		t.waitSupported = err == nil
		if !t.waitSupported {
			log.L().Warn("firewall: iptables-restore без поддержки --wait, lock-retry отключён")
		}
	})
	return t.waitSupported
}

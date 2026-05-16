package firewall

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

func TestIPTables_EnsureRule_ДобавляетЕслиОтсутствует(t *testing.T) {
	// -C не в таблице → ошибка (правило отсутствует) → должен вызвать -A
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -A CHAIN -j MARK --set-mark 0x53": {ExitCode: 0},
	})
	ipt := New(r)
	err := ipt.EnsureRule(context.Background(), "mangle", "CHAIN",
		"-j", "MARK", "--set-mark", "0x53")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestIPTables_EnsureRule_ПропускаетЕслиСуществует(t *testing.T) {
	// -C успешен → -A не должен вызываться (нет в таблице → вернул бы ошибку)
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -C CHAIN -j MARK --set-mark 0x53": {ExitCode: 0},
	})
	ipt := New(r)
	err := ipt.EnsureRule(context.Background(), "mangle", "CHAIN",
		"-j", "MARK", "--set-mark", "0x53")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestIPTables_InsertRule_ВставляетВНачало проверяет что InsertRule вызывает -I
// с указанной позицией. Используется для exclude-правил, которые ОБЯЗАНЫ идти
// перед mark-правилами (safety-fixes #1 — защита от SSH-lockout при reapply).
func TestIPTables_InsertRule_ВставляетВНачало(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -I signcraze 1 -j RETURN": {ExitCode: 0},
	})
	ipt := New(r)
	err := ipt.InsertRule(context.Background(), "mangle", "signcraze", 1, "-j", "RETURN")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestIPTables_InsertRule_ПропускаетЕслиСуществует(t *testing.T) {
	// -C успешен → -I не должен вызываться (нет в Mock map → вернул бы ошибку)
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -C signcraze -j RETURN": {ExitCode: 0},
	})
	ipt := New(r)
	err := ipt.InsertRule(context.Background(), "mangle", "signcraze", 1, "-j", "RETURN")
	if err != nil {
		t.Fatalf("ожидался nil для существующего правила: %v", err)
	}
}

func TestIPTables_DeleteRule_УдаляетЕслиСуществует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -C CHAIN -j ACCEPT": {ExitCode: 0},
		"iptables -t mangle -D CHAIN -j ACCEPT": {ExitCode: 0},
	})
	ipt := New(r)
	err := ipt.DeleteRule(context.Background(), "mangle", "CHAIN", "-j", "ACCEPT")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestIPTables_DeleteRule_ПропускаетЕслиОтсутствует(t *testing.T) {
	// -C нет в таблице → ошибка → DeleteRule должен вернуть nil (идемпотентно)
	r := exectx.Mock(map[string]exectx.Result{})
	ipt := New(r)
	err := ipt.DeleteRule(context.Background(), "mangle", "CHAIN", "-j", "ACCEPT")
	if err != nil {
		t.Fatalf("ожидался nil, получено: %v", err)
	}
}

func TestIPTables_EnsureChain_СоздаётНовую(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -N signcraze": {ExitCode: 0},
	})
	ipt := New(r)
	err := ipt.EnsureChain(context.Background(), "mangle", "signcraze")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestIPTables_EnsureChain_ИгнорируетСуществующую(t *testing.T) {
	// -N возвращает ошибку "Chain already exists" → EnsureChain должен вернуть nil
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -N signcraze": {
			ExitCode: 1,
			Stderr:   []byte("iptables: Chain already exists"),
		},
	})
	ipt := New(r)
	err := ipt.EnsureChain(context.Background(), "mangle", "signcraze")
	if err != nil {
		t.Fatalf("ожидался nil для существующей цепочки, получено: %v", err)
	}
}

// countingRunner — N первых вызовов успех, дальше ошибка. Для теста
// DeleteJumpAll, который повторяет -D пока exit==0.
type countingRunner struct {
	calls       int
	successQty  int
	errAfterMsg string
}

func (r *countingRunner) Run(_ context.Context, _ string, _ ...string) (exectx.Result, error) {
	r.calls++
	if r.calls <= r.successQty {
		return exectx.Result{ExitCode: 0}, nil
	}
	return exectx.Result{ExitCode: 1}, fmt.Errorf("%s", r.errAfterMsg)
}

// TestIPTables_DeleteJumpAll_УдаляетВсе — несколько успешных -D подряд,
// затем -D возвращает ошибку (правил больше нет) → метод выходит.
// Используется в Remove() как замена comment-based чистки.
func TestIPTables_DeleteJumpAll_УдаляетВсе(t *testing.T) {
	r := &countingRunner{successQty: 2, errAfterMsg: "no rule"}
	ipt := New(r)
	if err := ipt.DeleteJumpAll(context.Background(), "mangle", "PREROUTING", "signcraze_policy"); err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if r.calls != 3 {
		t.Errorf("ожидалось 3 вызова (2 успех + 1 not-found), получено %d", r.calls)
	}
}

// TestIPTables_DeleteJumpAll_NoOpEслиОтсутствует — первый -D сразу возвращает
// ошибку → выходим без ошибки наружу.
func TestIPTables_DeleteJumpAll_NoOpЕслиОтсутствует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{})
	ipt := New(r)
	if err := ipt.DeleteJumpAll(context.Background(), "mangle", "PREROUTING", "signcraze_policy"); err != nil {
		t.Fatalf("ожидался nil, получено: %v", err)
	}
}

func TestIPTables_FlushAndDeleteChain_УдаляетЦепочку(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -F signcraze": {ExitCode: 0},
		"iptables -t mangle -X signcraze": {ExitCode: 0},
	})
	ipt := New(r)
	err := ipt.FlushAndDeleteChain(context.Background(), "mangle", "signcraze")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestIPTables_FlushAndDeleteChain_ИгнорируетОтсутствующую(t *testing.T) {
	// -F возвращает ошибку (цепочки нет) → должен вернуть nil
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -F signcraze": {
			ExitCode: 1,
			Stderr:   []byte("iptables: No chain/target/match by that name"),
		},
	})
	ipt := New(r)
	err := ipt.FlushAndDeleteChain(context.Background(), "mangle", "signcraze")
	if err != nil {
		t.Fatalf("ожидался nil для отсутствующей цепочки, получено: %v", err)
	}
}

func TestIPTables_ListRules_ВозвращаетПравила(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -S signcraze": {
			ExitCode: 0,
			Stdout:   []byte("-N signcraze\n-A signcraze -j MARK\n"),
		},
	})
	ipt := New(r)
	rules, err := ipt.ListRules(context.Background(), "mangle", "signcraze")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("len(rules) = %d, ожидалось 2", len(rules))
	}
}

func TestIPTables_DeleteRulesByComment_УдаляетПоКомментарию(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -S": {
			ExitCode: 0,
			Stdout: []byte(
				"-N signcraze\n" +
					"-A signcraze -j MARK --set-mark 0x53 -m comment --comment signcraze:mark-ipv4\n" +
					"-A PREROUTING -j ACCEPT\n",
			),
		},
		"iptables -t mangle -D signcraze -j MARK --set-mark 0x53 -m comment --comment signcraze:mark-ipv4": {ExitCode: 0},
	})
	ipt := New(r)
	err := ipt.DeleteRulesByComment(context.Background(), "mangle", "signcraze:")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestIPTables_DeleteRulesByComment_QuotedComment — iptables-save может выводить
// --comment "value with spaces" в кавычках. strings.Fields() разрезал бы это на
// отдельные токены, и iptables -D не нашёл бы исходное правило.
func TestIPTables_DeleteRulesByComment_QuotedComment(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -S": {
			ExitCode: 0,
			Stdout: []byte(
				"-A signcraze -p tcp -m comment --comment \"signcraze:foo bar\" -j MARK\n",
			),
		},
		// Кавычки должны быть удалены при формировании delete-args
		"iptables -t mangle -D signcraze -p tcp -m comment --comment signcraze:foo bar -j MARK": {ExitCode: 0},
	})
	ipt := New(r)
	err := ipt.DeleteRulesByComment(context.Background(), "mangle", "signcraze:")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// --- Task A: BatchBuilder + RestoreBatch ---

// stdinRecorder — StdinRunner, записывающий переданный stdin для golden-тестов.
type stdinRecorder struct {
	name  string
	args  []string
	stdin []byte
	fail  bool
}

func (r *stdinRecorder) Run(_ context.Context, name string, args ...string) (exectx.Result, error) {
	return exectx.Result{ExitCode: 0}, nil
}

func (r *stdinRecorder) RunWithStdin(_ context.Context, stdin io.Reader, name string, args ...string) (exectx.Result, error) {
	r.name = name
	r.args = args
	data, err := io.ReadAll(stdin)
	if err != nil {
		return exectx.Result{ExitCode: 1}, err
	}
	r.stdin = data
	if r.fail {
		return exectx.Result{ExitCode: 1, Stderr: []byte("iptables-restore error")},
			fmt.Errorf("iptables-restore failed")
	}
	return exectx.Result{ExitCode: 0}, nil
}

// TestBatchBuilder_Bytes_формат — проверяет что BatchBuilder генерирует
// корректный формат iptables-save: *table / :chain / -A rules / COMMIT.
func TestBatchBuilder_Bytes_формат(t *testing.T) {
	var b BatchBuilder
	b.Table("mangle").
		Chain("signcraze_policy").
		Rule("PREROUTING", "-j", "signcraze_policy").
		Rule("signcraze_policy", "-m", "mark", "--mark", "0xab", "-j", "MARK", "--set-mark", "0x53").
		Commit()

	got := string(b.Bytes())
	want := "*mangle\n:signcraze_policy - [0:0]\n-A PREROUTING -j signcraze_policy\n-A signcraze_policy -m mark --mark 0xab -j MARK --set-mark 0x53\nCOMMIT\n"
	if got != want {
		t.Errorf("BatchBuilder.Bytes():\ngot:  %q\nwant: %q", got, want)
	}
}

// TestIPTables_RestoreBatch_ПередаётСтандартныйВвод — RestoreBatch должен
// вызвать iptables-restore --noflush --wait 2 с правильным stdin.
func TestIPTables_RestoreBatch_ПередаётСтандартныйВвод(t *testing.T) {
	rec := &stdinRecorder{}
	ipt := New(rec)
	dump := []byte("*mangle\nCOMMIT\n")
	if err := ipt.RestoreBatch(context.Background(), dump); err != nil {
		t.Fatalf("RestoreBatch вернул ошибку: %v", err)
	}
	if rec.name != "iptables-restore" {
		t.Errorf("команда = %q, ожидалось iptables-restore", rec.name)
	}
	if string(rec.stdin) != string(dump) {
		t.Errorf("stdin = %q, ожидалось %q", rec.stdin, dump)
	}
	// Проверяем флаги.
	wantArgs := []string{"--noflush", "--wait", "2"}
	for i, want := range wantArgs {
		if i >= len(rec.args) || rec.args[i] != want {
			t.Errorf("args[%d] = %q, ожидалось %q (args=%v)", i, rec.args[i], want, rec.args)
		}
	}
}

// TestIPTables_RestoreBatch_ОшибкаBezStdinRunner — runner без StdinRunner
// должен вернуть ошибку (не паниковать).
func TestIPTables_RestoreBatch_ОшибкаBezStdinRunner(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{})
	ipt := New(r)
	err := ipt.RestoreBatch(context.Background(), []byte("*mangle\nCOMMIT\n"))
	if err == nil {
		t.Fatal("ожидалась ошибка при runner без StdinRunner")
	}
}

// TestIPTables_RestoreBatch_ПробрасываетОшибку — ошибка iptables-restore
// должна вернуться наружу.
func TestIPTables_RestoreBatch_ПробрасываетОшибку(t *testing.T) {
	rec := &stdinRecorder{fail: true}
	ipt := New(rec)
	err := ipt.RestoreBatch(context.Background(), []byte("*mangle\nCOMMIT\n"))
	if err == nil {
		t.Fatal("ожидалась ошибка от iptables-restore")
	}
}

// TestSplitIptablesArgs_табличный — разные форматы вывода iptables-save.
func TestSplitIptablesArgs_табличный(t *testing.T) {
	tests := []struct {
		in   string
		want []string
		err  bool
	}{
		{"-p tcp -j MARK", []string{"-p", "tcp", "-j", "MARK"}, false},
		{"-m comment --comment \"signcraze:foo\" -j MARK",
			[]string{"-m", "comment", "--comment", "signcraze:foo", "-j", "MARK"}, false},
		{"-m comment --comment \"with space\" -j ACCEPT",
			[]string{"-m", "comment", "--comment", "with space", "-j", "ACCEPT"}, false},
		{"--comment \"unbalanced", nil, true},
	}
	for _, tt := range tests {
		got, err := splitIptablesArgs(tt.in)
		if tt.err {
			if err == nil {
				t.Errorf("splitIptablesArgs(%q): ожидалась ошибка", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("splitIptablesArgs(%q): %v", tt.in, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("splitIptablesArgs(%q) len=%d, ожидалось %d: got=%v", tt.in, len(got), len(tt.want), got)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitIptablesArgs(%q)[%d] = %q, ожидалось %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}

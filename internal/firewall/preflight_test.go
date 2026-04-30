package firewall

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// TestCheckRequiredIptablesModules_AllPresent — оба пробных вызова
// возвращают help-текст без "Couldn't find" → ошибки нет.
func TestCheckRequiredIptablesModules_AllPresent(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -j TPROXY --help": {ExitCode: 0, Stdout: []byte("TPROXY target options: ...")},
		"iptables -m set --help":    {ExitCode: 0, Stdout: []byte("set match options: ...")},
	})
	if err := CheckRequiredIptablesModules(context.Background(), r); err != nil {
		t.Fatalf("ожидался nil, получено: %v", err)
	}
}

// TestCheckRequiredIptablesModules_TproxyMissing — TPROXY отсутствует
// (busybox без xt_TPROXY на Keenetic). Ошибка должна упоминать TPROXY и
// opkg-пакет iptables-mod-tproxy.
func TestCheckRequiredIptablesModules_TproxyMissing(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -j TPROXY --help": {ExitCode: 2, Stderr: []byte("iptables v1.4.21: Couldn't find target `TPROXY'")},
		"iptables -m set --help":    {ExitCode: 0, Stdout: []byte("set match options")},
	})
	err := CheckRequiredIptablesModules(context.Background(), r)
	if err == nil {
		t.Fatal("ожидалась ошибка про отсутствующий TPROXY")
	}
	msg := err.Error()
	if !strings.Contains(msg, "TPROXY") || !strings.Contains(msg, "iptables-mod-tproxy") {
		t.Errorf("ошибка должна содержать TPROXY и opkg-инструкцию, получено: %s", msg)
	}
}

// TestCheckRequiredIptablesModules_BothMissing — оба модуля отсутствуют;
// сообщение перечисляет оба и оба opkg-пакета.
func TestCheckRequiredIptablesModules_BothMissing(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -j TPROXY --help": {ExitCode: 2, Stderr: []byte("Couldn't find target")},
		"iptables -m set --help":    {ExitCode: 2, Stderr: []byte("Couldn't find match")},
	})
	err := CheckRequiredIptablesModules(context.Background(), r)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	msg := err.Error()
	for _, want := range []string{"TPROXY", "set", "iptables-mod-tproxy", "iptables-mod-ipset", "ipset"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ошибка должна содержать %q, получено: %s", want, msg)
		}
	}
}

// TestIptablesModuleAvailable_RunnerError — runner возвращает ошибку
// без распознаваемого stderr → считаем модуль недоступным (fail-safe).
func TestIptablesModuleAvailable_RunnerError(t *testing.T) {
	r := matcherErrRunner{err: errors.New("exec failed")}
	if iptablesModuleAvailable(context.Background(), r, "-j", "TPROXY") {
		t.Error("ожидалось false при ошибке runner")
	}
}

type matcherErrRunner struct{ err error }

func (m matcherErrRunner) Run(_ context.Context, _ string, _ ...string) (exectx.Result, error) {
	return exectx.Result{}, m.err
}

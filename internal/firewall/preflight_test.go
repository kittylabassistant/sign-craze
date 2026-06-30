package firewall

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// TestCheckTUNAvailable_HostHasTUN — на dev-машине /dev/net/tun обычно есть.
// Тест skip'ается если файла нет (CI-контейнер без TUN — это нормально).
// Проверяем напрямую checkTUNAvailableReal, обходя test-override переменной.
func TestCheckTUNAvailable_HostHasTUN(t *testing.T) {
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		t.Skipf("/dev/net/tun отсутствует на хосте — skip: %v", err)
	}
	if err := checkTUNAvailableReal(); err != nil {
		t.Errorf("ожидался nil, получено: %v", err)
	}
}

// TestCheckRequiredBinaries_AllPresent — все бинари отвечают на --version → nil.
func TestCheckRequiredBinaries_AllPresent(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables --version":         {ExitCode: 0},
		"iptables-restore --version": {ExitCode: 0},
		"ipset --version":            {ExitCode: 0},
	})
	if err := CheckRequiredBinaries(context.Background(), r); err != nil {
		t.Errorf("ожидался nil, получено: %v", err)
	}
}

// TestCheckRequiredBinaries_Missing — отсутствие ipset (нет записи в Mock →
// Run вернёт ошибку) должно дать actionable-ошибку с подсказкой opkg (issue #3).
func TestCheckRequiredBinaries_Missing(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables --version":         {ExitCode: 0},
		"iptables-restore --version": {ExitCode: 0},
	})
	err := CheckRequiredBinaries(context.Background(), r)
	if err == nil {
		t.Fatal("ожидалась ошибка про отсутствующий ipset")
	}
	if !strings.Contains(err.Error(), "ipset") || !strings.Contains(err.Error(), "opkg install") {
		t.Errorf("ошибка должна упоминать ipset и opkg install, получено: %v", err)
	}
}

// TestCheckRequiredIptablesModules_SetMatchOK — match `set` доступен:
// iptables либо принимает rule (exit 0), либо ругается на несуществующий
// ipset (`doesn't exist`). Оба случая — положительный исход probe.
func TestCheckRequiredIptablesModules_SetMatchOK(t *testing.T) {
	cases := []struct {
		name   string
		result exectx.Result
	}{
		{"set match works, rule applied", exectx.Result{ExitCode: 0}},
		{"set match works, ipset missing", exectx.Result{
			ExitCode: 1,
			Stderr:   []byte("iptables v1.4.21: Set signcraze_probe_set doesn't exist."),
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := exectx.Mock(map[string]exectx.Result{
				// EnsureChain → list-chain check → если "doesn't exist" — создаём.
				"iptables -t mangle -N signcraze_probe": {ExitCode: 0},
				"iptables -t mangle -F signcraze_probe": {ExitCode: 0},
				"iptables -t mangle -X signcraze_probe": {ExitCode: 0},
				"iptables -t mangle -A signcraze_probe -m set --match-set signcraze_probe_set dst -j MARK --set-mark 0x1": c.result,
			})
			err := CheckRequiredIptablesModules(context.Background(), r)
			if err != nil {
				t.Errorf("ожидался nil, получено: %v", err)
			}
		})
	}
}

// TestCheckRequiredIptablesModules_SetMatchMissing — busybox iptables без xt_set
// возвращает «No chain/target/match by that name» при попытке использовать
// `-m set`. Pre-flight должен поймать это.
func TestCheckRequiredIptablesModules_SetMatchMissing(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"iptables -t mangle -N signcraze_probe": {ExitCode: 0},
		"iptables -t mangle -F signcraze_probe": {ExitCode: 0},
		"iptables -t mangle -X signcraze_probe": {ExitCode: 0},
		"iptables -t mangle -A signcraze_probe -m set --match-set signcraze_probe_set dst -j MARK --set-mark 0x1": {
			ExitCode: 2,
			Stderr:   []byte("iptables: No chain/target/match by that name."),
		},
	})
	err := CheckRequiredIptablesModules(context.Background(), r)
	if err == nil {
		t.Fatal("ожидалась ошибка про отсутствующий xt_set")
	}
	if !strings.Contains(err.Error(), "set") || !strings.Contains(err.Error(), "ipset") {
		t.Errorf("ошибка должна упоминать set и ipset, получено: %v", err)
	}
}

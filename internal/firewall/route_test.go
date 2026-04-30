package firewall

import (
	"context"
	"errors"
	"testing"

	scerrors "github.com/kittylabassistant/sign-craze/internal/errors"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// TestCheckFWMarkAvailable_NoConflict — нет правила с нашим fwmark → OK.
func TestCheckFWMarkAvailable_NoConflict(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip rule show": {ExitCode: 0, Stdout: []byte("0:\tlocal\n32766:\tmain\n")},
	})
	if err := CheckFWMarkAvailable(context.Background(), r, 0x53, 83); err != nil {
		t.Errorf("ожидался nil, получено %v", err)
	}
}

// TestCheckFWMarkAvailable_OurMarkExists — правило с нашим fwmark и нашей таблицей → OK.
func TestCheckFWMarkAvailable_OurMarkExists(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip rule show": {
			ExitCode: 0,
			Stdout:   []byte("32765:\tfrom all fwmark 0x53 lookup 83\n"),
		},
	})
	if err := CheckFWMarkAvailable(context.Background(), r, 0x53, 83); err != nil {
		t.Errorf("своё правило не должно быть конфликтом: %v", err)
	}
}

// TestCheckFWMarkAvailable_ConflictDifferentTable — fwmark 0x53 указывает на ЧУЖУЮ таблицу.
// Наглядный сценарий: XKeen или другой инструмент уже занял 0x53.
func TestCheckFWMarkAvailable_ConflictDifferentTable(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip rule show": {
			ExitCode: 0,
			Stdout:   []byte("32700:\tfrom all fwmark 0x53 lookup 100\n"),
		},
	})
	err := CheckFWMarkAvailable(context.Background(), r, 0x53, 83)
	if err == nil {
		t.Fatal("ожидалась ошибка конфликта")
	}
	if !errors.Is(err, scerrors.ErrFWMarkConflict) {
		t.Errorf("ожидался ErrFWMarkConflict, получено %T: %v", err, err)
	}
}

func TestEnsureIPRule_ДобавляетЕслиОтсутствует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		// ip rule show: правило отсутствует (нет совпадения в выводе)
		"ip rule show": {ExitCode: 0, Stdout: []byte("0:\tlocal\n32766:\tmain\n")},
		"ip rule add fwmark 0x53 table 83 priority 32765": {ExitCode: 0},
	})
	err := EnsureIPRule(context.Background(), r, 0x53, 83, 32765)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestEnsureIPRule_ПропускаетЕслиСуществует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip rule show": {
			ExitCode: 0,
			Stdout:   []byte("0:\tlocal\n32765:\tfwmark 0x53 lookup 83\n32766:\tmain\n"),
		},
	})
	err := EnsureIPRule(context.Background(), r, 0x53, 83, 32765)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestDeleteIPRule_УдаляетЕслиСуществует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip rule show": {
			ExitCode: 0,
			Stdout:   []byte("32765:\tfwmark 0x53 lookup 83\n"),
		},
		"ip rule del fwmark 0x53 table 83": {ExitCode: 0},
	})
	err := DeleteIPRule(context.Background(), r, 0x53, 83)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestDeleteIPRule_ПропускаетЕслиОтсутствует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip rule show": {ExitCode: 0, Stdout: []byte("0:\tlocal\n32766:\tmain\n")},
	})
	err := DeleteIPRule(context.Background(), r, 0x53, 83)
	if err != nil {
		t.Fatalf("ожидался nil, получено: %v", err)
	}
}

// EnsureLocalRoute использует `ip route replace` — идемпотентно без
// предварительной проверки (см. route.go: busybox-`ip` не показывает
// local-type routes в `route show table N`, давая false negatives).
func TestEnsureLocalRoute_ReplaceИдемпотентно(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip route replace local 0.0.0.0/0 dev lo table 83": {ExitCode: 0},
	})
	err := EnsureLocalRoute(context.Background(), r, 83)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestDeleteLocalRoute_УдаляетЕслиСуществует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip route show table 83":                       {ExitCode: 0, Stdout: []byte("local 0.0.0.0/0 dev lo scope host\n")},
		"ip route del local 0.0.0.0/0 dev lo table 83": {ExitCode: 0},
	})
	err := DeleteLocalRoute(context.Background(), r, 83)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestDeleteLocalRoute_ПропускаетЕслиОтсутствует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip route show table 83": {ExitCode: 0, Stdout: []byte("")},
	})
	err := DeleteLocalRoute(context.Background(), r, 83)
	if err != nil {
		t.Fatalf("ожидался nil, получено: %v", err)
	}
}

// TestLocalRouteExists_СтрогоеСравнение — подстрока "local" в чужом маршруте
// (например, locally-generated) не должна давать false positive.
func TestLocalRouteExists_СтрогоеСравнение(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
		want   bool
	}{
		{"наш маршрут", "local 0.0.0.0/0 dev lo scope host\n", true},
		{"чужой с local-словом", "192.168.1.1 dev eth0 src 192.168.1.10\n", false},
		{"только locally-generated подстрока",
			"default via 1.1.1.1 dev eth0 src locally-generated\n", false},
		{"наш + чужой", "192.168.1.0/24 dev eth0\nlocal 0.0.0.0/0 dev lo\n", true},
		{"пустой", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := exectx.Mock(map[string]exectx.Result{
				"ip route show table 83": {ExitCode: 0, Stdout: []byte(c.stdout)},
			})
			got := localRouteExists(context.Background(), r, 83)
			if got != c.want {
				t.Errorf("localRouteExists = %v, ожидалось %v (stdout=%q)", got, c.want, c.stdout)
			}
		})
	}
}

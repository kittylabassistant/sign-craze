package firewall

import (
	"context"
	"errors"
	"testing"
	"time"

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

// EnsureTUNRoute использует `ip route replace` — идемпотентно без
// предварительной проверки.
func TestEnsureTUNRoute_ReplaceИдемпотентно(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip route replace default dev signbox-tun table 83": {ExitCode: 0},
	})
	err := EnsureTUNRoute(context.Background(), r, "signbox-tun", 83)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestDeleteTUNRoute_УдаляетЕслиСуществует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip route del default dev signbox-tun table 83": {ExitCode: 0},
	})
	err := DeleteTUNRoute(context.Background(), r, "signbox-tun", 83)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestDeleteTUNRoute_ИгнорируетОшибкуЕслиОтсутствует(t *testing.T) {
	// Имитируем сценарий «маршрут уже отсутствует»: runner возвращает err.
	// DeleteTUNRoute не падает — это idempotent cleanup.
	r := exectx.Mock(map[string]exectx.Result{
		"ip route del default dev signbox-tun table 83": {
			ExitCode: 2,
			Stderr:   []byte("RTNETLINK answers: No such process"),
		},
	})
	err := DeleteTUNRoute(context.Background(), r, "signbox-tun", 83)
	if err != nil {
		t.Fatalf("ожидался nil (idempotent), получено: %v", err)
	}
}

func TestEnsureLinkUp_OK(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip link set signbox-tun up": {ExitCode: 0},
	})
	if err := EnsureLinkUp(context.Background(), r, "signbox-tun"); err != nil {
		t.Fatalf("ожидался nil, получено: %v", err)
	}
}

func TestEnsureLinkUp_KernelError(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip link set signbox-tun up": {
			ExitCode: 2,
			Stderr:   []byte("Cannot find device \"signbox-tun\""),
		},
	})
	if err := EnsureLinkUp(context.Background(), r, "signbox-tun"); err == nil {
		t.Fatal("ожидалась ошибка от kernel, получено nil")
	}
}

func TestWaitForInterface_ПоявляетсяСразу(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip link show signbox-tun": {
			ExitCode: 0,
			Stdout:   []byte("12: signbox-tun: <POINTOPOINT,UP> mtu 1500 qdisc fq_codel state UNKNOWN\n"),
		},
	})
	if err := WaitForInterface(context.Background(), r, "signbox-tun", time.Second); err != nil {
		t.Fatalf("ожидался nil, получено: %v", err)
	}
}

func TestWaitForInterface_TimeoutЕслиНетИнтерфейса(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip link show signbox-tun": {ExitCode: 1, Stderr: []byte("Device \"signbox-tun\" does not exist.")},
	})
	err := WaitForInterface(context.Background(), r, "signbox-tun", 300*time.Millisecond)
	if err == nil {
		t.Fatal("ожидалась ошибка timeout")
	}
}

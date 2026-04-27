package firewall

import (
	"context"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

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

func TestEnsureLocalRoute_ДобавляетЕслиОтсутствует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip route show table 83": {ExitCode: 0, Stdout: []byte("")},
		"ip route add local 0.0.0.0/0 dev lo table 83": {ExitCode: 0},
	})
	err := EnsureLocalRoute(context.Background(), r, 83)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestEnsureLocalRoute_ПропускаетЕслиСуществует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip route show table 83": {ExitCode: 0, Stdout: []byte("local 0.0.0.0/0 dev lo scope host\n")},
	})
	err := EnsureLocalRoute(context.Background(), r, 83)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestDeleteLocalRoute_УдаляетЕслиСуществует(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ip route show table 83": {ExitCode: 0, Stdout: []byte("local 0.0.0.0/0 dev lo scope host\n")},
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

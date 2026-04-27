package firewall

import (
	"context"
	"net/netip"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

func TestIPSet_EnsureSet_СоздаётНовый(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ipset list signcraze_ipv4":                        {ExitCode: 1, Stderr: []byte("The set with the given name does not exist")},
		"ipset create signcraze_ipv4 hash:net family inet": {ExitCode: 0},
	})
	s := NewIPSet(r)
	err := s.EnsureSet(context.Background(), "signcraze_ipv4", "hash:net", "inet")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestIPSet_EnsureSet_ПропускаетСуществующий(t *testing.T) {
	// ipset list возвращает 0 → набор существует → создание пропускается
	r := exectx.Mock(map[string]exectx.Result{
		"ipset list signcraze_ipv4": {ExitCode: 0, Stdout: []byte("Name: signcraze_ipv4\n")},
	})
	s := NewIPSet(r)
	err := s.EnsureSet(context.Background(), "signcraze_ipv4", "hash:net", "inet")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestIPSet_DestroySet_УдаляетНабор(t *testing.T) {
	r := exectx.Mock(map[string]exectx.Result{
		"ipset destroy signcraze_ipv4": {ExitCode: 0},
	})
	s := NewIPSet(r)
	err := s.DestroySet(context.Background(), "signcraze_ipv4")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestIPSet_DestroySet_ИгнорируетОтсутствующий(t *testing.T) {
	// destroy возвращает ошибку "does not exist" → должен вернуть nil
	r := exectx.Mock(map[string]exectx.Result{
		"ipset destroy signcraze_ipv4": {ExitCode: 1, Stderr: []byte("The set with the given name does not exist")},
	})
	s := NewIPSet(r)
	err := s.DestroySet(context.Background(), "signcraze_ipv4")
	if err != nil {
		t.Fatalf("ожидался nil для отсутствующего набора, получено: %v", err)
	}
}

func TestIPSet_AtomicReplace_ЗаменяетСодержимое(t *testing.T) {
	cidrs := []netip.Prefix{
		netip.MustParsePrefix("1.2.3.0/24"),
		netip.MustParsePrefix("10.0.0.0/8"),
	}
	r := exectx.Mock(map[string]exectx.Result{
		"ipset create signcraze_ipv4_tmp hash:net family inet": {ExitCode: 0},
		"ipset add signcraze_ipv4_tmp 1.2.3.0/24":              {ExitCode: 0},
		"ipset add signcraze_ipv4_tmp 10.0.0.0/8":              {ExitCode: 0},
		"ipset swap signcraze_ipv4 signcraze_ipv4_tmp":         {ExitCode: 0},
		"ipset destroy signcraze_ipv4_tmp":                     {ExitCode: 0},
	})
	s := NewIPSet(r)
	err := s.AtomicReplace(context.Background(), "signcraze_ipv4", "hash:net", "inet", cidrs)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestIPSet_AtomicReplace_ОткатПриОшибке(t *testing.T) {
	cidrs := []netip.Prefix{netip.MustParsePrefix("1.2.3.0/24")}
	r := exectx.Mock(map[string]exectx.Result{
		"ipset create signcraze_ipv4_tmp hash:net family inet": {ExitCode: 0},
		"ipset add signcraze_ipv4_tmp 1.2.3.0/24":              {ExitCode: 1, Stderr: []byte("hash is full")},
		"ipset destroy signcraze_ipv4_tmp":                     {ExitCode: 0},
	})
	s := NewIPSet(r)
	err := s.AtomicReplace(context.Background(), "signcraze_ipv4", "hash:net", "inet", cidrs)
	if err == nil {
		t.Fatal("ожидалась ошибка при сбое add")
	}
}

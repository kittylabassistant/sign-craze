package geo

import (
	"context"
	"net/netip"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

func TestParseCIDRList_ПарситIPv4(t *testing.T) {
	input := []byte("1.2.3.0/24\n10.0.0.0/8\n192.168.1.0/24\n")
	prefixes, err := ParseCIDRList(input)
	if err != nil {
		t.Fatalf("ParseCIDRList: %v", err)
	}
	if len(prefixes) != 3 {
		t.Errorf("len = %d, ожидалось 3", len(prefixes))
	}
}

func TestParseCIDRList_ПарситIPv6(t *testing.T) {
	input := []byte("2001:db8::/32\n::1/128\n")
	prefixes, err := ParseCIDRList(input)
	if err != nil {
		t.Fatalf("ParseCIDRList: %v", err)
	}
	if len(prefixes) != 2 {
		t.Errorf("len = %d, ожидалось 2", len(prefixes))
	}
}

func TestParseCIDRList_ПропускаетКомментарии(t *testing.T) {
	input := []byte("# Комментарий\n1.2.3.0/24\n# Ещё комментарий\n10.0.0.0/8\n")
	prefixes, err := ParseCIDRList(input)
	if err != nil {
		t.Fatalf("ParseCIDRList: %v", err)
	}
	if len(prefixes) != 2 {
		t.Errorf("len = %d, ожидалось 2 (комментарии должны пропускаться)", len(prefixes))
	}
}

func TestParseCIDRList_ПропускаетПустыеСтроки(t *testing.T) {
	input := []byte("\n\n1.2.3.0/24\n\n10.0.0.0/8\n\n")
	prefixes, err := ParseCIDRList(input)
	if err != nil {
		t.Fatalf("ParseCIDRList: %v", err)
	}
	if len(prefixes) != 2 {
		t.Errorf("len = %d, ожидалось 2", len(prefixes))
	}
}

func TestParseCIDRList_ОшибкаНаНевалидномCIDR(t *testing.T) {
	input := []byte("1.2.3.0/24\nnot-a-cidr\n10.0.0.0/8\n")
	_, err := ParseCIDRList(input)
	if err == nil {
		t.Error("ожидалась ошибка для невалидного CIDR")
	}
}

func TestParseCIDRList_МаскируетПрефиксы(t *testing.T) {
	// 1.2.3.4/24 должен стать 1.2.3.0/24 (Masked)
	input := []byte("1.2.3.4/24\n")
	prefixes, err := ParseCIDRList(input)
	if err != nil {
		t.Fatalf("ParseCIDRList: %v", err)
	}
	want := netip.MustParsePrefix("1.2.3.0/24")
	if prefixes[0] != want {
		t.Errorf("prefix = %v, ожидался %v (masked)", prefixes[0], want)
	}
}

func TestParseCIDRList_ПустойВход(t *testing.T) {
	prefixes, err := ParseCIDRList([]byte(""))
	if err != nil {
		t.Fatalf("ParseCIDRList: %v", err)
	}
	if len(prefixes) != 0 {
		t.Errorf("len = %d, ожидалось 0 для пустого входа", len(prefixes))
	}
}

func TestApplyToIPSet_ВызываетAtomicReplace(t *testing.T) {
	prefixes := []netip.Prefix{
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

	err := ApplyToIPSet(context.Background(), r, "signcraze_ipv4", "inet", prefixes)
	if err != nil {
		t.Fatalf("ApplyToIPSet: %v", err)
	}
}

func TestApplyToIPSet_ОшибкаПередаётся(t *testing.T) {
	// create tmp → ошибка → AtomicReplace вернёт ошибку
	r := exectx.Mock(map[string]exectx.Result{
		"ipset create signcraze_ipv4_tmp hash:net family inet": {
			ExitCode: 1,
			Stderr:   []byte("permission denied"),
		},
	})

	err := ApplyToIPSet(context.Background(), r, "signcraze_ipv4", "inet", nil)
	if err == nil {
		t.Error("ожидалась ошибка при сбое создания tmp ipset")
	}
}

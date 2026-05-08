package geo

import (
	"context"
	"net/netip"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

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

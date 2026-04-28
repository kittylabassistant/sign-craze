package geo

import (
	"context"
	"net/netip"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

func TestDecompileSRS_ExtractsCIDRs(t *testing.T) {
	stdout := `{
		"version": 2,
		"rules": [
			{"ip_cidr": ["1.1.1.0/24", "2.2.2.0/24"]},
			{"domain_suffix": ["example.com"]},
			{"ip_cidr": ["2001:db8::/32"]}
		]
	}`
	r := exectx.MockMatcher(func(name string, args ...string) (exectx.Result, error) {
		if name != "/opt/sbin/sing-box" || len(args) < 2 || args[0] != "rule-set" || args[1] != "decompile" {
			return exectx.Result{ExitCode: 1}, nil
		}
		return exectx.Result{Stdout: []byte(stdout)}, nil
	})

	prefixes, err := DecompileSRS(context.Background(), r, "/opt/sbin/sing-box", "/path/to/foo.srs")
	if err != nil {
		t.Fatalf("DecompileSRS: %v", err)
	}
	if len(prefixes) != 3 {
		t.Errorf("ожидалось 3 префикса, получено %d: %v", len(prefixes), prefixes)
	}
}

func TestDecompileSRS_DeduplicatesCIDRs(t *testing.T) {
	stdout := `{
		"version": 2,
		"rules": [
			{"ip_cidr": ["1.1.1.0/24", "1.1.1.0/24"]},
			{"ip_cidr": ["1.1.1.0/24"]}
		]
	}`
	r := exectx.MockMatcher(func(_ string, _ ...string) (exectx.Result, error) {
		return exectx.Result{Stdout: []byte(stdout)}, nil
	})
	prefixes, err := DecompileSRS(context.Background(), r, "sb", "f.srs")
	if err != nil {
		t.Fatalf("DecompileSRS: %v", err)
	}
	if len(prefixes) != 1 {
		t.Errorf("ожидался 1 префикс после дедупа, получено %d", len(prefixes))
	}
}

func TestDecompileSRS_InvalidCIDR(t *testing.T) {
	stdout := `{"version": 2, "rules": [{"ip_cidr": ["not-a-cidr"]}]}`
	r := exectx.MockMatcher(func(_ string, _ ...string) (exectx.Result, error) {
		return exectx.Result{Stdout: []byte(stdout)}, nil
	})
	if _, err := DecompileSRS(context.Background(), r, "sb", "f.srs"); err == nil {
		t.Error("ожидалась ошибка для некорректного CIDR")
	}
}

func TestDecompileSRS_RunnerError(t *testing.T) {
	r := exectx.MockMatcher(func(_ string, _ ...string) (exectx.Result, error) {
		return exectx.Result{ExitCode: 1, Stderr: []byte("file not found")}, &errCmd{}
	})
	if _, err := DecompileSRS(context.Background(), r, "sb", "missing.srs"); err == nil {
		t.Error("ожидалась ошибка от runner")
	}
}

type errCmd struct{}

func (e *errCmd) Error() string { return "mock exec error" }

func TestSplitByFamily(t *testing.T) {
	v4cidr := netip.MustParsePrefix("1.1.1.0/24")
	v6cidr := netip.MustParsePrefix("2001:db8::/32")

	v4, v6 := SplitByFamily([]netip.Prefix{v4cidr, v6cidr, v4cidr})
	if len(v4) != 2 || len(v6) != 1 {
		t.Errorf("ожидалось 2 v4 + 1 v6, получено %d v4 + %d v6", len(v4), len(v6))
	}
}

package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// decompileOutput — структура JSON, возвращаемая `sing-box rule-set decompile`.
// Соответствует sing-box PlainRuleSetCompat. Только нужные поля.
type decompileOutput struct {
	Version int                   `json:"version"`
	Rules   []decompileOutputRule `json:"rules"`
}

type decompileOutputRule struct {
	IPCIDR []string `json:"ip_cidr,omitempty"`
}

// DecompileSRS вызывает `sing-box rule-set decompile` на указанном файле и
// извлекает все CIDR из правил. Используется для заполнения kernel ipset
// (signcraze_ipv4/ipv6) после загрузки .srs (safety-fixes #17).
//
// singboxBin — путь к бинарю sing-box (например, /opt/sbin/sing-box).
// srsPath — путь к скомпилированному правилу .srs.
//
// Возвращает уникальные префиксы; разделение по семейству (v4/v6) — на
// стороне вызывающего через netip.Prefix.Addr().Is4() / Is6().
func DecompileSRS(ctx context.Context, runner exectx.Runner, singboxBin, srsPath string) ([]netip.Prefix, error) {
	res, err := runner.Run(ctx, singboxBin, "rule-set", "decompile", "-o", "/dev/stdout", srsPath)
	if err != nil {
		return nil, fmt.Errorf("geo decompile: %w (stderr: %s)", err, res.Stderr)
	}

	var out decompileOutput
	if err := json.Unmarshal(res.Stdout, &out); err != nil {
		return nil, fmt.Errorf("geo decompile: парсинг JSON: %w", err)
	}

	seen := make(map[string]struct{})
	var prefixes []netip.Prefix
	for _, rule := range out.Rules {
		for _, cidr := range rule.IPCIDR {
			if _, dup := seen[cidr]; dup {
				continue
			}
			seen[cidr] = struct{}{}
			p, err := netip.ParsePrefix(cidr)
			if err != nil {
				return nil, fmt.Errorf("geo decompile: некорректный CIDR %q: %w", cidr, err)
			}
			prefixes = append(prefixes, p)
		}
	}
	return prefixes, nil
}

// SplitByFamily разделяет префиксы на IPv4 и IPv6 списки.
// Используется перед заполнением signcraze_ipv4 / signcraze_ipv6.
func SplitByFamily(prefixes []netip.Prefix) (v4, v6 []netip.Prefix) {
	for _, p := range prefixes {
		if p.Addr().Is4() {
			v4 = append(v4, p)
		} else {
			v6 = append(v6, p)
		}
	}
	return v4, v6
}

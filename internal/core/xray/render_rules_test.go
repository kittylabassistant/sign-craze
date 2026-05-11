package xray

import (
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestTranslateRoutingRules(t *testing.T) {
	tests := []struct {
		name           string
		rc             *types.RoutingConfig
		defaultTag     string
		wantRuleCount  int
		wantOutbounds  []string // ожидаемые outboundTag в порядке появления
		wantDomains    []string // подстроки, которые должны встретиться в JSON domain[]
		wantIPs        []string // подстроки в ip[]
		wantWarningSub []string // подстроки, которые должны быть в warnings
	}{
		{
			name: "пустой RoutingConfig+final → fallback не нужен",
			rc: &types.RoutingConfig{
				Version: 1,
			},
			defaultTag:    "vpn",
			wantRuleCount: 0,
		},
		{
			name: "Final → catch-all rule",
			rc: &types.RoutingConfig{
				Version: 1,
				Final:   "vpn",
			},
			defaultTag:    "vpn",
			wantRuleCount: 1,
			wantOutbounds: []string{"vpn"},
		},
		{
			name: "Domain matchers с префиксами",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{
						Domain:        []string{"exact.com"},
						DomainSuffix:  []string{"suf.com"},
						DomainKeyword: []string{"kw"},
						DomainRegex:   []string{`^re\..+`},
						Outbound:      "vpn",
					},
				},
				Final: "direct",
			},
			defaultTag:    "vpn",
			wantRuleCount: 2,
			wantOutbounds: []string{"vpn", "direct"},
			wantDomains:   []string{"domain:full:exact.com", "domain:suf.com", "domain:keyword:kw", `regexp:^re\..+`},
		},
		{
			name: "RuleSet geosite-X → geosite:X translation",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{RuleSet: []string{"geosite-youtube"}, Outbound: "direct"},
				},
			},
			defaultTag:    "vpn",
			wantRuleCount: 1,
			wantDomains:   []string{"geosite:youtube"},
		},
		{
			name: "RuleSet geoip-ru → geoip:ru translation",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{RuleSet: []string{"geoip-ru"}, Outbound: "direct"},
				},
			},
			defaultTag:    "vpn",
			wantRuleCount: 1,
			wantIPs:       []string{"geoip:ru"},
		},
		{
			name: "RuleSet без префикса → warning + skip",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{RuleSet: []string{"refilter-blocked-domains"}, Outbound: "vpn"},
				},
			},
			defaultTag:     "vpn",
			wantRuleCount:  0, // нет matchers, outbound пустой → skip
			wantWarningSub: []string{"refilter-blocked-domains", "geosite-/geoip-"},
		},
		{
			name: "Action reject → outboundTag=block",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{Domain: []string{"ads.com"}, Action: "reject"},
				},
			},
			defaultTag:    "vpn",
			wantRuleCount: 1,
			wantOutbounds: []string{"block"},
		},
		{
			name: "Port translation: 80,443 + 1000:2000 → 80,443,1000-2000",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{Port: []uint16{80, 443}, PortRange: []string{"1000:2000"}, Outbound: "vpn"},
				},
			},
			defaultTag:    "vpn",
			wantRuleCount: 1,
		},
		{
			name: "sniff action → пропуск с warning",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{Domain: []string{"x.com"}, Action: "sniff"},
				},
			},
			defaultTag:     "vpn",
			wantRuleCount:  0,
			wantWarningSub: []string{"sniff", "не поддерживает"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, warnings := TranslateRoutingRules(tt.rc, tt.defaultTag)
			if len(rules) != tt.wantRuleCount {
				t.Errorf("rules count: got %d, want %d (rules: %+v)", len(rules), tt.wantRuleCount, rules)
			}

			// Verify outboundTags в порядке появления
			for i, wantOut := range tt.wantOutbounds {
				if i >= len(rules) {
					t.Errorf("rules[%d]: ожидался outboundTag=%q, но rule отсутствует", i, wantOut)
					continue
				}
				got, _ := rules[i]["outboundTag"].(string)
				if got != wantOut {
					t.Errorf("rules[%d].outboundTag: got %q, want %q", i, got, wantOut)
				}
			}

			// Verify domain[] подстроки
			allDomains := flattenStringSlices(rules, "domain")
			for _, wantD := range tt.wantDomains {
				if !containsString(allDomains, wantD) {
					t.Errorf("domain matcher %q не найден в %v", wantD, allDomains)
				}
			}

			// Verify ip[] подстроки
			allIPs := flattenStringSlices(rules, "ip")
			for _, wantIP := range tt.wantIPs {
				if !containsString(allIPs, wantIP) {
					t.Errorf("ip matcher %q не найден в %v", wantIP, allIPs)
				}
			}

			// Verify warnings
			joinedWarnings := strings.Join(warnings, " | ")
			for _, sub := range tt.wantWarningSub {
				if !strings.Contains(joinedWarnings, sub) {
					t.Errorf("warning подстрока %q не найдена в: %s", sub, joinedWarnings)
				}
			}
		})
	}
}

// flattenStringSlices извлекает все строки из field rule[key] across all rules.
func flattenStringSlices(rules []map[string]any, key string) []string {
	var out []string
	for _, r := range rules {
		v, ok := r[key]
		if !ok {
			continue
		}
		ss, ok := v.([]string)
		if !ok {
			continue
		}
		out = append(out, ss...)
	}
	return out
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

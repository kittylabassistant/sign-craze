package mihomo

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
		wantRules      []string // ожидаемые строки правил (substring match)
		wantProviders  []string // ожидаемые tag'и провайдеров
		wantWarningSub []string // подстроки в warnings
	}{
		{
			name: "Final → MATCH,<final>",
			rc: &types.RoutingConfig{
				Version: 1,
				Final:   "vpn",
			},
			defaultTag: "vpn",
			wantRules:  []string{"MATCH,vpn"},
		},
		{
			name: "Final пустой → MATCH,<defaultTag>",
			rc: &types.RoutingConfig{
				Version: 1,
			},
			defaultTag: "vpn",
			wantRules:  []string{"MATCH,vpn"},
		},
		{
			name: "DomainSuffix → DOMAIN-SUFFIX (без leading dot)",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{DomainSuffix: []string{".youtube.com"}, Outbound: "direct"},
				},
				Final: "vpn",
			},
			defaultTag: "vpn",
			wantRules:  []string{"DOMAIN-SUFFIX,youtube.com,DIRECT", "MATCH,vpn"},
		},
		{
			name: "Action reject → REJECT",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{Domain: []string{"ads.com"}, Action: "reject"},
				},
			},
			defaultTag: "vpn",
			wantRules:  []string{"DOMAIN,ads.com,REJECT"},
		},
		{
			name: "Outbound direct → DIRECT (built-in)",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{Domain: []string{"local.lan"}, Outbound: "direct"},
				},
			},
			defaultTag: "vpn",
			wantRules:  []string{"DOMAIN,local.lan,DIRECT"},
		},
		{
			name: "IPv4 CIDR → IP-CIDR + no-resolve",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{IPCIDR: []string{"10.0.0.0/8"}, Outbound: "direct"},
				},
			},
			defaultTag: "vpn",
			wantRules:  []string{"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve"},
		},
		{
			name: "IPv6 CIDR → IP-CIDR6",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{IPCIDR: []string{"2001:db8::/32"}, Outbound: "direct"},
				},
			},
			defaultTag: "vpn",
			wantRules:  []string{"IP-CIDR6,2001:db8::/32,DIRECT,no-resolve"},
		},
		{
			name: "Port → DST-PORT + range : → -",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{Port: []uint16{443}, PortRange: []string{"1000:2000"}, Outbound: "vpn"},
				},
			},
			defaultTag: "vpn",
			wantRules:  []string{"DST-PORT,443,vpn", "DST-PORT,1000-2000,vpn"},
		},
		{
			name: "Network → NETWORK",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{Network: "udp", Outbound: "direct"},
				},
			},
			defaultTag: "vpn",
			wantRules:  []string{"NETWORK,udp,DIRECT"},
		},
		{
			name: "RuleSet с .mrs URL → RULE-SET + provider",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{RuleSet: []string{"geosite-youtube"}, Outbound: "direct"},
				},
				RuleSets: []types.RuleSetRef{
					{
						Tag:    "geosite-youtube",
						Type:   "remote",
						Format: "binary",
						URL:    "https://example.com/youtube.mrs",
					},
				},
			},
			defaultTag:    "vpn",
			wantRules:     []string{"RULE-SET,geosite-youtube,DIRECT"},
			wantProviders: []string{"geosite-youtube"},
		},
		{
			name: "RuleSet с .srs URL → пропуск с warning",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{RuleSet: []string{"geosite-youtube"}, Outbound: "direct"},
				},
				RuleSets: []types.RuleSetRef{
					{
						Tag:    "geosite-youtube",
						Type:   "remote",
						Format: "binary",
						URL:    "https://example.com/youtube.srs",
					},
				},
			},
			defaultTag:     "vpn",
			wantWarningSub: []string{"geosite-youtube", ".mrs"},
		},
		{
			name: "Protocol matcher → пропуск с warning",
			rc: &types.RoutingConfig{
				Version: 1,
				Rules: []types.RouteRule{
					{Protocol: []string{"bittorrent"}, Outbound: "direct"},
				},
			},
			defaultTag:     "vpn",
			wantWarningSub: []string{"protocol"},
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
			wantWarningSub: []string{"sniff"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, providers, warnings := TranslateRoutingRules(tt.rc, tt.defaultTag)

			joined := strings.Join(rules, " | ")
			for _, want := range tt.wantRules {
				if !strings.Contains(joined, want) {
					t.Errorf("rule %q не найдено в: %s", want, joined)
				}
			}

			for _, wantTag := range tt.wantProviders {
				found := false
				for _, p := range providers {
					if p.Tag == wantTag {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("provider tag %q не найден в %v", wantTag, providers)
				}
			}

			joinedWarnings := strings.Join(warnings, " | ")
			for _, sub := range tt.wantWarningSub {
				if !strings.Contains(joinedWarnings, sub) {
					t.Errorf("warning подстрока %q не найдена в: %s", sub, joinedWarnings)
				}
			}
		})
	}
}

func TestParseInterval(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 86400},
		{"24h0m0s", 86400},
		{"1h", 3600},
		{"30m", 1800},
		{"45s", 45},
		{"1h30m", 5400},
		{"garbage", 86400},
	}
	for _, c := range cases {
		got := parseInterval(c.in)
		if got != c.want {
			t.Errorf("parseInterval(%q): got %d, want %d", c.in, got, c.want)
		}
	}
}

func TestNormalizeAction(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"direct", "DIRECT"},
		{"block", "REJECT"},
		{"vpn", "vpn"},
		{"my-proxy", "my-proxy"},
	}
	for _, c := range cases {
		got := normalizeAction(c.in)
		if got != c.want {
			t.Errorf("normalizeAction(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

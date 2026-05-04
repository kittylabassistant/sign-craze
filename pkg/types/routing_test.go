package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestRoutingConfig_Roundtrip проверяет полный Marshal/Unmarshal через JSON.
func TestRoutingConfig_Roundtrip(t *testing.T) {
	orig := RoutingConfig{
		Version: 1,
		Inbounds: []Inbound{
			{
				Tag:  "tun-in",
				Type: "tun",
				Settings: map[string]any{
					"interface_name": "sing",
					"mtu":            float64(1280),
				},
			},
		},
		Outbounds: []Outbound{
			{Tag: "direct", Type: "direct"},
			{
				Tag:    "proxy",
				Type:   "socks",
				Server: "127.0.0.1",
				Port:   1080,
			},
		},
		Rules: []RouteRule{
			{
				Domain:   []string{"example.com"},
				Outbound: "proxy",
			},
		},
		RuleSets: []RuleSetRef{
			{
				Tag:  "geosite-ru",
				Type: "remote",
				URL:  "https://example.com/geosite-ru.srs",
			},
		},
		Final: "direct",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got RoutingConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// проверяем ключевые поля
	if got.Version != orig.Version {
		t.Errorf("Version: got %d, want %d", got.Version, orig.Version)
	}
	if len(got.Inbounds) != 1 {
		t.Fatalf("Inbounds: got %d, want 1", len(got.Inbounds))
	}
	if got.Inbounds[0].Tag != "tun-in" {
		t.Errorf("Inbounds[0].Tag: got %q, want %q", got.Inbounds[0].Tag, "tun-in")
	}
	if got.Inbounds[0].Type != "tun" {
		t.Errorf("Inbounds[0].Type: got %q, want %q", got.Inbounds[0].Type, "tun")
	}
	if got.Inbounds[0].Settings["interface_name"] != "sing" {
		t.Errorf("Inbounds[0].Settings[interface_name]: got %v, want %q", got.Inbounds[0].Settings["interface_name"], "sing")
	}
	if len(got.Outbounds) != 2 {
		t.Fatalf("Outbounds: got %d, want 2", len(got.Outbounds))
	}
	if got.Final != "direct" {
		t.Errorf("Final: got %q, want %q", got.Final, "direct")
	}
	if len(got.RuleSets) != 1 || got.RuleSets[0].Tag != "geosite-ru" {
		t.Errorf("RuleSets неверны")
	}
}

// TestRouteRule_Validate — table-driven тесты валидации RouteRule.
func TestRouteRule_Validate(t *testing.T) {
	cases := []struct {
		name    string
		rule    RouteRule
		wantErr bool
		errSubs string
	}{
		{
			name:    "пустое action без outbound, но с conditions — ошибка",
			rule:    RouteRule{Action: "", Domain: []string{"example.com"}, Outbound: ""},
			wantErr: true,
			errSubs: "outbound обязателен",
		},
		{
			name:    "route action с conditions и outbound — ok",
			rule:    RouteRule{Action: "route", Domain: []string{"example.com"}, Outbound: "proxy"},
			wantErr: false,
		},
		{
			name:    "block action без outbound — ok",
			rule:    RouteRule{Action: "block", Domain: []string{"ads.example.com"}},
			wantErr: false,
		},
		{
			name:    "sniff action без outbound — ok",
			rule:    RouteRule{Action: "sniff"},
			wantErr: false,
		},
		{
			name:    "плохой CIDR — ошибка",
			rule:    RouteRule{Action: "block", IPCIDR: []string{"not-a-cidr"}},
			wantErr: true,
			errSubs: "ip_cidr",
		},
		{
			name:    "плохой source CIDR — ошибка",
			rule:    RouteRule{Action: "block", SourceIPCIDR: []string{"999.0.0.1/8"}},
			wantErr: true,
			errSubs: "source_ip_cidr",
		},
		{
			name:    "плохой regex — ошибка",
			rule:    RouteRule{Action: "block", DomainRegex: []string{"[invalid"}},
			wantErr: true,
			errSubs: "domain_regex",
		},
		{
			name:    "плохой port_range без двоеточия — ошибка",
			rule:    RouteRule{Action: "block", PortRange: []string{"8080"}},
			wantErr: true,
			errSubs: "port_range",
		},
		{
			name:    "плохой port_range from > to — ошибка",
			rule:    RouteRule{Action: "block", PortRange: []string{"9000:8000"}},
			wantErr: true,
			errSubs: "port_range",
		},
		{
			name:    "корректный port_range — ok",
			rule:    RouteRule{Action: "block", PortRange: []string{"8000:9000"}},
			wantErr: false,
		},
		{
			name:    "плохой network — ошибка",
			rule:    RouteRule{Network: "sctp", Outbound: "direct"},
			wantErr: true,
			errSubs: "network",
		},
		{
			name:    "пустое правило без conditions — ok (допустимо как final-catch)",
			rule:    RouteRule{Outbound: "direct"},
			wantErr: false,
		},
		{
			name:    "valid remote rule_set action — ok",
			rule:    RouteRule{Action: "route", RuleSet: []string{"geosite-ru"}, Outbound: "proxy"},
			wantErr: false,
		},
		{
			name:    "неизвестный action — ошибка",
			rule:    RouteRule{Action: "forward"},
			wantErr: true,
			errSubs: "action",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if tc.wantErr && err == nil {
				t.Error("ожидалась ошибка, получен nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("неожиданная ошибка: %v", err)
			}
			if tc.wantErr && err != nil && tc.errSubs != "" {
				if !strings.Contains(err.Error(), tc.errSubs) {
					t.Errorf("ошибка %q не содержит %q", err.Error(), tc.errSubs)
				}
			}
		})
	}
}

// TestInbound_MarshalSettings проверяет, что Settings мерджатся на top-level.
func TestInbound_MarshalSettings(t *testing.T) {
	in := Inbound{
		Tag:  "in",
		Type: "tun",
		Settings: map[string]any{
			"interface_name": "sing",
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}

	if raw["tag"] != "in" {
		t.Errorf("tag: got %v, want %q", raw["tag"], "in")
	}
	if raw["type"] != "tun" {
		t.Errorf("type: got %v, want %q", raw["type"], "tun")
	}
	if raw["interface_name"] != "sing" {
		t.Errorf("interface_name: got %v, want %q", raw["interface_name"], "sing")
	}
	// Settings не должен попасть как отдельный ключ
	if _, ok := raw["settings"]; ok {
		t.Error("\"settings\" не должен быть ключом верхнего уровня")
	}
}

// TestRuleSetRef_Validate — тесты валидации RuleSetRef.
func TestRuleSetRef_Validate(t *testing.T) {
	cases := []struct {
		name    string
		rs      RuleSetRef
		wantErr bool
		errSubs string
	}{
		{
			name:    "remote без URL — ошибка",
			rs:      RuleSetRef{Tag: "geo", Type: "remote"},
			wantErr: true,
			errSubs: "url обязателен",
		},
		{
			name:    "local без Path — ошибка",
			rs:      RuleSetRef{Tag: "geo", Type: "local"},
			wantErr: true,
			errSubs: "path обязателен",
		},
		{
			name:    "valid remote — ok",
			rs:      RuleSetRef{Tag: "geo", Type: "remote", URL: "https://example.com/geo.srs"},
			wantErr: false,
		},
		{
			name:    "valid local — ok",
			rs:      RuleSetRef{Tag: "geo", Type: "local", Path: "/opt/share/geo.srs"},
			wantErr: false,
		},
		{
			name:    "inline без extra — ok",
			rs:      RuleSetRef{Tag: "geo", Type: "inline"},
			wantErr: false,
		},
		{
			name:    "пустой tag — ошибка",
			rs:      RuleSetRef{Tag: "", Type: "remote", URL: "https://example.com"},
			wantErr: true,
			errSubs: "tag",
		},
		{
			name:    "неизвестный type — ошибка",
			rs:      RuleSetRef{Tag: "geo", Type: "ftp"},
			wantErr: true,
			errSubs: "type",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rs.Validate()
			if tc.wantErr && err == nil {
				t.Error("ожидалась ошибка, получен nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("неожиданная ошибка: %v", err)
			}
			if tc.wantErr && err != nil && tc.errSubs != "" {
				if !strings.Contains(err.Error(), tc.errSubs) {
					t.Errorf("ошибка %q не содержит %q", err.Error(), tc.errSubs)
				}
			}
		})
	}
}

// TestRoutingConfig_Validate_DuplicateTags проверяет, что дублирующиеся outbound-теги вызывают ошибку.
func TestRoutingConfig_Validate_DuplicateTags(t *testing.T) {
	c := &RoutingConfig{
		Version: 1,
		Outbounds: []Outbound{
			{Tag: "direct", Type: "direct"},
			{Tag: "direct", Type: "direct"},
		},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("ожидалась ошибка на дублирующийся тег, получен nil")
	}
	if !strings.Contains(err.Error(), "дублирующийся тег") {
		t.Errorf("ошибка %q не содержит %q", err.Error(), "дублирующийся тег")
	}
}

// TestRoutingConfig_Validate_FinalUnknown проверяет ошибку при неизвестном Final.
func TestRoutingConfig_Validate_FinalUnknown(t *testing.T) {
	c := &RoutingConfig{
		Version: 1,
		Final:   "missing",
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("ожидалась ошибка на неизвестный final, получен nil")
	}
	if !strings.Contains(err.Error(), "final") {
		t.Errorf("ошибка %q не содержит %q", err.Error(), "final")
	}
}

// TestRoutingConfig_Validate_FinalBuiltin проверяет, что Final="direct" валиден без outbound.
func TestRoutingConfig_Validate_FinalBuiltin(t *testing.T) {
	c := &RoutingConfig{
		Version: 1,
		Final:   "direct",
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Final=\"direct\" должен быть валиден, получено: %v", err)
	}

	c.Final = "block"
	if err := c.Validate(); err != nil {
		t.Errorf("Final=\"block\" должен быть валиден, получено: %v", err)
	}
}

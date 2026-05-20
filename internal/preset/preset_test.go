package preset

import (
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestList_ReturnsAllBuiltins(t *testing.T) {
	got := List()
	if len(got) != 8 {
		t.Fatalf("ожидалось 8 пресетов, получено %d", len(got))
	}
	wantNames := []string{
		"sign-craze-default", "block-ads", "ru-direct", "ru-direct-rest-vpn",
		"blocked-vpn", "discord-vpn", "torrents-direct", "block-bogon-udp",
	}
	nameSet := make(map[string]bool, len(got))
	for _, p := range got {
		nameSet[p.Name] = true
	}
	for _, name := range wantNames {
		if !nameSet[name] {
			t.Errorf("пресет %q отсутствует в List()", name)
		}
	}
}

func TestFind_Existing(t *testing.T) {
	p := Find("sign-craze-default")
	if p == nil {
		t.Fatal("Find(\"sign-craze-default\") вернул nil")
	}
	if Find("nonexistent") != nil {
		t.Error("Find(\"nonexistent\") должен вернуть nil")
	}
}

func TestFind_ReturnsPointerToBuiltin(t *testing.T) {
	// Find должен возвращать указатель на элемент BuiltinPresets, а не копию.
	for i := range BuiltinPresets {
		got := Find(BuiltinPresets[i].Name)
		if got != &BuiltinPresets[i] {
			t.Errorf("Find(%q): ожидался указатель на BuiltinPresets[%d], получен другой", BuiltinPresets[i].Name, i)
		}
	}
}

func TestResolveRuleSetURL_SRS(t *testing.T) {
	url, ok := ResolveRuleSetURL("geosite-youtube", core.GeoSRS)
	if !ok {
		t.Fatal("ok=false для geosite-youtube/GeoSRS")
	}
	if !strings.Contains(url, "SagerNet") {
		t.Errorf("ожидался SagerNet URL, получен %q", url)
	}
	if !strings.HasSuffix(url, ".srs") {
		t.Errorf("ожидался .srs URL, получен %q", url)
	}
}

func TestResolveRuleSetURL_MRS(t *testing.T) {
	url, ok := ResolveRuleSetURL("geoip-ru", core.GeoMRS)
	if !ok {
		t.Fatal("ok=false для geoip-ru/GeoMRS")
	}
	if !strings.Contains(url, "MetaCubeX") {
		t.Errorf("ожидался MetaCubeX URL, получен %q", url)
	}
	if !strings.HasSuffix(url, ".mrs") {
		t.Errorf("ожидался .mrs URL, получен %q", url)
	}
}

func TestResolveRuleSetURL_DAT_Geosite(t *testing.T) {
	url, ok := ResolveRuleSetURL("geosite-discord", core.GeoDAT)
	if !ok {
		t.Fatal("ok=false для geosite-discord/GeoDAT: xray должен транслировать через matcher")
	}
	if url != "" {
		t.Errorf("для GeoDAT URL должен быть пустым (translation в matcher), получен %q", url)
	}
}

func TestResolveRuleSetURL_DAT_Refilter_Unsupported(t *testing.T) {
	_, ok := ResolveRuleSetURL("refilter-blocked-domains", core.GeoDAT)
	if ok {
		t.Error("ok=true для refilter-blocked-domains/GeoDAT: xray не поддерживает refilter")
	}
}

func TestResolveRuleSetURL_UnknownTag(t *testing.T) {
	for _, format := range []core.GeoFormat{core.GeoSRS, core.GeoMRS, core.GeoDAT} {
		_, ok := ResolveRuleSetURL("unknown-tag-xyz", format)
		if ok {
			t.Errorf("ok=true для неизвестного тега (format=%v)", format)
		}
	}
}

func TestApplyToConfig_ReplaceMode_OverwritesRules(t *testing.T) {
	cfg := &types.RoutingConfig{
		Version: 1,
		Rules:   []types.RouteRule{{Network: "tcp", Outbound: "direct"}},
		Final:   "custom",
	}
	p := Find("torrents-direct")
	if p == nil {
		t.Fatal("пресет torrents-direct не найден")
	}
	ApplyToConfig(cfg, p, "vpn", core.GeoSRS, ModeReplace)
	if len(cfg.Rules) != 1 {
		t.Errorf("ожидалось 1 правило (только из пресета), получено %d", len(cfg.Rules))
	}
	if cfg.Rules[0].Protocol == nil || cfg.Rules[0].Protocol[0] != "bittorrent" {
		t.Errorf("ожидалось правило bittorrent, получено %+v", cfg.Rules[0])
	}
	// torrents-direct не задаёт Final → Final остаётся пустым (mode=replace не задаёт)
	// или остаётся как был (пресет не изменяет Final если p.Final == "")
	// т.к. p.Final == "" → блок if p.Final != "" не выполняется → cfg.Final не меняется
	if cfg.Final != "custom" {
		t.Errorf("torrents-direct не задаёт Final, ожидался cfg.Final=custom, получено %q", cfg.Final)
	}
}

func TestApplyToConfig_AddMode_AppendsRules(t *testing.T) {
	cfg := &types.RoutingConfig{
		Version: 1,
		Rules:   []types.RouteRule{{Network: "tcp", Outbound: "direct"}},
	}
	p := Find("torrents-direct")
	if p == nil {
		t.Fatal("пресет torrents-direct не найден")
	}
	ApplyToConfig(cfg, p, "vpn", core.GeoSRS, ModeAdd)
	if len(cfg.Rules) != 2 {
		t.Errorf("ожидалось 2 правила (existing+preset), получено %d", len(cfg.Rules))
	}
}

func TestApplyToConfig_VPNPlaceholderResolved(t *testing.T) {
	cfg := &types.RoutingConfig{Version: 1}
	p := Find("discord-vpn")
	if p == nil {
		t.Fatal("пресет discord-vpn не найден")
	}
	ApplyToConfig(cfg, p, "my-vless", core.GeoSRS, ModeReplace)
	if len(cfg.Rules) == 0 {
		t.Fatal("ожидалось как минимум 1 правило")
	}
	if cfg.Rules[0].Outbound != "my-vless" {
		t.Errorf("ожидался outbound=my-vless (резолв {vpn}), получено %q", cfg.Rules[0].Outbound)
	}
}

func TestApplyToConfig_RuleSetDedupe(t *testing.T) {
	cfg := &types.RoutingConfig{
		Version: 1,
		RuleSets: []types.RuleSetRef{
			{Tag: "geoip-ru", Type: "remote", URL: "http://example.com/geoip-ru.srs"},
		},
	}
	p := Find("ru-direct")
	if p == nil {
		t.Fatal("пресет ru-direct не найден")
	}
	ApplyToConfig(cfg, p, "", core.GeoSRS, ModeAdd)
	if len(cfg.RuleSets) != 1 {
		t.Errorf("ожидался 1 rule_set (без дублей), получено %d", len(cfg.RuleSets))
	}
}

func TestApplyToConfig_FinalPlaceholder_NotOverwriteExisting(t *testing.T) {
	cfg := &types.RoutingConfig{
		Version: 1,
		Final:   "custom-final",
	}
	p := Find("ru-direct-rest-vpn")
	if p == nil {
		t.Fatal("пресет ru-direct-rest-vpn не найден")
	}
	ApplyToConfig(cfg, p, "vpn", core.GeoSRS, ModeAdd)
	if cfg.Final != "custom-final" {
		t.Errorf("mode=add не должен перезаписывать явный Final, ожидался custom-final, получено %q", cfg.Final)
	}
}

func TestApplyToConfig_FinalPlaceholder_Replace_Overwrites(t *testing.T) {
	cfg := &types.RoutingConfig{
		Version: 1,
		Final:   "custom-final",
	}
	p := Find("ru-direct-rest-vpn")
	if p == nil {
		t.Fatal("пресет ru-direct-rest-vpn не найден")
	}
	ApplyToConfig(cfg, p, "vpn", core.GeoSRS, ModeReplace)
	if cfg.Final != "vpn" {
		t.Errorf("mode=replace должен перезаписать Final через {vpn}, ожидался vpn, получено %q", cfg.Final)
	}
}

func TestApplyToConfig_Refilter_XrayWarning(t *testing.T) {
	cfg := &types.RoutingConfig{Version: 1}
	p := Find("blocked-vpn")
	if p == nil {
		t.Fatal("пресет blocked-vpn не найден")
	}
	warnings := ApplyToConfig(cfg, p, "vpn", core.GeoDAT, ModeAdd)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "refilter-blocked-domains") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ожидался warning про refilter-blocked-domains для GeoDAT, получено warnings=%v", warnings)
	}
}

func TestUsesVPN(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"blocked-vpn", true},        // Rules содержит {vpn}
		{"ru-direct-rest-vpn", true}, // Final == {vpn}
		{"torrents-direct", false},   // нет {vpn}
		{"ru-direct", false},         // нет {vpn}
		{"block-ads", false},         // нет {vpn}
		{"block-bogon-udp", false},   // нет {vpn}
	}
	for _, tc := range cases {
		p := Find(tc.name)
		if p == nil {
			t.Fatalf("пресет %q не найден", tc.name)
		}
		got := UsesVPN(p)
		if got != tc.want {
			t.Errorf("UsesVPN(%q)=%v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}

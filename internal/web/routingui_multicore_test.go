package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/pkg/types"

	// Blank imports регистрируют адаптеры в core registry (sing-box / xray / mihomo).
	// Это аналог internal/cli/cores.go — тесты web используют те же реальные адаптеры.
	_ "github.com/kittylabassistant/sign-craze/internal/core/mihomo"
	_ "github.com/kittylabassistant/sign-craze/internal/core/xray"
	_ "github.com/kittylabassistant/sign-craze/internal/singbox"
)

// canonicalVLESS возвращает минимальный canonical VLESS Reality outbound для
// рендера xray/mihomo/sing-box. Server/Port — фиктивные но синтаксически валидные.
func canonicalVLESS(tag string) types.Outbound {
	return types.Outbound{
		Tag:      tag,
		Type:     "vless",
		Server:   "example.com",
		Port:     443,
		Protocol: types.ProtocolVLESS,
		Transport: &types.Transport{
			Kind: types.TransportTCP,
		},
		TLS: &types.TLSConfig{
			Enabled:    true,
			ServerName: "example.com",
			Reality: &types.RealityConfig{
				Enabled:   true,
				PublicKey: "publickeypublickeypublickeypublickeyABCDEF12",
				ShortID:   "0123456789abcdef",
			},
			UTLS: &types.UTLSConfig{Enabled: true, Fingerprint: "chrome"},
		},
		Proto: &types.ProtoOpts{
			UUID: "00000000-0000-0000-0000-000000000000",
		},
	}
}

// renderForCore возвращает Renderer-closure для конкретного ядра — используется
// в тестах per-core flow. Эмулирует логику cli/cmd_ui.go:uiRenderParams.
func renderForCore(c core.Core) func(context.Context, *types.RoutingConfig) ([]byte, error) {
	return func(_ context.Context, cfg *types.RoutingConfig) ([]byte, error) {
		var outs []types.Outbound
		var defaultTag string
		if cfg != nil && len(cfg.Outbounds) > 0 {
			outs = cfg.Outbounds
			defaultTag = cfg.Outbounds[0].Tag
		}
		return c.RenderConfig(types.CoreRenderParams{
			Mode:               types.ModeFull,
			Outbounds:          outs,
			RoutingConfig:      cfg,
			InboundMode:        "tproxy",
			DefaultOutboundTag: defaultTag,
			// Preview: как в uiRenderParams — предпросмотр не требует
			// локальных geo-ассетов (geosite.dat/geoip.dat).
			Preview: true,
		})
	}
}

// setupCoreDeps подключает Renderer/ConfigFormat/ActiveGeoFormat callbacks для
// активного ядра к существующему серверу. Тесты проверяют, что Server использует
// правильные closures для outputs.
func setupCoreDeps(t *testing.T, s *Server, c core.Core) {
	t.Helper()
	s.cfg.RoutingUI.Renderer = renderForCore(c)
	s.cfg.RoutingUI.ConfigFormat = func() core.ConfigFormat { return c.ConfigFormat() }
	s.cfg.RoutingUI.ActiveGeoFormat = func() core.GeoFormat { return c.GeoFormat() }
}

// TestE2E_FullFlow_Xray — full flow с xray Renderer.
// Проверяет: routing.json содержит правила/outbound, preview возвращает xray JSON
// с dokodemo-door inbound, translation RuleSet→matcher работает.
func TestE2E_FullFlow_Xray(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	setupCoreDeps(t, s, core.MustGet("xray"))

	// add outbound (vless-test — совпадает с DefaultOutboundTag из makeTestServerWithRouting)
	rec := do(s, authReq("POST", "/api/outbounds", canonicalVLESS("vless-test")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("add outbound: %d body=%s", rec.Code, rec.Body.String())
	}

	// apply preset с translation: ru-direct → RuleSet geoip-ru → translated в xray geoip:ru matcher
	rec = do(s, authReq("POST", "/api/presets/ru-direct/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply ru-direct: %d body=%s", rec.Code, rec.Body.String())
	}

	// preview → xray JSON с dokodemo-door inbound
	rec = do(s, authReq("GET", "/api/preview", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: ожидался application/json, получен %q", ct)
	}
	body := rec.Body.String()
	for _, sub := range []string{"dokodemo-door", "tproxy-in", "geoip:ru"} {
		if !strings.Contains(body, sub) {
			t.Errorf("preview xray не содержит %q. body: %s", sub, body)
		}
	}
}

// TestE2E_FullFlow_Mihomo — full flow с mihomo Renderer.
// Проверяет: preview возвращает YAML с tproxy-port + rule-providers (если .mrs URL).
func TestE2E_FullFlow_Mihomo(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	setupCoreDeps(t, s, core.MustGet("mihomo"))

	rec := do(s, authReq("POST", "/api/outbounds", canonicalVLESS("vless-test")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("add outbound: %d body=%s", rec.Code, rec.Body.String())
	}

	// apply preset с .mrs URL для mihomo
	rec = do(s, authReq("POST", "/api/presets/ru-direct/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply ru-direct: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(s, authReq("GET", "/api/preview", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("Content-Type: ожидался application/yaml, получен %q", ct)
	}
	body := rec.Body.String()
	for _, sub := range []string{"tproxy-port:", "proxies:", "rules:", "rule-providers:"} {
		if !strings.Contains(body, sub) {
			t.Errorf("preview mihomo не содержит %q. body:\n%s", sub, body)
		}
	}
	// Mihomo должен использовать .mrs URL из ru-direct preset
	if !strings.Contains(body, ".mrs") {
		t.Errorf("preview mihomo должен ссылаться на .mrs URL: %s", body)
	}
}

// TestE2E_PresetApply_PerCore — preset apply резолвит URL под формат активного ядра.
func TestE2E_PresetApply_PerCore(t *testing.T) {
	cases := []struct {
		name       string
		coreName   string
		wantSubstr string // подстрока в RuleSets[0].URL после apply
	}{
		{"sing-box", "sing-box", ".srs"},
		{"mihomo", "mihomo", ".mrs"},
		{"xray", "xray", ""}, // xray — URL пустой, translation в matcher
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := makeTestServerWithRouting(t)
			c := core.MustGet(tc.coreName)
			s.cfg.RoutingUI.ActiveGeoFormat = func() core.GeoFormat { return c.GeoFormat() }

			rec := do(s, authReq("POST", "/api/presets/ru-direct/apply", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("apply: %d body=%s", rec.Code, rec.Body.String())
			}

			rec = do(s, authReq("GET", "/api/rule_sets", nil))
			var rss []types.RuleSetRef
			_ = json.Unmarshal(rec.Body.Bytes(), &rss)
			if tc.wantSubstr == "" {
				// xray: rule_set entries не добавляются (translation в matcher).
				if len(rss) != 0 {
					t.Errorf("xray: ожидался 0 rule_sets (translation в matcher), получено %+v", rss)
				}
				return
			}
			if len(rss) != 1 || rss[0].Tag != "geoip-ru" {
				t.Fatalf("ожидался 1 rule_set geoip-ru, получено %+v", rss)
			}
			if !strings.Contains(rss[0].URL, tc.wantSubstr) {
				t.Errorf("URL не содержит %q: %s", tc.wantSubstr, rss[0].URL)
			}
		})
	}
}

// TestE2E_Validate_Warnings_XrayTunInbound — TUN inbound + xray → warning через Validator.
func TestE2E_Validate_Warnings_XrayTunInbound(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	xrayCore := core.MustGet("xray")
	setupCoreDeps(t, s, xrayCore)
	// Validator — упрощённый: только сбор compat-warnings без CheckConfig.
	s.cfg.RoutingUI.Validator = func(ctx context.Context, cfg *types.RoutingConfig) ([]string, error) {
		return collectCompatWarningsForTest(xrayCore, cfg), nil
	}

	// add TUN inbound (нелегально для xray)
	rec := do(s, authReq("POST", "/api/inbounds", types.Inbound{
		Tag: "tun-in", Type: "tun",
		Settings: map[string]any{"address": []string{"172.19.0.1/30"}},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("add tun inbound: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(s, authReq("POST", "/api/validate", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("validate: %d body=%s", rec.Code, rec.Body.String())
	}
	var vr validateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &vr)
	if !vr.OK {
		t.Errorf("validate ok=false (но TUN на xray должен быть warning, не fatal): %v", vr.Errors)
	}
	if len(vr.Warnings) == 0 || !strings.Contains(strings.Join(vr.Warnings, " "), "TUN") {
		t.Errorf("ожидался warning про TUN inbound, получено: %v", vr.Warnings)
	}
}

// collectCompatWarningsForTest дублирует логику cli/cmd_ui_render.go:collectCompatWarnings
// для использования в web-test без зависимости на cli пакет (тогда бы был cycle).
func collectCompatWarningsForTest(c core.Core, cfg *types.RoutingConfig) []string {
	if cfg == nil {
		return nil
	}
	var warnings []string
	if c.Name() != "sing-box" {
		for _, ib := range cfg.Inbounds {
			if ib.Type == "tun" {
				warnings = append(warnings,
					"inbound "+ib.Tag+" (type=tun): "+c.Name()+" не поддерживает TUN, inbound будет проигнорирован")
			}
		}
	}
	return warnings
}

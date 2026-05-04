package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestE2E_FullFlow — сквозной flow редактирования routing.json через REST:
// state → preset apply → add outbound → add rule → reorder → validate → preview → apply.
// Использует реальный Renderer (singbox.Render) и проверяет что routing.json
// на диске содержит ожидаемое после серии операций.
func TestE2E_FullFlow(t *testing.T) {
	s, pw, routingPath := makeTestServerWithRouting(t)

	// подключаем реальный renderer (как в cli/cmd_ui.go)
	s.cfg.RoutingUI.Renderer = func(ctx context.Context, cfg *types.RoutingConfig) ([]byte, error) {
		params := singbox.DefaultConfigParams()
		params.RoutingConfig = cfg
		if len(cfg.Outbounds) > 0 {
			params.DefaultOutboundTag = cfg.Outbounds[0].Tag
		} else {
			params.DefaultOutboundTag = "direct"
		}
		return singbox.Render(params)
	}

	// 1. изначальный state — пустой config с version=1
	rec := do(s, authReq("GET", "/api/state", pw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/state: %d", rec.Code)
	}

	// 2. apply preset block-bogon-udp — добавляет 1 правило, без rule_sets
	rec = do(s, authReq("POST", "/api/presets/block-bogon-udp/apply", pw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply preset: %d body=%s", rec.Code, rec.Body.String())
	}

	// 3. apply preset ru-direct — добавляет 1 правило + 1 rule_set
	rec = do(s, authReq("POST", "/api/presets/ru-direct/apply", pw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply ru-direct: %d", rec.Code)
	}

	// 4. add VLESS outbound
	rec = do(s, authReq("POST", "/api/outbounds", pw, types.Outbound{
		Tag: "vless-out", Type: "vless", Server: "example.com", Port: 443,
		Settings: map[string]any{"uuid": "00000000-0000-0000-0000-000000000000"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("add outbound: %d body=%s", rec.Code, rec.Body.String())
	}

	// 5. add rule "udp 50000-50030 → vless-out"
	rec = do(s, authReq("POST", "/api/rules", pw, types.RouteRule{
		Network: "udp", PortRange: []string{"50000:50030"}, Outbound: "vless-out",
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("add rule: %d body=%s", rec.Code, rec.Body.String())
	}

	// 6. validate с body=null (текущий routing.json) → ok
	rec = do(s, authReq("POST", "/api/validate", pw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("validate: %d body=%s", rec.Code, rec.Body.String())
	}
	var vr validateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &vr)
	if !vr.OK {
		t.Fatalf("validate ok=false errors=%v", vr.Errors)
	}

	// 7. preview → должен вернуть валидный sing-box JSON
	rec = do(s, authReq("GET", "/api/preview", pw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d body=%s", rec.Code, rec.Body.String())
	}
	var preview map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("preview невалидный JSON: %v", err)
	}
	if _, ok := preview["route"]; !ok {
		t.Errorf("preview не содержит route секцию")
	}

	// 8. reorder rules (3 правила: idx 0=block-bogon, 1=ru-direct, 2=vless-50k)
	// перенести vless-правило в начало (idx 2 → 0)
	rec = do(s, authReq("POST", "/api/rules/reorder", pw, map[string]int{"from": 2, "to": 0}))
	if rec.Code != http.StatusOK {
		t.Fatalf("reorder: %d", rec.Code)
	}

	// 9. apply → routing.json должен быть на диске + needs_restart=true
	rec = do(s, authReq("POST", "/api/apply", pw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "needs_restart") {
		t.Errorf("apply response не содержит needs_restart")
	}

	// 10. assert: routing.json существует и содержит ожидаемое
	data, err := os.ReadFile(routingPath)
	if err != nil {
		t.Fatalf("routing.json не существует: %v", err)
	}
	var saved types.RoutingConfig
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("routing.json невалидный: %v", err)
	}
	if len(saved.Rules) != 3 {
		t.Errorf("ожидалось 3 правила в routing.json, %d", len(saved.Rules))
	}
	if saved.Rules[0].Outbound != "vless-out" {
		t.Errorf("после reorder первое правило должно быть vless-out, %v", saved.Rules[0])
	}
	if len(saved.Outbounds) != 1 || saved.Outbounds[0].Tag != "vless-out" {
		t.Errorf("ожидался 1 outbound vless-out, %v", saved.Outbounds)
	}
	if len(saved.RuleSets) != 1 || saved.RuleSets[0].Tag != "geoip-ru" {
		t.Errorf("ожидался 1 rule_set geoip-ru, %v", saved.RuleSets)
	}

	// 11. validate финальной конфигурации — должна быть OK
	if err := saved.Validate(); err != nil {
		t.Errorf("итоговый routing.json невалиден: %v", err)
	}
}

// TestE2E_DeleteFlow — проверяет цепочку delete operations.
func TestE2E_DeleteFlow(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)

	// add 2 outbounds
	_ = do(s, authReq("POST", "/api/outbounds", pw, types.Outbound{Tag: "a", Type: "direct"}))
	_ = do(s, authReq("POST", "/api/outbounds", pw, types.Outbound{Tag: "b", Type: "block"}))

	// delete one
	rec := do(s, authReq("DELETE", "/api/outbounds/a", pw, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}

	// list → only b
	rec = do(s, authReq("GET", "/api/outbounds", pw, nil))
	var list []types.Outbound
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Tag != "b" {
		t.Errorf("после delete: %+v", list)
	}

	// delete несуществующего → 404
	rec = do(s, authReq("DELETE", "/api/outbounds/nonexistent", pw, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("delete nonexistent: %d", rec.Code)
	}
}

// TestE2E_CustomRuleSet — добавление кастомного rule_set с пользовательским URL.
// Покрывает кейс "пользователь хочет SRS не из встроенных пресетов".
func TestE2E_CustomRuleSet(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)

	rec := do(s, authReq("POST", "/api/rule_sets", pw, types.RuleSetRef{
		Tag: "my-custom", Type: "remote", Format: "binary",
		URL:            "https://example.com/my.srs",
		DownloadDetour: "direct",
		UpdateInterval: "12h0m0s",
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("add custom rule_set: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(s, authReq("GET", "/api/rule_sets", pw, nil))
	var rss []types.RuleSetRef
	_ = json.Unmarshal(rec.Body.Bytes(), &rss)
	if len(rss) != 1 || rss[0].Tag != "my-custom" {
		t.Errorf("custom rule_set не сохранён: %v", rss)
	}

	// невалидный (remote без URL) → 400
	rec = do(s, authReq("POST", "/api/rule_sets", pw, types.RuleSetRef{
		Tag: "invalid", Type: "remote",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400 для remote без URL, %d", rec.Code)
	}
}

// TestE2E_SniffBaseRule_InRendered — проверяет что sniff base-rule
// присутствует в rendered config (необходим для protocol matching: bittorrent, ssh, tls).
func TestE2E_SniffBaseRule_InRendered(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)
	s.cfg.RoutingUI.Renderer = func(ctx context.Context, cfg *types.RoutingConfig) ([]byte, error) {
		params := singbox.DefaultConfigParams()
		params.RoutingConfig = cfg
		params.DefaultOutboundTag = "direct"
		return singbox.Render(params)
	}

	// добавляем outbound для валидности
	_ = do(s, authReq("POST", "/api/outbounds", pw, types.Outbound{Tag: "out", Type: "direct"}))

	rec := do(s, authReq("GET", "/api/preview", pw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"action":"sniff"`) {
		t.Error("preview не содержит sniff base-rule, protocol matching не будет работать")
	}
	if !strings.Contains(body, `"action":"hijack-dns"`) {
		t.Error("preview не содержит hijack-dns base-rule")
	}
}

// TestE2E_PresetMultiApply — apply нескольких пресетов подряд, проверка дедупа.
func TestE2E_PresetMultiApply(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)

	for _, name := range []string{"block-ads", "ru-direct", "discord-vpn", "torrents-direct"} {
		rec := do(s, authReq("POST", "/api/presets/"+name+"/apply", pw, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("apply %s: %d body=%s", name, rec.Code, rec.Body.String())
		}
	}

	// проверяем итоговый state
	rec := do(s, authReq("GET", "/api/rules", pw, nil))
	var rules []types.RouteRule
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if len(rules) != 4 {
		t.Errorf("ожидалось 4 правила (по одному на пресет), %d", len(rules))
	}

	rec = do(s, authReq("GET", "/api/rule_sets", pw, nil))
	var rss []types.RuleSetRef
	_ = json.Unmarshal(rec.Body.Bytes(), &rss)
	// block-ads + ru-direct + discord-vpn = 3 rule_sets; torrents-direct без rule_set
	if len(rss) != 3 {
		t.Errorf("ожидалось 3 rule_sets, %d", len(rss))
	}
}

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/routing"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// makeTestServerWithRouting — Server с настроенным RoutingUI deps на временный routing.json.
// DefaultOutboundTag возвращает фиксированный "vless-test" — пресеты с {vpn}-placeholder
// успешно резолвятся в детерминированный тег без зависимости от state.
func makeTestServerWithRouting(t *testing.T) (*Server, string) {
	t.Helper()
	s := makeTestServer(t)
	dir := t.TempDir()
	routingPath := filepath.Join(dir, "routing.json")
	s.cfg.RoutingUI = &RoutingUIDeps{
		RoutingPath:        routingPath,
		DefaultOutboundTag: func() string { return "vless-test" },
	}
	return s, routingPath
}

// authReq — POST/PUT/DELETE с Origin == Host (доступ ограничен LAN-биндингом
// и WAN INPUT DROP на firewall-уровне, Basic Auth в проекте нет).
func authReq(method, path string, body any) *http.Request {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	req.Host = "127.0.0.1:9092"
	req.Header.Set("Origin", "http://127.0.0.1:9092")
	return req
}

// do executes request through full middleware chain.
func do(s *Server, req *http.Request) *httptest.ResponseRecorder {
	h := servingMux(s)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ===== Outbounds CRUD =====

func TestOutbounds_AddListUpdateDelete(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// add
	rec := do(s, authReq("POST", "/api/outbounds", types.Outbound{
		Tag: "vless-out", Type: "vless", Server: "1.1.1.1", Port: 443,
		Settings: map[string]any{"uuid": "00000000-0000-0000-0000-000000000000"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/outbounds: %d body=%s", rec.Code, rec.Body.String())
	}

	// list
	rec = do(s, authReq("GET", "/api/outbounds", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d", rec.Code)
	}
	var list []types.Outbound
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Tag != "vless-out" {
		t.Fatalf("список: %+v", list)
	}

	// duplicate add → 409
	rec = do(s, authReq("POST", "/api/outbounds", types.Outbound{Tag: "vless-out", Type: "direct"}))
	if rec.Code != http.StatusConflict {
		t.Errorf("дубликат: %d", rec.Code)
	}

	// update
	rec = do(s, authReq("PUT", "/api/outbounds/vless-out", types.Outbound{
		Tag: "vless-out", Type: "vless", Server: "2.2.2.2", Port: 443,
		Settings: map[string]any{"uuid": "11111111-1111-1111-1111-111111111111"},
	}))
	if rec.Code != http.StatusOK {
		t.Errorf("PUT: %d body=%s", rec.Code, rec.Body.String())
	}

	// delete
	rec = do(s, authReq("DELETE", "/api/outbounds/vless-out", nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("DELETE: %d", rec.Code)
	}
}

func TestOutbounds_AddInvalid(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("POST", "/api/outbounds", types.Outbound{Tag: "", Type: "vless"}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400, получен %d", rec.Code)
	}
}

// ===== Inbounds CRUD =====

func TestInbounds_AddList(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("POST", "/api/inbounds", types.Inbound{
		Tag: "tproxy-in", Type: "tproxy",
		Settings: map[string]any{"listen": "::", "listen_port": 7895},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST: %d body=%s", rec.Code, rec.Body.String())
	}
	rec = do(s, authReq("GET", "/api/inbounds", nil))
	if !strings.Contains(rec.Body.String(), "tproxy-in") {
		t.Errorf("inbound не найден: %s", rec.Body.String())
	}
}

// ===== Rules CRUD + Reorder =====

func TestRules_AddListReorderDelete(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// add 3 rules
	for i, action := range []string{"block", "block", "block"} {
		_ = i
		rec := do(s, authReq("POST", "/api/rules", types.RouteRule{
			Network: "udp", Port: []uint16{uint16(135 + i)}, Action: action,
		}))
		if rec.Code != http.StatusCreated {
			t.Fatalf("add rule %d: %d body=%s", i, rec.Code, rec.Body.String())
		}
	}

	// list
	rec := do(s, authReq("GET", "/api/rules", nil))
	var rules []types.RouteRule
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if len(rules) != 3 {
		t.Fatalf("ожидалось 3 правила, %d", len(rules))
	}

	// reorder: переместить idx=2 на idx=0
	rec = do(s, authReq("POST", "/api/rules/reorder", map[string]int{"from": 2, "to": 0}))
	if rec.Code != http.StatusOK {
		t.Fatalf("reorder: %d body=%s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if rules[0].Port[0] != 137 {
		t.Errorf("после reorder первое правило должно быть port=137, %v", rules[0])
	}

	// delete
	rec = do(s, authReq("DELETE", "/api/rules/0", nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("DELETE: %d", rec.Code)
	}
}

func TestRules_AddInvalidCIDR(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("POST", "/api/rules", types.RouteRule{
		IPCIDR: []string{"not-a-cidr"}, Outbound: "direct",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400, получен %d", rec.Code)
	}
}

// ===== Presets =====

func TestPresets_List(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("GET", "/api/presets", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/presets: %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"sign-craze-default", "block-ads", "ru-direct", "ru-direct-rest-vpn", "blocked-vpn", "discord-vpn", "torrents-direct", "block-bogon-udp"} {
		if !strings.Contains(body, want) {
			t.Errorf("preset %q отсутствует в response", want)
		}
	}
}

func TestPresets_Apply_BlockBogonUDP(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	rec := do(s, authReq("POST", "/api/presets/block-bogon-udp/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d body=%s", rec.Code, rec.Body.String())
	}
	rec = do(s, authReq("GET", "/api/rules", nil))
	var rules []types.RouteRule
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if len(rules) == 0 || rules[0].Network != "udp" {
		t.Errorf("preset не применился: %+v", rules)
	}
}

func TestPresets_Apply_RuleSet_NoDuplicate(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	// apply ru-direct дважды → rule_set не дублируется
	_ = do(s, authReq("POST", "/api/presets/ru-direct/apply", nil))
	_ = do(s, authReq("POST", "/api/presets/ru-direct/apply", nil))

	rec := do(s, authReq("GET", "/api/rule_sets", nil))
	var rss []types.RuleSetRef
	_ = json.Unmarshal(rec.Body.Bytes(), &rss)
	if len(rss) != 1 {
		t.Errorf("ожидался 1 rule_set, %d", len(rss))
	}
}

func TestPresets_Apply_NotFound(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("POST", "/api/presets/nonexistent/apply", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("ожидался 404, получен %d", rec.Code)
	}
}

// TestPresets_Apply_VPNPlaceholder_ResolvedToDefaultOutbound проверяет, что
// пресеты с {vpn}-placeholder получают актуальный тег VPN-outbound.
func TestPresets_Apply_VPNPlaceholder_ResolvedToDefaultOutbound(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	rec := do(s, authReq("POST", "/api/presets/discord-vpn/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply discord-vpn: %d body=%s", rec.Code, rec.Body.String())
	}
	rec = do(s, authReq("GET", "/api/rules", nil))
	var rules []types.RouteRule
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if len(rules) != 1 || rules[0].Outbound != "vless-test" {
		t.Errorf("ожидалось 1 правило с outbound=vless-test, получено %+v", rules)
	}
}

// TestPresets_Apply_VPNRequired_NoOutbound_412 проверяет, что без VPN-outbound
// preset с {vpn}-placeholder возвращает 412 Precondition Failed.
func TestPresets_Apply_VPNRequired_NoOutbound_412(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	// Снимаем DefaultOutboundTag — имитируем отсутствие VPN-outbound.
	s.cfg.RoutingUI.DefaultOutboundTag = func() string { return "" }

	rec := do(s, authReq("POST", "/api/presets/blocked-vpn/apply", nil))
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("ожидался 412 без VPN-outbound, получен %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestPresets_Apply_RuDirectRestVPN проверяет: geoip-ru → direct (правило),
// final → vpn-тег (резолв {vpn}), 1 rule_set добавлен.
func TestPresets_Apply_RuDirectRestVPN(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// В реальном flow BootstrapFromState переносит state.Outbounds в
	// routing.json.Outbounds. В тестах симулируем это вручную, иначе
	// RoutingConfig.Validate отклонит Final="vless-test" — outbound с таким
	// тегом отсутствует в cfg.Outbounds.
	rec := do(s, authReq("POST", "/api/outbounds", types.Outbound{
		Tag: "vless-test", Type: "vless", Server: "1.1.1.1", Port: 443,
		Settings: map[string]any{"uuid": "00000000-0000-0000-0000-000000000000"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup outbound: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(s, authReq("POST", "/api/presets/ru-direct-rest-vpn/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply ru-direct-rest-vpn: %d body=%s", rec.Code, rec.Body.String())
	}

	if got := fetchFinal(t, s); got != "vless-test" {
		t.Errorf("ожидался final=vless-test (резолв {vpn}), получили %q", got)
	}

	rec = do(s, authReq("GET", "/api/rules", nil))
	var rules []types.RouteRule
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if len(rules) != 1 || rules[0].Outbound != "direct" || len(rules[0].RuleSet) != 1 || rules[0].RuleSet[0] != "geoip-ru" {
		t.Errorf("ожидалось 1 правило geoip-ru→direct, получено %+v", rules)
	}

	rec = do(s, authReq("GET", "/api/rule_sets", nil))
	var rss []types.RuleSetRef
	_ = json.Unmarshal(rec.Body.Bytes(), &rss)
	if len(rss) != 1 || rss[0].Tag != "geoip-ru" {
		t.Errorf("ожидался 1 rule_set geoip-ru, получено %+v", rss)
	}
}

// TestPresets_Apply_RuDirectRestVPN_NoOutbound_412 — без VPN-outbound
// пресет возвращает 412 (Final={vpn} требует резолва).
func TestPresets_Apply_RuDirectRestVPN_NoOutbound_412(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	s.cfg.RoutingUI.DefaultOutboundTag = func() string { return "" }

	rec := do(s, authReq("POST", "/api/presets/ru-direct-rest-vpn/apply", nil))
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("ожидался 412, получен %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestPresets_Apply_RuDirectRestVPN_FinalDoesNotOverwrite — если оператор уже
// явно задал Final, пресет не должен его перезаписывать.
func TestPresets_Apply_RuDirectRestVPN_FinalDoesNotOverwrite(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// outbound для резолва {vpn} (см. комментарий в TestPresets_Apply_RuDirectRestVPN).
	rec := do(s, authReq("POST", "/api/outbounds", types.Outbound{
		Tag: "vless-test", Type: "vless", Server: "1.1.1.1", Port: 443,
		Settings: map[string]any{"uuid": "00000000-0000-0000-0000-000000000000"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup outbound: %d body=%s", rec.Code, rec.Body.String())
	}

	// оператор явно ставит final=direct
	rec = do(s, authReq("PUT", "/api/final", map[string]string{"final": "direct"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT final direct setup: %d", rec.Code)
	}

	rec = do(s, authReq("POST", "/api/presets/ru-direct-rest-vpn/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d body=%s", rec.Code, rec.Body.String())
	}

	if got := fetchFinal(t, s); got != "direct" {
		t.Errorf("явный final=direct должен сохраниться, получили %q", got)
	}
}

// TestPresets_Apply_SignCrazeDefault_FullFlow проверяет, что preset
// sign-craze-default корректно применяется: 4 правила, 3 rule_set, final=direct.
func TestPresets_Apply_SignCrazeDefault_FullFlow(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	rec := do(s, authReq("POST", "/api/presets/sign-craze-default/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply sign-craze-default: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(s, authReq("GET", "/api/state", nil))
	var state struct {
		Config types.RoutingConfig `json:"config"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &state)

	if state.Config.Final != "direct" {
		t.Errorf("Final: ожидался %q, получен %q", "direct", state.Config.Final)
	}
	if len(state.Config.Rules) != 4 {
		t.Fatalf("ожидалось 4 правила, получено %d", len(state.Config.Rules))
	}
	// Проверка резолва {vpn} → vless-test в blocked-rule.
	var blocked *types.RouteRule
	for i, r := range state.Config.Rules {
		for _, rs := range r.RuleSet {
			if rs == "refilter-blocked-domains" {
				blocked = &state.Config.Rules[i]
			}
		}
	}
	if blocked == nil {
		t.Fatal("правило refilter-blocked-domains не найдено")
	}
	if blocked.Outbound != "vless-test" {
		t.Errorf("blocked.Outbound: ожидался vless-test, получен %q", blocked.Outbound)
	}
	if len(state.Config.RuleSets) != 3 {
		t.Errorf("ожидалось 3 rule_set, получено %d", len(state.Config.RuleSets))
	}
}

// TestPresets_Apply_Final_DoesNotOverwriteExplicit проверяет, что Final пресета
// не перезаписывает уже выставленный оператором Final.
func TestPresets_Apply_Final_DoesNotOverwriteExplicit(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// Установим Final="vless-test" вручную через сохранённый routing.json.
	cfg := types.RoutingConfig{
		Version:   1,
		Outbounds: []types.Outbound{{Tag: "vless-test", Type: "direct"}},
		Final:     "vless-test",
	}
	if err := s.saveRoutingConfig(&cfg); err != nil {
		t.Fatalf("saveRoutingConfig: %v", err)
	}

	rec := do(s, authReq("POST", "/api/presets/sign-craze-default/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(s, authReq("GET", "/api/state", nil))
	var st struct {
		Config types.RoutingConfig `json:"config"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Config.Final != "vless-test" {
		t.Errorf("Final был перезаписан: ожидался vless-test, получен %q", st.Config.Final)
	}
}

// ===== Validate =====

func TestValidate_OK(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	cfg := types.RoutingConfig{
		Version: 1,
		Outbounds: []types.Outbound{
			{Tag: "out", Type: "direct"},
		},
	}
	rec := do(s, authReq("POST", "/api/validate", cfg))
	if rec.Code != http.StatusOK {
		t.Fatalf("validate: %d body=%s", rec.Code, rec.Body.String())
	}
	var resp validateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.OK {
		t.Errorf("ожидался ok=true, errors=%v", resp.Errors)
	}
}

func TestValidate_Invalid(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	// Конфиг с некорректным action в правиле — ошибка валидации не в version,
	// поэтому авто-патч version не скрывает проблему.
	cfg := types.RoutingConfig{
		Version: 1,
		Rules:   []types.RouteRule{{Action: "invalid-action"}},
	}
	rec := do(s, authReq("POST", "/api/validate", cfg))
	if rec.Code != http.StatusOK {
		t.Fatalf("validate: %d", rec.Code)
	}
	var resp validateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.OK {
		t.Errorf("ожидался ok=false")
	}
	if len(resp.Errors) == 0 {
		t.Errorf("ожидались errors")
	}
}

// TestValidate_VersionZeroAutoPatch проверяет, что version=0 в теле запроса
// автоматически патчится до SchemaVersion и валидация проходит успешно.
func TestValidate_VersionZeroAutoPatch(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	cfg := types.RoutingConfig{
		Version: 0, // должен быть автоматически поднят до SchemaVersion
	}
	rec := do(s, authReq("POST", "/api/validate", cfg))
	if rec.Code != http.StatusOK {
		t.Fatalf("validate: %d body=%s", rec.Code, rec.Body.String())
	}
	var resp validateResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.OK {
		t.Errorf("ожидался ok=true после авто-патча version, errors=%v", resp.Errors)
	}
}

// ===== Apply =====

func TestApply_NeedsRestart(t *testing.T) {
	s, routingPath := makeTestServerWithRouting(t)

	// add outbound first
	_ = do(s, authReq("POST", "/api/outbounds", types.Outbound{Tag: "out", Type: "direct"}))

	rec := do(s, authReq("POST", "/api/apply", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "needs_restart") {
		t.Errorf("response не содержит needs_restart: %s", rec.Body.String())
	}

	// файл должен быть создан
	if _, err := os.Stat(routingPath); err != nil {
		t.Errorf("routing.json не создан: %v", err)
	}
}

// ===== Final outbound =====

// fetchFinal — GET /api/state и вернуть config.final.
func fetchFinal(t *testing.T, s *Server) string {
	t.Helper()
	rec := do(s, authReq("GET", "/api/state", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/state: %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Config types.RoutingConfig `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	return resp.Config.Final
}

func TestFinal_SetBuiltinDirect(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	rec := do(s, authReq("PUT", "/api/final", map[string]string{"final": "direct"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/final direct: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := fetchFinal(t, s); got != "direct" {
		t.Errorf("ожидался final=direct, получили %q", got)
	}
}

func TestFinal_SetBuiltinBlock(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	rec := do(s, authReq("PUT", "/api/final", map[string]string{"final": "block"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/final block: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := fetchFinal(t, s); got != "block" {
		t.Errorf("ожидался final=block, получили %q", got)
	}
}

func TestFinal_SetUserOutbound(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// заранее добавляем outbound, на который укажет Final
	rec := do(s, authReq("POST", "/api/outbounds", types.Outbound{
		Tag: "proxy-x", Type: "vless", Server: "1.1.1.1", Port: 443,
		Settings: map[string]any{"uuid": "00000000-0000-0000-0000-000000000000"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup outbound: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(s, authReq("PUT", "/api/final", map[string]string{"final": "proxy-x"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/final proxy-x: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := fetchFinal(t, s); got != "proxy-x" {
		t.Errorf("ожидался final=proxy-x, получили %q", got)
	}
}

func TestFinal_SetInvalidTag(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	rec := do(s, authReq("PUT", "/api/final", map[string]string{"final": "ghost"}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ожидался 400 на несуществующий tag, получен %d body=%s", rec.Code, rec.Body.String())
	}
	if got := fetchFinal(t, s); got != "" {
		t.Errorf("после ошибочного PUT final должен остаться пустым, получили %q", got)
	}
}

func TestFinal_SetEmptyClears(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// сначала ставим direct
	rec := do(s, authReq("PUT", "/api/final", map[string]string{"final": "direct"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("setup PUT direct: %d body=%s", rec.Code, rec.Body.String())
	}

	// затем очищаем
	rec = do(s, authReq("PUT", "/api/final", map[string]string{"final": ""}))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT empty: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := fetchFinal(t, s); got != "" {
		t.Errorf("ожидался пустой final после очистки, получили %q", got)
	}
}

// ===== Presets Preview (AS-IS / TO-BE без записи на диск) =====

// decodePresetResp декодирует ответ preview/apply в presetApplyResponse-like форму.
func decodePresetResp(t *testing.T, body []byte) (*types.RoutingConfig, []string) {
	t.Helper()
	var resp struct {
		Config   *types.RoutingConfig `json:"config"`
		Warnings []string             `json:"warnings,omitempty"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode preview resp: %v body=%s", err, body)
	}
	return resp.Config, resp.Warnings
}

// TestPresetsPreview_AddMode_DoesNotWriteDisk — preview не создаёт routing.json.
func TestPresetsPreview_AddMode_DoesNotWriteDisk(t *testing.T) {
	s, routingPath := makeTestServerWithRouting(t)

	rec := do(s, authReq("POST", "/api/presets/torrents-direct/preview", map[string]string{"mode": "add"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(routingPath); !os.IsNotExist(err) {
		t.Errorf("routing.json не должен быть создан preview-вызовом (err=%v)", err)
	}
}

// TestPresetsPreview_AddMode_ReturnsAppendedConfig — add-режим: правила добавляются к существующим.
func TestPresetsPreview_AddMode_ReturnsAppendedConfig(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// Подготовим существующее правило в routing.json.
	cfg := types.RoutingConfig{
		Version: 1,
		Rules: []types.RouteRule{
			{Network: "tcp", Port: []uint16{22}, Action: "block"},
		},
	}
	if err := s.saveRoutingConfig(&cfg); err != nil {
		t.Fatalf("saveRoutingConfig: %v", err)
	}

	rec := do(s, authReq("POST", "/api/presets/torrents-direct/preview", map[string]string{"mode": "add"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d body=%s", rec.Code, rec.Body.String())
	}
	gotCfg, _ := decodePresetResp(t, rec.Body.Bytes())
	if gotCfg == nil || len(gotCfg.Rules) != 2 {
		t.Fatalf("ожидалось 2 правила (existing+preset), получено %+v", gotCfg)
	}
	if gotCfg.Rules[0].Port[0] != 22 {
		t.Errorf("первое правило должно остаться существующим (port=22), %+v", gotCfg.Rules[0])
	}
	if len(gotCfg.Rules[1].Protocol) == 0 || gotCfg.Rules[1].Protocol[0] != "bittorrent" {
		t.Errorf("второе правило — preset (bittorrent), %+v", gotCfg.Rules[1])
	}
}

// TestPresetsPreview_ReplaceMode_ClearsRulesOnly — replace очищает Rules, RuleSets остаются.
func TestPresetsPreview_ReplaceMode_ClearsRulesOnly(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// Подготовим routing.json с custom правилом и custom rule_set.
	cfg := types.RoutingConfig{
		Version: 1,
		Rules: []types.RouteRule{
			{Network: "tcp", Port: []uint16{22}, Action: "block"},
		},
		RuleSets: []types.RuleSetRef{
			{Tag: "my-custom-list", Type: "remote", Format: "binary",
				URL: "https://example.com/list.srs", DownloadDetour: "direct"},
		},
	}
	if err := s.saveRoutingConfig(&cfg); err != nil {
		t.Fatalf("saveRoutingConfig: %v", err)
	}

	rec := do(s, authReq("POST", "/api/presets/torrents-direct/preview", map[string]string{"mode": "replace"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview replace: %d body=%s", rec.Code, rec.Body.String())
	}
	gotCfg, _ := decodePresetResp(t, rec.Body.Bytes())

	// Rules: только из пресета (torrents-direct → 1 правило).
	if len(gotCfg.Rules) != 1 {
		t.Fatalf("Rules: ожидалось 1 (только preset), получено %d (%+v)", len(gotCfg.Rules), gotCfg.Rules)
	}
	if len(gotCfg.Rules[0].Protocol) == 0 || gotCfg.Rules[0].Protocol[0] != "bittorrent" {
		t.Errorf("правило не из preset: %+v", gotCfg.Rules[0])
	}

	// RuleSets: custom сохранён (replace их не трогает).
	found := false
	for _, rs := range gotCfg.RuleSets {
		if rs.Tag == "my-custom-list" {
			found = true
		}
	}
	if !found {
		t.Errorf("custom rule_set должен остаться при replace, RuleSets=%+v", gotCfg.RuleSets)
	}
}

// TestPresetsPreview_ReplaceMode_ForceSetsFinal — replace перезаписывает Final даже если уже задан.
func TestPresetsPreview_ReplaceMode_ForceSetsFinal(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// outbound для резолва Final.
	rec := do(s, authReq("POST", "/api/outbounds", types.Outbound{
		Tag: "vless-test", Type: "vless", Server: "1.1.1.1", Port: 443,
		Settings: map[string]any{"uuid": "00000000-0000-0000-0000-000000000000"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup outbound: %d body=%s", rec.Code, rec.Body.String())
	}

	// Установим Final=direct вручную.
	rec = do(s, authReq("PUT", "/api/final", map[string]string{"final": "direct"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT final=direct: %d", rec.Code)
	}

	// replace через ru-direct-rest-vpn (Final={vpn} → vless-test).
	rec = do(s, authReq("POST", "/api/presets/ru-direct-rest-vpn/preview",
		map[string]string{"mode": "replace"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview replace: %d body=%s", rec.Code, rec.Body.String())
	}
	gotCfg, _ := decodePresetResp(t, rec.Body.Bytes())
	if gotCfg.Final != "vless-test" {
		t.Errorf("replace должен force-set Final=vless-test, получили %q", gotCfg.Final)
	}
}

// TestPresetsPreview_ReplaceMode_AddsMissingRuleSets — replace добавляет недостающие rule_sets из пресета.
func TestPresetsPreview_ReplaceMode_AddsMissingRuleSets(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	rec := do(s, authReq("POST", "/api/presets/ru-direct/preview",
		map[string]string{"mode": "replace"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d body=%s", rec.Code, rec.Body.String())
	}
	gotCfg, _ := decodePresetResp(t, rec.Body.Bytes())
	found := false
	for _, rs := range gotCfg.RuleSets {
		if rs.Tag == "geoip-ru" {
			found = true
		}
	}
	if !found {
		t.Errorf("ожидался добавленный rule_set geoip-ru, RuleSets=%+v", gotCfg.RuleSets)
	}
}

// TestPresetsPreview_VPNRequired_412 — пресет с {vpn} без outbound → 412.
func TestPresetsPreview_VPNRequired_412(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	s.cfg.RoutingUI.DefaultOutboundTag = func() string { return "" }

	rec := do(s, authReq("POST", "/api/presets/blocked-vpn/preview",
		map[string]string{"mode": "add"}))
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("ожидался 412, получен %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestPresetsPreview_NotFound_404.
func TestPresetsPreview_NotFound_404(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("POST", "/api/presets/nonexistent/preview", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("ожидался 404, получен %d", rec.Code)
	}
}

// TestPresetsPreview_DefaultModeIsAdd — пустое тело или пустой mode = "add".
func TestPresetsPreview_DefaultModeIsAdd(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	rec := do(s, authReq("POST", "/api/presets/torrents-direct/preview", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview: %d body=%s", rec.Code, rec.Body.String())
	}
	gotCfg, _ := decodePresetResp(t, rec.Body.Bytes())
	if len(gotCfg.Rules) != 1 {
		t.Errorf("ожидалось 1 правило (add к пустому), %+v", gotCfg.Rules)
	}
}

// ===== Routing Commit (PUT /api/routing) =====

// TestRoutingCommit_Valid_WritesToDisk — PUT /api/routing записывает на диск.
func TestRoutingCommit_Valid_WritesToDisk(t *testing.T) {
	s, routingPath := makeTestServerWithRouting(t)

	body := map[string]any{
		"rules": []types.RouteRule{
			{Network: "tcp", Port: []uint16{443}, Action: "block"},
		},
		"rule_sets": []types.RuleSetRef{},
		"final":     "direct",
	}
	rec := do(s, authReq("PUT", "/api/routing", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/routing: %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(routingPath); err != nil {
		t.Errorf("routing.json не создан: %v", err)
	}

	// проверим что на диске наши правила.
	rec = do(s, authReq("GET", "/api/state", nil))
	var st struct {
		Config types.RoutingConfig `json:"config"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if len(st.Config.Rules) != 1 || st.Config.Rules[0].Port[0] != 443 {
		t.Errorf("commit не сохранил правила: %+v", st.Config.Rules)
	}
	if st.Config.Final != "direct" {
		t.Errorf("Final не сохранён: %q", st.Config.Final)
	}
}

// TestRoutingCommit_PreservesInboundsOutbounds — commit не трогает Inbounds/Outbounds.
func TestRoutingCommit_PreservesInboundsOutbounds(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	// Подготовим outbound и inbound.
	rec := do(s, authReq("POST", "/api/outbounds", types.Outbound{
		Tag: "keep-me", Type: "vless", Server: "1.1.1.1", Port: 443,
		Settings: map[string]any{"uuid": "00000000-0000-0000-0000-000000000000"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup outbound: %d body=%s", rec.Code, rec.Body.String())
	}
	rec = do(s, authReq("POST", "/api/inbounds", types.Inbound{
		Tag: "tproxy-keep", Type: "tproxy",
		Settings: map[string]any{"listen": "::", "listen_port": 7895},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup inbound: %d body=%s", rec.Code, rec.Body.String())
	}

	// Commit с пустыми Rules.
	body := map[string]any{
		"rules":     []types.RouteRule{},
		"rule_sets": []types.RuleSetRef{},
		"final":     "",
	}
	rec = do(s, authReq("PUT", "/api/routing", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/routing: %d body=%s", rec.Code, rec.Body.String())
	}

	// Inbounds/Outbounds должны остаться.
	rec = do(s, authReq("GET", "/api/state", nil))
	var st struct {
		Config types.RoutingConfig `json:"config"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if len(st.Config.Outbounds) != 1 || st.Config.Outbounds[0].Tag != "keep-me" {
		t.Errorf("Outbounds потеряны: %+v", st.Config.Outbounds)
	}
	if len(st.Config.Inbounds) != 1 || st.Config.Inbounds[0].Tag != "tproxy-keep" {
		t.Errorf("Inbounds потеряны: %+v", st.Config.Inbounds)
	}
}

// TestRoutingCommit_ValidationError_FinalTag — Final с несуществующим тегом → 400.
func TestRoutingCommit_ValidationError_FinalTag(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	body := map[string]any{
		"rules":     []types.RouteRule{},
		"rule_sets": []types.RuleSetRef{},
		"final":     "ghost-tag",
	}
	rec := do(s, authReq("PUT", "/api/routing", body))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400 для несуществующего final tag, получен %d body=%s",
			rec.Code, rec.Body.String())
	}
}

// TestRoutingCommit_EmptyRulesIsValid — пустые правила — допустимое состояние.
func TestRoutingCommit_EmptyRulesIsValid(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)

	body := map[string]any{
		"rules":     []types.RouteRule{},
		"rule_sets": []types.RuleSetRef{},
		"final":     "",
	}
	rec := do(s, authReq("PUT", "/api/routing", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/routing с пустыми правилами: %d body=%s",
			rec.Code, rec.Body.String())
	}
}

// ===== rule_set.URL validation при добавлении (инцидент 2026-05-12, tasks/lessons.md) =====

// makeTestServerWithRuleSetChecker — Server с RoutingUI deps и реальным
// RuleSetChecker (через routing.CheckRuleSetURL), как это делает production
// wiring в internal/cli/cmd_ui.go. Unreachable → nil (не блокирует), Mismatch
// → error (блокирует, apiRuleSetsAdd вернёт 400).
func makeTestServerWithRuleSetChecker(t *testing.T) (*Server, string) {
	t.Helper()
	s, routingPath := makeTestServerWithRouting(t)
	client := &http.Client{Timeout: 3 * time.Second}
	s.cfg.RoutingUI.RuleSetChecker = func(ctx context.Context, ref types.RuleSetRef) error {
		res := routing.CheckRuleSetURL(ctx, client, ref)
		if res.Unreachable {
			return nil
		}
		if res.Mismatch {
			return fmt.Errorf("%s", res.Detail)
		}
		return nil
	}
	return s, routingPath
}

// TestRuleSetsAdd_NoChecker_SkipsValidation — RuleSetChecker не сконфигурирован
// (nil, как в makeTestServerWithRouting) → проверка полностью пропускается,
// добавление проходит независимо от доступности URL. Регрессионный тест на
// то, что этот фикс не меняет поведение существующих тестов (см.
// TestE2E_CustomRuleSet в routingui_e2e_test.go — URL на example.com, без
// реального сервера).
func TestRuleSetsAdd_NoChecker_SkipsValidation(t *testing.T) {
	s, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("POST", "/api/rule_sets", types.RuleSetRef{
		Tag: "no-checker", Type: "remote", URL: "https://unreachable.invalid/x.srs",
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("без RuleSetChecker: ожидался 201, получено %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestRuleSetsAdd_Checker404_400 — сервер отвечает 404 → 400 с понятным
// текстом причины, rule_set не сохраняется.
func TestRuleSetsAdd_Checker404_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	s, _ := makeTestServerWithRuleSetChecker(t)
	rec := do(s, authReq("POST", "/api/rule_sets", types.RuleSetRef{
		Tag: "ru-sites", Type: "remote", Format: "binary", URL: srv.URL + "/geosite-ru.srs",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("404 rule_set: ожидался 400, получено %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "404") {
		t.Errorf("тело ответа не объясняет причину: %s", rec.Body.String())
	}

	rec = do(s, authReq("GET", "/api/rule_sets", nil))
	var rss []types.RuleSetRef
	_ = json.Unmarshal(rec.Body.Bytes(), &rss)
	if len(rss) != 0 {
		t.Errorf("rule_set с 404 не должен был сохраниться: %+v", rss)
	}
}

// TestRuleSetsAdd_CheckerFormatMismatch_400 — JSON source вместо compiled SRS
// под .srs URL (ровно кейс инцидента 2026-05-12) → 400.
func TestRuleSetsAdd_CheckerFormatMismatch_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version": 2, "rules": []}`))
	}))
	defer srv.Close()

	s, _ := makeTestServerWithRuleSetChecker(t)
	rec := do(s, authReq("POST", "/api/rule_sets", types.RuleSetRef{
		Tag: "ru-sites", Type: "remote", Format: "binary", URL: srv.URL + "/geosite-category-ru.srs",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("format mismatch: ожидался 400, получено %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestRuleSetsAdd_CheckerOK_201 — корректный compiled rule-set проходит
// проверку и сохраняется.
func TestRuleSetsAdd_CheckerOK_201(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(append([]byte("SRS"), make([]byte, 16)...))
	}))
	defer srv.Close()

	s, _ := makeTestServerWithRuleSetChecker(t)
	rec := do(s, authReq("POST", "/api/rule_sets", types.RuleSetRef{
		Tag: "geoip-ru", Type: "remote", Format: "binary", URL: srv.URL + "/geoip-ru.srs",
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("валидный rule_set: ожидался 201, получено %d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(s, authReq("GET", "/api/rule_sets", nil))
	var rss []types.RuleSetRef
	_ = json.Unmarshal(rec.Body.Bytes(), &rss)
	if len(rss) != 1 || rss[0].Tag != "geoip-ru" {
		t.Errorf("валидный rule_set не сохранён: %+v", rss)
	}
}

// TestRuleSetsAdd_CheckerUnreachable_StillAdds — сервер недоступен (offline
// роутер) → проверка не блокирует добавление, иначе оператор без интернета
// не смог бы отредактировать routing.json вообще.
func TestRuleSetsAdd_CheckerUnreachable_StillAdds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL + "/geoip-ru.srs"
	srv.Close()

	s, _ := makeTestServerWithRuleSetChecker(t)
	rec := do(s, authReq("POST", "/api/rule_sets", types.RuleSetRef{
		Tag: "geoip-ru", Type: "remote", URL: url,
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("сеть недоступна: ожидался 201 (проверка пропущена), получено %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestRuleSetsAdd_CheckerInvalidJSON_StillHandledByCrudAdd — невалидный JSON
// тела не должен ломаться о предварительное чтение в apiRuleSetsAdd: ошибку
// в итоге всё равно возвращает crudAdd тем же способом, что и раньше.
func TestRuleSetsAdd_CheckerInvalidJSON_StillHandledByCrudAdd(t *testing.T) {
	s, _ := makeTestServerWithRuleSetChecker(t)
	req := httptest.NewRequest("POST", "/api/rule_sets", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "127.0.0.1:9092"
	req.Header.Set("Origin", "http://127.0.0.1:9092")

	rec := do(s, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("невалидный JSON: ожидался 400, получено %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestRuleSetsAdd_CheckerNoURL_SkipsCheck — rule_set без URL (type=local) не
// должен дёргать RuleSetChecker вообще (нет смысла проверять пустой URL).
func TestRuleSetsAdd_CheckerNoURL_SkipsCheck(t *testing.T) {
	s, _ := makeTestServerWithRuleSetChecker(t)
	rec := do(s, authReq("POST", "/api/rule_sets", types.RuleSetRef{
		Tag: "local-set", Type: "local", Path: "/opt/etc/sign-craze/local.srs",
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("local rule_set без URL: ожидался 201, получено %d body=%s", rec.Code, rec.Body.String())
	}
}

package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// makeTestServerWithRouting — Server с настроенным RoutingUI deps на временный routing.json.
func makeTestServerWithRouting(t *testing.T) (*Server, string, string) {
	t.Helper()
	s, password := makeTestServer(t)
	dir := t.TempDir()
	routingPath := filepath.Join(dir, "routing.json")
	s.cfg.RoutingUI = &RoutingUIDeps{RoutingPath: routingPath}
	return s, password, routingPath
}

// authReq — POST/PUT/DELETE с Basic Auth и Origin == Host.
func authReq(method, path, password string, body any) *http.Request {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.SetBasicAuth("admin", password)
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
	s, pw, _ := makeTestServerWithRouting(t)

	// add
	rec := do(s, authReq("POST", "/api/outbounds", pw, types.Outbound{
		Tag: "vless-out", Type: "vless", Server: "1.1.1.1", Port: 443,
		Settings: map[string]any{"uuid": "00000000-0000-0000-0000-000000000000"},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/outbounds: %d body=%s", rec.Code, rec.Body.String())
	}

	// list
	rec = do(s, authReq("GET", "/api/outbounds", pw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d", rec.Code)
	}
	var list []types.Outbound
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0].Tag != "vless-out" {
		t.Fatalf("список: %+v", list)
	}

	// duplicate add → 409
	rec = do(s, authReq("POST", "/api/outbounds", pw, types.Outbound{Tag: "vless-out", Type: "direct"}))
	if rec.Code != http.StatusConflict {
		t.Errorf("дубликат: %d", rec.Code)
	}

	// update
	rec = do(s, authReq("PUT", "/api/outbounds/vless-out", pw, types.Outbound{
		Tag: "vless-out", Type: "vless", Server: "2.2.2.2", Port: 443,
		Settings: map[string]any{"uuid": "11111111-1111-1111-1111-111111111111"},
	}))
	if rec.Code != http.StatusOK {
		t.Errorf("PUT: %d body=%s", rec.Code, rec.Body.String())
	}

	// delete
	rec = do(s, authReq("DELETE", "/api/outbounds/vless-out", pw, nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("DELETE: %d", rec.Code)
	}
}

func TestOutbounds_AddInvalid(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("POST", "/api/outbounds", pw, types.Outbound{Tag: "", Type: "vless"}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400, получен %d", rec.Code)
	}
}

// ===== Inbounds CRUD =====

func TestInbounds_AddList(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("POST", "/api/inbounds", pw, types.Inbound{
		Tag: "tproxy-in", Type: "tproxy",
		Settings: map[string]any{"listen": "::", "listen_port": 7895},
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST: %d body=%s", rec.Code, rec.Body.String())
	}
	rec = do(s, authReq("GET", "/api/inbounds", pw, nil))
	if !strings.Contains(rec.Body.String(), "tproxy-in") {
		t.Errorf("inbound не найден: %s", rec.Body.String())
	}
}

// ===== Rules CRUD + Reorder =====

func TestRules_AddListReorderDelete(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)

	// add 3 rules
	for i, action := range []string{"block", "block", "block"} {
		_ = i
		rec := do(s, authReq("POST", "/api/rules", pw, types.RouteRule{
			Network: "udp", Port: []uint16{uint16(135 + i)}, Action: action,
		}))
		if rec.Code != http.StatusCreated {
			t.Fatalf("add rule %d: %d body=%s", i, rec.Code, rec.Body.String())
		}
	}

	// list
	rec := do(s, authReq("GET", "/api/rules", pw, nil))
	var rules []types.RouteRule
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if len(rules) != 3 {
		t.Fatalf("ожидалось 3 правила, %d", len(rules))
	}

	// reorder: переместить idx=2 на idx=0
	rec = do(s, authReq("POST", "/api/rules/reorder", pw, map[string]int{"from": 2, "to": 0}))
	if rec.Code != http.StatusOK {
		t.Fatalf("reorder: %d body=%s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if rules[0].Port[0] != 137 {
		t.Errorf("после reorder первое правило должно быть port=137, %v", rules[0])
	}

	// delete
	rec = do(s, authReq("DELETE", "/api/rules/0", pw, nil))
	if rec.Code != http.StatusNoContent {
		t.Errorf("DELETE: %d", rec.Code)
	}
}

func TestRules_AddInvalidCIDR(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("POST", "/api/rules", pw, types.RouteRule{
		IPCIDR: []string{"not-a-cidr"}, Outbound: "direct",
	}))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400, получен %d", rec.Code)
	}
}

// ===== Presets =====

func TestPresets_List(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("GET", "/api/presets", pw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/presets: %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"block-ads", "ru-direct", "blocked-vpn", "discord-vpn", "torrents-direct", "block-bogon-udp"} {
		if !strings.Contains(body, want) {
			t.Errorf("preset %q отсутствует в response", want)
		}
	}
}

func TestPresets_Apply_BlockBogonUDP(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)

	rec := do(s, authReq("POST", "/api/presets/block-bogon-udp/apply", pw, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d body=%s", rec.Code, rec.Body.String())
	}
	rec = do(s, authReq("GET", "/api/rules", pw, nil))
	var rules []types.RouteRule
	_ = json.Unmarshal(rec.Body.Bytes(), &rules)
	if len(rules) == 0 || rules[0].Network != "udp" {
		t.Errorf("preset не применился: %+v", rules)
	}
}

func TestPresets_Apply_RuleSet_NoDuplicate(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)
	// apply ru-direct дважды → rule_set не дублируется
	_ = do(s, authReq("POST", "/api/presets/ru-direct/apply", pw, nil))
	_ = do(s, authReq("POST", "/api/presets/ru-direct/apply", pw, nil))

	rec := do(s, authReq("GET", "/api/rule_sets", pw, nil))
	var rss []types.RuleSetRef
	_ = json.Unmarshal(rec.Body.Bytes(), &rss)
	if len(rss) != 1 {
		t.Errorf("ожидался 1 rule_set, %d", len(rss))
	}
}

func TestPresets_Apply_NotFound(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)
	rec := do(s, authReq("POST", "/api/presets/nonexistent/apply", pw, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("ожидался 404, получен %d", rec.Code)
	}
}

// ===== Validate =====

func TestValidate_OK(t *testing.T) {
	s, pw, _ := makeTestServerWithRouting(t)
	cfg := types.RoutingConfig{
		Version: 1,
		Outbounds: []types.Outbound{
			{Tag: "out", Type: "direct"},
		},
	}
	rec := do(s, authReq("POST", "/api/validate", pw, cfg))
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
	s, pw, _ := makeTestServerWithRouting(t)
	cfg := types.RoutingConfig{
		Version: 0, // невалидно: version должен быть > 0
	}
	rec := do(s, authReq("POST", "/api/validate", pw, cfg))
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

// ===== Apply =====

func TestApply_NeedsRestart(t *testing.T) {
	s, pw, routingPath := makeTestServerWithRouting(t)

	// add outbound first
	_ = do(s, authReq("POST", "/api/outbounds", pw, types.Outbound{Tag: "out", Type: "direct"}))

	rec := do(s, authReq("POST", "/api/apply", pw, nil))
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

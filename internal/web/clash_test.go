package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newClashMux(t *testing.T) http.Handler {
	t.Helper()
	s, _ := makeTestServer(t)
	mux := http.NewServeMux()
	registerClashRoutes(mux, s)
	return mux
}

func TestClashHello(t *testing.T) {
	mux := newClashMux(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("неверный JSON: %v", err)
	}
	if body["hello"] != "clash" {
		t.Errorf("hello=%q, ожидалось %q", body["hello"], "clash")
	}
}

func TestClashVersion(t *testing.T) {
	mux := newClashMux(t)
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("неверный JSON: %v", err)
	}
	if _, ok := body["version"]; !ok {
		t.Error("нет поля version")
	}
	if premium, _ := body["premium"].(bool); premium {
		t.Error("premium должен быть false")
	}
}

func TestClashConfigs(t *testing.T) {
	mux := newClashMux(t)
	req := httptest.NewRequest(http.MethodGet, "/configs", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("неверный JSON: %v", err)
	}
	if body["mode"] == nil {
		t.Error("нет поля mode")
	}
}

func TestClashConnections(t *testing.T) {
	mux := newClashMux(t)
	req := httptest.NewRequest(http.MethodGet, "/connections", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
}

func TestClashSPAFallback_отдаёт_indexHTML(t *testing.T) {
	mux := newClashMux(t)
	req := httptest.NewRequest(http.MethodGet, "/несуществующий-путь", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// SPA fallback → index.html; 200 или 301 (redirect к /)
	body, _ := io.ReadAll(rec.Body)
	if rec.Code != http.StatusOK && rec.Code != http.StatusMovedPermanently {
		t.Errorf("ожидался 200/301, получен %d, тело: %s", rec.Code, body)
	}
}

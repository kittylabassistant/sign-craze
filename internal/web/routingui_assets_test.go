package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRoutingUIAssets_Embedded проверяет что index.html встроен и не пустой.
func TestRoutingUIAssets_Embedded(t *testing.T) {
	fsys := routingUIFS()
	f, err := fsys.Open("index.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("index.html пустой")
	}
}

// TestRoutingUIAssets_VendorPreact проверяет что preact.module.js скачан.
func TestRoutingUIAssets_VendorPreact(t *testing.T) {
	fsys := routingUIFS()
	f, err := fsys.Open("vendor/preact.module.js")
	if err != nil {
		t.Skipf("preact не скачан: %v", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() < 1000 {
		t.Errorf("preact слишком маленький: %d bytes", info.Size())
	}
}

// TestRoutingUI_SPAServesIndex проверяет что GET / возвращает index.html с заголовком приложения.
func TestRoutingUI_SPAServesIndex(t *testing.T) {
	s, password := makeTestServer(t)
	h := servingMux(s)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", password)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Routing Editor") {
		t.Errorf("body=%q не содержит 'Routing Editor'", rec.Body.String())
	}
}

// TestRoutingUI_SPAServesAppJS проверяет что app.js раздаётся корректно.
func TestRoutingUI_SPAServesAppJS(t *testing.T) {
	s, password := makeTestServer(t)
	h := servingMux(s)

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	req.SetBasicAuth("admin", password)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("app.js: status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "preact") {
		t.Errorf("app.js не содержит 'preact'")
	}
}

// TestRoutingUI_SPAFallback проверяет что неизвестный путь возвращает index.html (SPA fallback).
func TestRoutingUI_SPAFallback(t *testing.T) {
	s, password := makeTestServer(t)
	h := servingMux(s)

	req := httptest.NewRequest(http.MethodGet, "/some/spa/route", nil)
	req.SetBasicAuth("admin", password)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// SPA fallback должен вернуть 200 + index.html
	if rec.Code != http.StatusOK {
		t.Fatalf("SPA fallback: status=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type=%q, ожидался text/html", ct)
	}
}

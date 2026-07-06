package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// makeTestServer создаёт Server для тестов.
func makeTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := NewServer(ServerConfig{})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

// TestSecurityHeaders_присутствуют — middleware должен ставить X-Content-Type-Options,
// X-Frame-Options, CSP, Referrer-Policy на каждый ответ.
func TestSecurityHeaders_присутствуют(t *testing.T) {
	handler := securityHeadersAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, ожидалось %q", k, got, v)
		}
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("отсутствует Content-Security-Policy")
	}
}

// TestServer_AdminHasReadHeaderTimeout — admin server (9091) должен иметь
// ReadHeaderTimeout > 0 для защиты от slowloris.
func TestServer_AdminHasReadHeaderTimeout(t *testing.T) {
	s := makeTestServer(t)
	if s.admin.ReadHeaderTimeout <= 0 {
		t.Errorf("admin.ReadHeaderTimeout=%v, ожидалось > 0", s.admin.ReadHeaderTimeout)
	}
	const wantTimeout = 15 * time.Second
	if s.admin.ReadHeaderTimeout != wantTimeout {
		t.Errorf("admin.ReadHeaderTimeout=%v, ожидалось %v", s.admin.ReadHeaderTimeout, wantTimeout)
	}
}

func TestRecoverMiddleware_обрабатывает_панику(t *testing.T) {
	handler := recoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		panic("тест паники")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("ожидался 500, получен %d", rec.Code)
	}
}

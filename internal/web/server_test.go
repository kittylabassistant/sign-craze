package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// makeTestServer создаёт Server с известным паролем для тестов.
func makeTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	const password = "test-secret"

	dir := t.TempDir()
	path := filepath.Join(dir, "admin.creds")

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if wErr := os.WriteFile(path, append(hash, '\n'), 0o600); wErr != nil {
		t.Fatalf("WriteFile: %v", wErr)
	}

	s, err := NewServer(ServerConfig{CredsPath: path})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s, password
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

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

func TestBasicAuth_отклоняет_без_заголовка(t *testing.T) {
	s, _ := makeTestServer(t)
	handler := s.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ожидался 401, получен %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("нет заголовка WWW-Authenticate")
	}
}

func TestBasicAuth_принимает_верный_пароль(t *testing.T) {
	s, password := makeTestServer(t)
	handler := s.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", password)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался 200, получен %d", rec.Code)
	}
}

func TestBasicAuth_отклоняет_неверный_пароль(t *testing.T) {
	s, _ := makeTestServer(t)
	handler := s.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "неверный")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ожидался 401, получен %d", rec.Code)
	}
}

// TestBasicAuth_отклоняет_неверный_username — username должен быть строго "admin",
// иначе любой логин с правильным паролем мог бы войти (нарушение семантики Basic Auth).
func TestBasicAuth_отклоняет_неверный_username(t *testing.T) {
	s, password := makeTestServer(t)
	handler := s.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("root", password) // правильный пароль, неправильный username
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ожидался 401 для username='root', получен %d", rec.Code)
	}
}

// TestSecurityHeaders_присутствуют — middleware должен ставить X-Content-Type-Options,
// X-Frame-Options, CSP, Referrer-Policy на каждый ответ.
func TestSecurityHeaders_присутствуют(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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

// TestOriginGuard_отклоняет_cross_origin_POST — POST без same-origin Origin → 403.
func TestOriginGuard_отклоняет_cross_origin_POST(t *testing.T) {
	handler := originGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/ports", nil)
	req.Host = "192.168.1.1:9091"
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ожидался 403, получен %d", rec.Code)
	}
}

// TestOriginGuard_пропускает_same_origin_POST — POST с Origin совпадающим с Host → OK.
func TestOriginGuard_пропускает_same_origin_POST(t *testing.T) {
	handler := originGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/ports", nil)
	req.Host = "192.168.1.1:9091"
	req.Header.Set("Origin", "http://192.168.1.1:9091")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("ожидался 204, получен %d", rec.Code)
	}
}

// TestOriginGuard_GET_без_Origin_OK — GET-запросы не требуют Origin.
func TestOriginGuard_GET_без_Origin_OK(t *testing.T) {
	handler := originGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ожидался 200, получен %d", rec.Code)
	}
}

// TestOriginGuard_POST_без_Origin_403 — браузер всегда выставляет Origin для POST,
// CLI должен использовать GET или явно установить Origin.
func TestOriginGuard_POST_без_Origin_403(t *testing.T) {
	handler := originGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/ports", nil)
	req.Host = "localhost:9091"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ожидался 403 без Origin, получен %d", rec.Code)
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

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

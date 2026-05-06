package web

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestClashRoot_браузер_получает_SPA — браузер с Accept: text/html на GET /
// должен получать SPA-страницу Zashboard, а не Clash hello-JSON. Это
// исправляет UX: ввод http://host:9090/ возвращал JSON вместо UI.
func TestClashRoot_браузер_получает_SPA(t *testing.T) {
	mux := newClashMux(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	// SPA отдаёт HTML; Clash hello-JSON это application/json.
	if ct == "application/json; charset=utf-8" {
		t.Errorf("браузер получил JSON вместо SPA: ct=%s", ct)
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
	if rec.Code != http.StatusOK && rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusNotFound {
		t.Errorf("ожидался 200/301/404, получен %d, тело: %s", rec.Code, body)
	}
}

// TestSPA_возвращает_404_для_путей_с_расширением — попытка GET /etc/passwd, /admin.creds
// и т.п. не должна маскироваться под index.html. Иначе attacker не отличает 404 от 200.
func TestSPA_возвращает_404_для_путей_с_расширением(t *testing.T) {
	mux := newClashMux(t)
	for _, p := range []string{"/admin.creds", "/etc/passwd.txt", "/missing.png"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: ожидался 404, получен %d", p, rec.Code)
		}
	}
}

// TestSPACSP_разрешает_inline_scripts — браузерный SPA-дашборд (Zashboard)
// в index.html содержит inline-<script> с конфигом приложения. CSP на :9090
// должна разрешать 'unsafe-inline' в script-src, иначе SPA не bootstrap'ится.
// Регрессионный тест: проверяет, что обёртка securityHeadersSPA возвращает
// CSP с 'unsafe-inline' для script-src.
func TestSPACSP_разрешает_inline_scripts(t *testing.T) {
	handler := securityHeadersSPA(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src") {
		t.Fatalf("CSP без script-src: %q", csp)
	}
	// Извлекаем директиву script-src и проверяем 'unsafe-inline' внутри неё.
	scriptSrc := extractCSPDirective(csp, "script-src")
	if !strings.Contains(scriptSrc, "'unsafe-inline'") {
		t.Errorf("script-src без 'unsafe-inline': %q", scriptSrc)
	}
}

// TestAdminCSP_строгая — на admin-порту :9091 CSP должна оставаться строгой
// (без 'unsafe-inline' в script-src). Лендинг — статический HTML без JS.
// Регрессионная защита: смягчение admin CSP должно явно требовать обновления
// этого теста.
func TestAdminCSP_строгая(t *testing.T) {
	handler := securityHeadersAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	scriptSrc := extractCSPDirective(csp, "script-src")
	if strings.Contains(scriptSrc, "'unsafe-inline'") {
		t.Errorf("admin CSP не должна разрешать inline-скрипты: %q", scriptSrc)
	}
	if strings.Contains(scriptSrc, "'unsafe-eval'") {
		t.Errorf("admin CSP не должна разрешать eval: %q", scriptSrc)
	}
}

// extractCSPDirective достаёт значение директивы из CSP-строки.
// "default-src 'self'; script-src 'self' 'unsafe-inline'" → "script-src 'self' 'unsafe-inline'".
func extractCSPDirective(csp, name string) string {
	for _, part := range strings.Split(csp, ";") {
		p := strings.TrimSpace(part)
		if strings.HasPrefix(p, name+" ") || p == name {
			return p
		}
	}
	return ""
}

// TestClashEmbed_отдаёт_встроенный_файл — проверяет, что embed.FS жив:
// SPA-handler находит и отдаёт реальный файл из assets/zashboard. Если
// submodule не инициализирован или embed-директива сломана, тест падает —
// раннее предупреждение до релиза.
func TestClashEmbed_отдаёт_встроенный_файл(t *testing.T) {
	fsys := zashboardFS()
	// Ищем любой реальный файл (не директорию) в embed.
	var probe string
	walkErr := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if probe == "" {
			probe = path
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("WalkDir: %v", walkErr)
	}
	if probe == "" {
		t.Skip("embed assets/zashboard пуст (submodule не инициализирован?)")
	}

	mux := newClashMux(t)
	req := httptest.NewRequest(http.MethodGet, "/"+probe, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /%s → %d, ожидался 200", probe, rec.Code)
	}
}

// TestLooksLikeSPARoute — таблица: какие пути идут в SPA, какие → 404.
func TestLooksLikeSPARoute(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"", true},
		{"index.html", true},
		{"proxies", true},
		{"settings/general", true},
		{"connections.html", true},
		{"app.js", false},
		{"favicon.ico", false},
		{"main.css", false},
		{"image.png", false},
		{"admin.creds", false},
	}
	for _, tt := range tests {
		if got := looksLikeSPARoute(tt.path); got != tt.want {
			t.Errorf("looksLikeSPARoute(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

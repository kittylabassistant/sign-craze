package web

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// makeRoutingUIServer создаёт Server с включённым routing UI на 127.0.0.1:0
// (kernel выбирает порт через FindFreePort).
func makeRoutingUIServer(t *testing.T, port uint16, maxAttempts int) (*Server, string) {
	t.Helper()
	const password = "test-secret"

	dir := t.TempDir()
	credsPath := filepath.Join(dir, "admin.creds")
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if wErr := os.WriteFile(credsPath, append(hash, '\n'), 0o600); wErr != nil {
		t.Fatalf("WriteFile: %v", wErr)
	}

	cfg := ServerConfig{
		CredsPath:            credsPath,
		RoutingUIEnabled:     true,
		RoutingUIBind:        "127.0.0.1",
		RoutingUIPort:        port,
		RoutingUIMaxAttempts: maxAttempts,
	}
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() {
		if s.routingUIListener != nil {
			_ = s.routingUIListener.Close()
		}
	})
	return s, password
}

// servingMux строит handler chain как в NewServer, но без открытия listener'а.
// Для unit-тестов через httptest.NewRecorder.
func servingMux(s *Server) http.Handler {
	mux := http.NewServeMux()
	registerRoutingUIRoutes(mux, s)
	return recoverMiddleware(securityHeadersSPA(s.basicAuth(originGuard(mux))))
}

func TestRoutingUI_HealthEndpoint(t *testing.T) {
	s, password := makeTestServer(t)
	h := servingMux(s)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.SetBasicAuth(adminUsername, password)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != `{"status":"ok"}` {
		t.Errorf("body=%q, ожидалось {\"status\":\"ok\"}", got)
	}
}

// TestRoutingUI_StateEmpty — без RoutingUI deps возвращает default конфиг.
func TestRoutingUI_StateEmpty(t *testing.T) {
	s, password := makeTestServer(t)
	h := servingMux(s)

	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.SetBasicAuth(adminUsername, password)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"version"`) {
		t.Errorf("body не содержит version: %s", rec.Body.String())
	}
}

func TestRoutingUI_RequiresAuth(t *testing.T) {
	s, _ := makeTestServer(t)
	h := servingMux(s)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("ожидался 401 без auth, получен %d", rec.Code)
	}
}

func TestRoutingUI_SPARoot(t *testing.T) {
	s, password := makeTestServer(t)
	h := servingMux(s)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth(adminUsername, password)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type=%q", ct)
	}
}

// TestNewServer_RoutingUI_PortAllocation проверяет что NewServer со включённым
// routing UI открывает listener на указанном порту.
func TestNewServer_RoutingUI_PortAllocation(t *testing.T) {
	s, _ := makeRoutingUIServer(t, 0, 1)
	if s.routingUI == nil {
		t.Fatal("ожидался ненулевой routingUI")
	}
	if s.routingUIPort == 0 {
		t.Fatal("ожидался ненулевой routingUIPort")
	}
	if s.RoutingUIPort() != s.routingUIPort {
		t.Errorf("RoutingUIPort() != routingUIPort: %d != %d", s.RoutingUIPort(), s.routingUIPort)
	}
}

// TestNewServer_RoutingUI_FallbackOnBusyPort проверяет fallback на следующий порт.
func TestNewServer_RoutingUI_FallbackOnBusyPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	occPort := uint16(occupied.Addr().(*net.TCPAddr).Port)

	s, _ := makeRoutingUIServer(t, occPort, 5)
	if s.routingUIPort == occPort {
		t.Fatalf("ожидался fallback порт != %d, получен %d", occPort, s.routingUIPort)
	}
}

// TestNewServer_RoutingUI_Disabled — если RoutingUIEnabled=false, routingUI остаётся nil.
func TestNewServer_RoutingUI_Disabled(t *testing.T) {
	s, _ := makeTestServer(t)
	if s.routingUI != nil {
		t.Error("ожидался nil routingUI при выключенном RoutingUIEnabled")
	}
	if s.RoutingUIPort() != 0 {
		t.Errorf("ожидался RoutingUIPort()=0 при выключенном UI, получен %d", s.RoutingUIPort())
	}
}

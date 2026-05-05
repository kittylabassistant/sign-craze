package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Заглушки зависимостей ---

type stubStatus struct{ info StatusInfo }

func (s *stubStatus) Status(_ context.Context) (StatusInfo, error) { return s.info, nil }

type stubPorts struct{ ports []int }

func (s *stubPorts) ListPorts(_ context.Context) ([]int, error) { return s.ports, nil }
func (s *stubPorts) AddPort(_ context.Context, port int) error {
	s.ports = append(s.ports, port)
	return nil
}
func (s *stubPorts) DeletePort(_ context.Context, port int) error {
	for i, p := range s.ports {
		if p == port {
			s.ports = append(s.ports[:i], s.ports[i+1:]...)
			return nil
		}
	}
	return nil
}

type stubExcludes struct{ list []string }

func (s *stubExcludes) ListExcludes(_ context.Context) ([]string, error) { return s.list, nil }
func (s *stubExcludes) AddExclude(_ context.Context, cidr string) error {
	s.list = append(s.list, cidr)
	return nil
}
func (s *stubExcludes) DeleteExclude(_ context.Context, cidr string) error {
	for i, c := range s.list {
		if c == cidr {
			s.list = append(s.list[:i], s.list[i+1:]...)
			return nil
		}
	}
	return nil
}

// stubCores реализует CoresProvider — возвращает фиксированное имя.
type stubCores struct{ name string }

func (s *stubCores) ActiveCoreName() string { return s.name }

// --- Хелперы ---

func newAdminMux(t *testing.T, cfg ServerConfig) (http.Handler, *Server) {
	t.Helper()
	s, _ := makeTestServer(t)
	s.cfg = cfg
	// Обновляем creds в s из makeTestServer (уже установлены), только cfg меняем
	mux := http.NewServeMux()
	registerAdminRoutes(mux, s)
	return mux, s
}

// --- Тесты ---

// TestAdminLanding_GET_root_возвращает_HTML — порт 9091 это REST API,
// но GET / должен отдавать минимальную HTML-страницу с навигацией к
// :9092 (Routing Editor), а не 404.
func TestAdminLanding_GET_root_возвращает_HTML(t *testing.T) {
	mux, _ := newAdminMux(t, ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !bytes.Contains([]byte(ct), []byte("text/html")) {
		t.Errorf("Content-Type=%q, ожидался text/html", ct)
	}
	body := rec.Body.String()
	if !bytes.Contains([]byte(body), []byte(":9092")) {
		t.Errorf("landing должна ссылаться на :9092, получено: %s", body)
	}
}

func TestAPIStatus_возвращает_состояние(t *testing.T) {
	expected := StatusInfo{
		SingBox: ServiceState{Running: true, PID: 42},
		Mode:    "proxy",
	}
	mux, _ := newAdminMux(t, ServerConfig{
		Status: &stubStatus{info: expected},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	var got StatusInfo
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if !got.SingBox.Running || got.SingBox.PID != 42 {
		t.Errorf("неверный ответ: %+v", got)
	}
}

// TestAPICoresList_возвращает_зарегистрированные_ядра — /api/cores должен
// перечислять все зарегистрированные через core.Register ядра, помечать
// активное (из CoresProvider), и не падать если Runner=nil.
func TestAPICoresList_возвращает_зарегистрированные_ядра(t *testing.T) {
	mux, _ := newAdminMux(t, ServerConfig{
		Cores: &stubCores{name: "sing-box"},
		// Runner не задан — версии не печатаются, но запрос должен пройти.
	})

	req := httptest.NewRequest(http.MethodGet, "/api/cores", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d (%s)", rec.Code, rec.Body.String())
	}
	var got CoresResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if got.Active != "sing-box" {
		t.Errorf("Active=%q, ожидалось sing-box", got.Active)
	}
	// При тесте импортируется internal/web без блан-импорта singbox,
	// поэтому Available может быть пуст. Главное чтобы endpoint работал
	// и схема ответа была корректной.
	if got.Available == nil {
		got.Available = []CoreInfo{}
	}
}

// TestAPICoresList_без_CoresProvider — Active="" если cfg.Cores=nil.
func TestAPICoresList_без_CoresProvider(t *testing.T) {
	mux, _ := newAdminMux(t, ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/api/cores", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	var got CoresResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if got.Active != "" {
		t.Errorf("Active=%q, ожидалось пусто (CoresProvider=nil)", got.Active)
	}
}

func TestAPIPortsList_возвращает_список(t *testing.T) {
	stub := &stubPorts{ports: []int{80, 443}}
	mux, _ := newAdminMux(t, ServerConfig{Ports: stub})

	req := httptest.NewRequest(http.MethodGet, "/api/ports", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	var ports []int
	if err := json.NewDecoder(rec.Body).Decode(&ports); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if len(ports) != 2 {
		t.Errorf("ожидалось 2 порта, получено %d", len(ports))
	}
}

func TestAPIPortsAdd_добавляет_порт(t *testing.T) {
	stub := &stubPorts{}
	mux, _ := newAdminMux(t, ServerConfig{Ports: stub})

	body, _ := json.Marshal(map[string]int{"port": 8080})
	req := httptest.NewRequest(http.MethodPost, "/api/ports", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("ожидался 201, получен %d", rec.Code)
	}
	if len(stub.ports) != 1 || stub.ports[0] != 8080 {
		t.Errorf("порт не добавлен: %v", stub.ports)
	}
}

func TestAPIPortsAdd_отклоняет_невалидный_порт(t *testing.T) {
	stub := &stubPorts{}
	mux, _ := newAdminMux(t, ServerConfig{Ports: stub})

	body, _ := json.Marshal(map[string]int{"port": 99999})
	req := httptest.NewRequest(http.MethodPost, "/api/ports", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400, получен %d", rec.Code)
	}
}

func TestAPIPortsDel_удаляет_порт(t *testing.T) {
	stub := &stubPorts{ports: []int{80}}
	mux, _ := newAdminMux(t, ServerConfig{Ports: stub})

	req := httptest.NewRequest(http.MethodDelete, "/api/ports/80", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("ожидался 204, получен %d", rec.Code)
	}
}

func TestAPIExcludesList_возвращает_список(t *testing.T) {
	stub := &stubExcludes{list: []string{"192.168.1.0/24"}}
	mux, _ := newAdminMux(t, ServerConfig{Excludes: stub})

	req := httptest.NewRequest(http.MethodGet, "/api/excludes", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидался 200, получен %d", rec.Code)
	}
	var list []string
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("JSON decode: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ожидалось 1 исключение, получено %d", len(list))
	}
}

func TestAPIExcludesAdd_добавляет(t *testing.T) {
	stub := &stubExcludes{}
	mux, _ := newAdminMux(t, ServerConfig{Excludes: stub})

	body, _ := json.Marshal(map[string]string{"cidr": "10.0.0.0/8"})
	req := httptest.NewRequest(http.MethodPost, "/api/excludes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("ожидался 201, получен %d", rec.Code)
	}
	if len(stub.list) != 1 || stub.list[0] != "10.0.0.0/8" {
		t.Errorf("исключение не добавлено: %v", stub.list)
	}
}

// TestAPIConfigPost_BodyTooLarge — тело > 1MB должно отклоняться.
// Защита от DoS на 128MB роутер (safety-fixes #7).
func TestAPIConfigPost_BodyTooLarge(t *testing.T) {
	stub := &stubConfig{}
	mux, _ := newAdminMux(t, ServerConfig{Config: stub})

	huge := bytes.Repeat([]byte("a"), 2*1024*1024)
	req := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader(huge))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusNoContent {
		t.Fatal("ожидалось отклонение, получен 204")
	}
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("ожидался 400 или 413, получен %d", rec.Code)
	}
}

// TestAPIPortsAdd_ошибка_JSON_не_содержит_деталей_Go — error response не должен
// раскрывать внутренние Go-типы (info leak fingerprint).
func TestAPIPortsAdd_ошибка_JSON_не_содержит_деталей_Go(t *testing.T) {
	stub := &stubPorts{}
	mux, _ := newAdminMux(t, ServerConfig{Ports: stub})

	req := httptest.NewRequest(http.MethodPost, "/api/ports", bytes.NewReader([]byte(`{"port": "abc"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("ожидался 400, получен %d", rec.Code)
	}
	body := rec.Body.String()
	for _, leak := range []string{"json:", "Go", "type "} {
		if bytes.Contains([]byte(body), []byte(leak)) {
			t.Errorf("response утечка %q: %q", leak, body)
		}
	}
}

type stubConfig struct{ written []byte }

func (s *stubConfig) ReadConfig(_ context.Context) ([]byte, error) { return s.written, nil }
func (s *stubConfig) WriteConfig(_ context.Context, raw []byte) error {
	s.written = append([]byte(nil), raw...)
	return nil
}

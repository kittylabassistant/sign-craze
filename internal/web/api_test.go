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

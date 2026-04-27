package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// registerAdminRoutes регистрирует admin REST маршруты на порту 9091.
func registerAdminRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("GET /api/status", s.apiStatus)
	mux.HandleFunc("GET /api/config", s.apiConfigGet)
	mux.HandleFunc("POST /api/config", s.apiConfigPost)
	mux.HandleFunc("GET /api/ports", s.apiPortsList)
	mux.HandleFunc("POST /api/ports", s.apiPortsAdd)
	mux.HandleFunc("DELETE /api/ports/{port}", s.apiPortsDel)
	mux.HandleFunc("GET /api/excludes", s.apiExcludesList)
	mux.HandleFunc("POST /api/excludes", s.apiExcludesAdd)
	mux.HandleFunc("DELETE /api/excludes/{cidr}", s.apiExcludesDel)
}

func (s *Server) apiStatus(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Status == nil {
		writeJSON(w, StatusInfo{})
		return
	}
	info, err := s.cfg.Status.Status(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, info)
}

func (s *Server) apiConfigGet(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Config == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	data, err := s.cfg.Config.ReadConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data) //nolint:errcheck
}

func (s *Server) apiConfigPost(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Config == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "неверный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.cfg.Config.WriteConfig(r.Context(), raw); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiPortsList(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ports == nil {
		writeJSON(w, []int{})
		return
	}
	ports, err := s.cfg.Ports.ListPorts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, ports)
}

func (s *Server) apiPortsAdd(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ports == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	var body struct {
		Port int `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "неверный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Port < 1 || body.Port > 65535 {
		http.Error(w, "порт вне диапазона 1-65535", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Ports.AddPort(r.Context(), body.Port); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) apiPortsDel(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ports == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	portStr := r.PathValue("port")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "неверный порт", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Ports.DeletePort(r.Context(), port); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiExcludesList(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Excludes == nil {
		writeJSON(w, []string{})
		return
	}
	excludes, err := s.cfg.Excludes.ListExcludes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, excludes)
}

func (s *Server) apiExcludesAdd(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Excludes == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	var body struct {
		CIDR string `json:"cidr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "неверный JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.CIDR == "" {
		http.Error(w, "cidr обязателен", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Excludes.AddExclude(r.Context(), body.CIDR); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) apiExcludesDel(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Excludes == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	// PathValue возвращает URL-encoded значение; декодировать не нужно — "/" в CIDR не встречается
	cidr := r.PathValue("cidr")
	cidr = strings.ReplaceAll(cidr, "%2F", "/") // на случай URL-кодирования слэша
	if cidr == "" {
		http.Error(w, "cidr обязателен", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Excludes.DeleteExclude(r.Context(), cidr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

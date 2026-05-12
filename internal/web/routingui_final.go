package web

import (
	"net/http"
)

// apiFinalSet — PUT /api/final {"final": "<tag>"}.
// Пустая строка очищает Final. Валидация (builtin direct/block либо
// существующий outbound tag) выполняется в RoutingConfig.Validate
// через saveRoutingConfig → routing.Save.
func (s *Server) apiFinalSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Final string `json:"final"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.Final = body.Final
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, cfg)
}

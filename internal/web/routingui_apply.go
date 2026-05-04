package web

import (
	"net/http"
)

// apiApply — POST /api/apply
// Сохраняет текущий routing.json (он уже сохранён инкрементально через CRUD,
// но здесь — финальная валидация + опциональный OnApply hook для regenerate).
// Возвращает {needs_restart: true|false}.
func (s *Server) apiApply(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := cfg.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// re-save для атомарности (если файла не было — создастся)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.cfg.RoutingUI != nil && s.cfg.RoutingUI.OnApply != nil {
		if err := s.cfg.RoutingUI.OnApply(r.Context()); err != nil {
			http.Error(w, "apply hook: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]any{"needs_restart": true})
}

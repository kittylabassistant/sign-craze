package web

import (
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// validateResponse возвращается из /api/validate и /api/preview-failed.
type validateResponse struct {
	OK     bool     `json:"ok"`
	Errors []string `json:"errors,omitempty"`
}

// apiValidate — POST /api/validate
// Body: RoutingConfig или null (тогда валидируется текущий routing.json).
// Запускает Validate() + Renderer (если задан) и возвращает {ok, errors}.
func (s *Server) apiValidate(w http.ResponseWriter, r *http.Request) {
	var cfg *types.RoutingConfig
	if r.ContentLength > 0 {
		var rc types.RoutingConfig
		if !decodeJSON(w, r, &rc) {
			return
		}
		cfg = &rc
	} else {
		c, err := s.loadRoutingConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cfg = c
	}

	resp := validateResponse{OK: true}

	if err := cfg.Validate(); err != nil {
		resp.OK = false
		resp.Errors = append(resp.Errors, err.Error())
		writeJSON(w, resp)
		return
	}

	if s.cfg.RoutingUI != nil && s.cfg.RoutingUI.Renderer != nil {
		if _, err := s.cfg.RoutingUI.Renderer(r.Context(), cfg); err != nil {
			resp.OK = false
			resp.Errors = append(resp.Errors, err.Error())
		}
	}
	writeJSON(w, resp)
}

// apiPreview — GET /api/preview
// Возвращает rendered config.json (raw) если Renderer задан, иначе 503.
func (s *Server) apiPreview(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RoutingUI == nil || s.cfg.RoutingUI.Renderer == nil {
		http.Error(w, "renderer не настроен", http.StatusServiceUnavailable)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out, err := s.cfg.RoutingUI.Renderer(r.Context(), cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(out)
}

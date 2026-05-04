package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/routing"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// loadRoutingConfig читает текущий routing.json или возвращает empty default.
func (s *Server) loadRoutingConfig() (*types.RoutingConfig, error) {
	if s.cfg.RoutingUI == nil || s.cfg.RoutingUI.RoutingPath == "" {
		return routing.Default(), nil
	}
	cfg, err := routing.Load(s.cfg.RoutingUI.RoutingPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return routing.Default(), nil
		}
		return nil, err
	}
	if cfg == nil {
		return routing.Default(), nil
	}
	return cfg, nil
}

// saveRoutingConfig валидирует и атомарно записывает routing.json.
func (s *Server) saveRoutingConfig(cfg *types.RoutingConfig) error {
	if s.cfg.RoutingUI == nil || s.cfg.RoutingUI.RoutingPath == "" {
		return errors.New("routing UI deps не инициализированы")
	}
	if cfg.Version == 0 {
		cfg.Version = routing.SchemaVersion
	}
	return routing.Save(s.cfg.RoutingUI.RoutingPath, cfg)
}

// apiRoutingState — GET /api/state — возвращает текущий RoutingConfig.
func (s *Server) apiRoutingState(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"config": cfg,
	})
}

// decodeJSON — типизированный декодер с лимитом тела.
func decodeJSON[T any](w http.ResponseWriter, r *http.Request, v *T) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "неверный JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

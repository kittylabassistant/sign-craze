package web

import (
	"log/slog"
	"net/http"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/routing"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// validateResponse возвращается из /api/validate и /api/preview-failed.
// Warnings не делают OK=false (cf. compatibility-warnings про TUN inbound
// на xray/mihomo, .srs URL на mihomo и т.д.).
type validateResponse struct {
	OK       bool     `json:"ok"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// apiValidate — POST /api/validate
// Body: RoutingConfig или null (тогда валидируется текущий routing.json).
// Запускает Validate() + Validator (через активное ядро) и возвращает {ok, errors, warnings}.
func (s *Server) apiValidate(w http.ResponseWriter, r *http.Request) {
	var cfg *types.RoutingConfig
	if r.ContentLength > 0 {
		var rc types.RoutingConfig
		if !decodeJSON(w, r, &rc) {
			return
		}
		// Авто-патч: клиент мог прислать конфиг без поля version (legacy или новый).
		// Тот же паттерн используется в saveRoutingConfig.
		if rc.Version == 0 {
			slog.Debug("web: validate: version=0, подставляем SchemaVersion",
				"schema_version", routing.SchemaVersion)
			rc.Version = routing.SchemaVersion
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

	// Core-aware валидация: рендер через активное ядро + CheckConfig + сбор warnings.
	// Если Validator не настроен — fallback на Renderer (legacy путь для тестов).
	if s.cfg.RoutingUI != nil {
		switch {
		case s.cfg.RoutingUI.Validator != nil:
			warnings, err := s.cfg.RoutingUI.Validator(r.Context(), cfg)
			resp.Warnings = append(resp.Warnings, warnings...)
			if err != nil {
				resp.OK = false
				resp.Errors = append(resp.Errors, err.Error())
			}
		case s.cfg.RoutingUI.Renderer != nil:
			if _, err := s.cfg.RoutingUI.Renderer(r.Context(), cfg); err != nil {
				resp.OK = false
				resp.Errors = append(resp.Errors, err.Error())
			}
		}
	}
	writeJSON(w, resp)
}

// apiPreview — GET /api/preview
// Возвращает rendered config (raw) если Renderer задан, иначе 503.
// Content-Type зависит от формата активного ядра: JSON для sing-box/xray,
// YAML для mihomo. Без ConfigFormat callback — fallback JSON.
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
	ct := "application/json; charset=utf-8"
	if s.cfg.RoutingUI.ConfigFormat != nil && s.cfg.RoutingUI.ConfigFormat() == core.ConfigYAML {
		ct = "application/yaml; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	if _, wErr := w.Write(out); wErr != nil {
		slog.Debug("web: запись preview response", "err", wErr)
	}
}

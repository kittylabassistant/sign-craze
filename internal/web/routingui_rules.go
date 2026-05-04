package web

import (
	"net/http"
	"strconv"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func (s *Server) apiRulesList(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg.Rules)
}

func (s *Server) apiRulesAdd(w http.ResponseWriter, r *http.Request) {
	var rule types.RouteRule
	if !decodeJSON(w, r, &rule) {
		return
	}
	if err := rule.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.Rules = append(cfg.Rules, rule)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"index": len(cfg.Rules) - 1, "rule": rule})
}

func (s *Server) apiRulesUpdate(w http.ResponseWriter, r *http.Request) {
	idx, err := parseRuleIdx(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var rule types.RouteRule
	if !decodeJSON(w, r, &rule) {
		return
	}
	if err := rule.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if idx < 0 || idx >= len(cfg.Rules) {
		http.Error(w, "индекс правила вне диапазона", http.StatusNotFound)
		return
	}
	cfg.Rules[idx] = rule
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, rule)
}

func (s *Server) apiRulesDelete(w http.ResponseWriter, r *http.Request) {
	idx, err := parseRuleIdx(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if idx < 0 || idx >= len(cfg.Rules) {
		http.Error(w, "индекс правила вне диапазона", http.StatusNotFound)
		return
	}
	cfg.Rules = append(cfg.Rules[:idx], cfg.Rules[idx+1:]...)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiRulesReorder — POST /api/rules/reorder body={from: int, to: int}.
func (s *Server) apiRulesReorder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	n := len(cfg.Rules)
	if body.From < 0 || body.From >= n || body.To < 0 || body.To >= n {
		http.Error(w, "индекс вне диапазона", http.StatusBadRequest)
		return
	}
	if body.From == body.To {
		writeJSON(w, cfg.Rules)
		return
	}
	rule := cfg.Rules[body.From]
	cfg.Rules = append(cfg.Rules[:body.From], cfg.Rules[body.From+1:]...)
	if body.To > body.From {
		body.To--
	}
	cfg.Rules = append(cfg.Rules[:body.To], append([]types.RouteRule{rule}, cfg.Rules[body.To:]...)...)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg.Rules)
}

func parseRuleIdx(r *http.Request) (int, error) {
	v := r.PathValue("idx")
	idx, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	return idx, nil
}

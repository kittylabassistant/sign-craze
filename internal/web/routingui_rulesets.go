package web

import (
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func (s *Server) apiRuleSetsList(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg.RuleSets)
}

func (s *Server) apiRuleSetsAdd(w http.ResponseWriter, r *http.Request) {
	var rs types.RuleSetRef
	if !decodeJSON(w, r, &rs) {
		return
	}
	if err := rs.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, existing := range cfg.RuleSets {
		if existing.Tag == rs.Tag {
			http.Error(w, "rule_set с таким tag уже существует", http.StatusConflict)
			return
		}
	}
	cfg.RuleSets = append(cfg.RuleSets, rs)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, rs)
}

func (s *Server) apiRuleSetsDelete(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := cfg.RuleSets[:0]
	found := false
	for _, rs := range cfg.RuleSets {
		if rs.Tag == tag {
			found = true
			continue
		}
		out = append(out, rs)
	}
	if !found {
		http.Error(w, "rule_set не найден", http.StatusNotFound)
		return
	}
	cfg.RuleSets = out
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

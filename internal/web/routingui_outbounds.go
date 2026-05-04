package web

import (
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func (s *Server) apiOutboundsList(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg.Outbounds)
}

func (s *Server) apiOutboundsAdd(w http.ResponseWriter, r *http.Request) {
	var ob types.Outbound
	if !decodeJSON(w, r, &ob) {
		return
	}
	if err := ob.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, existing := range cfg.Outbounds {
		if existing.Tag == ob.Tag {
			http.Error(w, "outbound с таким tag уже существует", http.StatusConflict)
			return
		}
	}
	cfg.Outbounds = append(cfg.Outbounds, ob)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, ob)
}

func (s *Server) apiOutboundsUpdate(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	var ob types.Outbound
	if !decodeJSON(w, r, &ob) {
		return
	}
	if err := ob.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	idx := -1
	for i, ex := range cfg.Outbounds {
		if ex.Tag == tag {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Error(w, "outbound не найден", http.StatusNotFound)
		return
	}
	cfg.Outbounds[idx] = ob
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, ob)
}

func (s *Server) apiOutboundsDelete(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := cfg.Outbounds[:0]
	found := false
	for _, ob := range cfg.Outbounds {
		if ob.Tag == tag {
			found = true
			continue
		}
		out = append(out, ob)
	}
	if !found {
		http.Error(w, "outbound не найден", http.StatusNotFound)
		return
	}
	cfg.Outbounds = out
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

package web

import (
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// apiInboundsList — GET /api/inbounds.
func (s *Server) apiInboundsList(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg.Inbounds)
}

// apiInboundsAdd — POST /api/inbounds.
func (s *Server) apiInboundsAdd(w http.ResponseWriter, r *http.Request) {
	var in types.Inbound
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := in.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, existing := range cfg.Inbounds {
		if existing.Tag == in.Tag {
			http.Error(w, "inbound с таким tag уже существует", http.StatusConflict)
			return
		}
	}
	cfg.Inbounds = append(cfg.Inbounds, in)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, in)
}

// apiInboundsUpdate — PUT /api/inbounds/{tag}.
func (s *Server) apiInboundsUpdate(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	var in types.Inbound
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := in.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	idx := -1
	for i, ex := range cfg.Inbounds {
		if ex.Tag == tag {
			idx = i
			break
		}
	}
	if idx < 0 {
		http.Error(w, "inbound не найден", http.StatusNotFound)
		return
	}
	cfg.Inbounds[idx] = in
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, in)
}

// apiInboundsDelete — DELETE /api/inbounds/{tag}.
func (s *Server) apiInboundsDelete(w http.ResponseWriter, r *http.Request) {
	tag := r.PathValue("tag")
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := cfg.Inbounds[:0]
	found := false
	for _, in := range cfg.Inbounds {
		if in.Tag == tag {
			found = true
			continue
		}
		out = append(out, in)
	}
	if !found {
		http.Error(w, "inbound не найден", http.StatusNotFound)
		return
	}
	cfg.Inbounds = out
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

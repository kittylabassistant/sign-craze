package web

import (
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// inboundSpec — параметризация generic CRUD (routingui_crud.go) под срез Inbounds.
var inboundSpec = crudSpec[types.Inbound]{
	entity: "inbound",
	get:    func(cfg *types.RoutingConfig) []types.Inbound { return cfg.Inbounds },
	set:    func(cfg *types.RoutingConfig, v []types.Inbound) { cfg.Inbounds = v },
	tagOf:  func(in types.Inbound) string { return in.Tag },
}

// apiInboundsList — GET /api/inbounds.
func (s *Server) apiInboundsList(w http.ResponseWriter, r *http.Request) {
	crudList(s, w, r, inboundSpec)
}

// apiInboundsAdd — POST /api/inbounds.
func (s *Server) apiInboundsAdd(w http.ResponseWriter, r *http.Request) {
	crudAdd(s, w, r, inboundSpec)
}

// apiInboundsUpdate — PUT /api/inbounds/{tag}.
func (s *Server) apiInboundsUpdate(w http.ResponseWriter, r *http.Request) {
	crudUpdate(s, w, r, inboundSpec)
}

// apiInboundsDelete — DELETE /api/inbounds/{tag}.
func (s *Server) apiInboundsDelete(w http.ResponseWriter, r *http.Request) {
	crudDelete(s, w, r, inboundSpec)
}

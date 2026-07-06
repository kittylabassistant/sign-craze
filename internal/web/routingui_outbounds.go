package web

import (
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// outboundSpec — параметризация generic CRUD (routingui_crud.go) под срез Outbounds.
var outboundSpec = crudSpec[types.Outbound]{
	entity: "outbound",
	get:    func(cfg *types.RoutingConfig) []types.Outbound { return cfg.Outbounds },
	set:    func(cfg *types.RoutingConfig, v []types.Outbound) { cfg.Outbounds = v },
	tagOf:  func(ob types.Outbound) string { return ob.Tag },
}

// apiOutboundsList — GET /api/outbounds.
func (s *Server) apiOutboundsList(w http.ResponseWriter, r *http.Request) {
	crudList(s, w, r, outboundSpec)
}

// apiOutboundsAdd — POST /api/outbounds.
func (s *Server) apiOutboundsAdd(w http.ResponseWriter, r *http.Request) {
	crudAdd(s, w, r, outboundSpec)
}

// apiOutboundsUpdate — PUT /api/outbounds/{tag}.
func (s *Server) apiOutboundsUpdate(w http.ResponseWriter, r *http.Request) {
	crudUpdate(s, w, r, outboundSpec)
}

// apiOutboundsDelete — DELETE /api/outbounds/{tag}.
func (s *Server) apiOutboundsDelete(w http.ResponseWriter, r *http.Request) {
	crudDelete(s, w, r, outboundSpec)
}

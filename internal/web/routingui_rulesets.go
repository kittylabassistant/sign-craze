package web

import (
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// ruleSetSpec — параметризация generic CRUD (routingui_crud.go) под срез RuleSets.
// Update-хендлера для rule_sets нет (не зарегистрирован в server.go) — только
// List/Add/Delete.
var ruleSetSpec = crudSpec[types.RuleSetRef]{
	entity: "rule_set",
	get:    func(cfg *types.RoutingConfig) []types.RuleSetRef { return cfg.RuleSets },
	set:    func(cfg *types.RoutingConfig, v []types.RuleSetRef) { cfg.RuleSets = v },
	tagOf:  func(rs types.RuleSetRef) string { return rs.Tag },
}

// apiRuleSetsList — GET /api/rule_sets.
func (s *Server) apiRuleSetsList(w http.ResponseWriter, r *http.Request) {
	crudList(s, w, r, ruleSetSpec)
}

// apiRuleSetsAdd — POST /api/rule_sets.
func (s *Server) apiRuleSetsAdd(w http.ResponseWriter, r *http.Request) {
	crudAdd(s, w, r, ruleSetSpec)
}

// apiRuleSetsDelete — DELETE /api/rule_sets/{tag}.
func (s *Server) apiRuleSetsDelete(w http.ResponseWriter, r *http.Request) {
	crudDelete(s, w, r, ruleSetSpec)
}

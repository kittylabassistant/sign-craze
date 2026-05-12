package web

import (
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// apiRoutingCommit — PUT /api/routing
// Атомарно заменяет routing-секцию конфига (Rules + RuleSets + Final).
// Inbounds/Outbounds сохраняются нетронутыми — у них своя CRUD-семантика
// на других вкладках UI. Используется клиентским буфером несохранённых
// изменений: после preview или ручного редактирования UI присылает финальный
// набор Rules+RuleSets+Final одним вызовом.
//
// Тело:   {"rules": [], "rule_sets": [], "final": ""}
// Ответ:  200 {"config": RoutingConfig}
//
//	400 ошибка валидации (RoutingConfig.Validate)
//	500 ошибка чтения/записи routing.json
func (s *Server) apiRoutingCommit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Rules    []types.RouteRule  `json:"rules"`
		RuleSets []types.RuleSetRef `json:"rule_sets"`
		Final    string             `json:"final"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg.Rules = body.Rules
	cfg.RuleSets = body.RuleSets
	cfg.Final = body.Final
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"config": cfg})
}

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// ruleSetCheckTimeout — бюджет на синхронную HEAD/GET-проверку rule_set.URL
// при добавлении через веб-редактор. Блокирует HTTP-ответ, поэтому короткий:
// оператор не должен подолгу ждать при сохранении конфига.
const ruleSetCheckTimeout = 5 * time.Second

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

// apiRuleSetsAdd — POST /api/rule_sets. Тонкая обёртка вокруг generic
// crudAdd: перед добавлением синхронно проверяет rule_set.URL, если он задан
// и в Server сконфигурирован RuleSetChecker (см. RoutingUIDeps.RuleSetChecker).
//
// Инцидент 2026-05-12 (tasks/lessons.md): rule_set с 404-URL и rule_set с
// mismatch формата (JSON source под .srs расширением, ожидавшимся как
// compiled binary) валили sing-box на старте у живых пользователей — конфиг
// сохранялся через UI без какой-либо проверки. Эта проверка ловит оба кейса
// на этапе сохранения, а не на следующем --restart.
//
// RuleSetChecker == nil (не сконфигурирован — например, в тестах, которые
// строят RoutingUIDeps напрямую без этого поля) → проверка полностью
// пропускается, поведение идентично коду до этого фикса.
//
// Тело запроса читается и буферизуется здесь (а не через decodeJSON), чтобы
// после локальной проверки восстановить r.Body для crudAdd — он делает
// собственные decode+Validate и не должен знать о предварительной проверке.
func (s *Server) apiRuleSetsAdd(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RoutingUI != nil && s.cfg.RoutingUI.RuleSetChecker != nil {
		raw, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody))
		_ = r.Body.Close()
		if err != nil {
			http.Error(w, "неверный JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Восстанавливаем тело для crudAdd независимо от исхода парсинга ниже —
		// невалидный JSON здесь не считаем ошибкой предварительной проверки,
		// его поймает и корректно отрапортует crudAdd (единый формат 400).
		r.Body = io.NopCloser(bytes.NewReader(raw))

		var ref types.RuleSetRef
		if json.Unmarshal(raw, &ref) == nil && ref.URL != "" {
			ctx, cancel := context.WithTimeout(r.Context(), ruleSetCheckTimeout)
			checkErr := s.cfg.RoutingUI.RuleSetChecker(ctx, ref)
			cancel()
			if checkErr != nil {
				http.Error(w, "rule_set.url: "+checkErr.Error(), http.StatusBadRequest)
				return
			}
		}
	}
	crudAdd(s, w, r, ruleSetSpec)
}

// apiRuleSetsDelete — DELETE /api/rule_sets/{tag}.
func (s *Server) apiRuleSetsDelete(w http.ResponseWriter, r *http.Request) {
	crudDelete(s, w, r, ruleSetSpec)
}

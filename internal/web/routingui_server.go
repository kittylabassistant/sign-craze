package web

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
)

// registerRoutingUIRoutes регистрирует endpoints UI-редактора routing.
// Phase 2.3: SPA-статика раздаётся из embed.FS (Preact + htm, offline bundle).
func registerRoutingUIRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, wErr := w.Write([]byte(`{"status":"ok"}`)); wErr != nil {
			slog.Debug("web: запись health response", "err", wErr)
		}
	})

	// state
	mux.HandleFunc("GET /api/state", s.apiRoutingState)

	// routing commit (атомарная замена Rules+RuleSets+Final, клиентский буфер)
	mux.HandleFunc("PUT /api/routing", s.apiRoutingCommit)

	// final outbound (default action для непокрытых правил)
	mux.HandleFunc("PUT /api/final", s.apiFinalSet)

	// inbounds CRUD
	mux.HandleFunc("GET /api/inbounds", s.apiInboundsList)
	mux.HandleFunc("POST /api/inbounds", s.apiInboundsAdd)
	mux.HandleFunc("PUT /api/inbounds/{tag}", s.apiInboundsUpdate)
	mux.HandleFunc("DELETE /api/inbounds/{tag}", s.apiInboundsDelete)

	// outbounds CRUD
	mux.HandleFunc("GET /api/outbounds", s.apiOutboundsList)
	mux.HandleFunc("POST /api/outbounds", s.apiOutboundsAdd)
	mux.HandleFunc("PUT /api/outbounds/{tag}", s.apiOutboundsUpdate)
	mux.HandleFunc("DELETE /api/outbounds/{tag}", s.apiOutboundsDelete)

	// rules CRUD + reorder
	mux.HandleFunc("GET /api/rules", s.apiRulesList)
	mux.HandleFunc("POST /api/rules", s.apiRulesAdd)
	mux.HandleFunc("PUT /api/rules/{idx}", s.apiRulesUpdate)
	mux.HandleFunc("DELETE /api/rules/{idx}", s.apiRulesDelete)
	mux.HandleFunc("POST /api/rules/reorder", s.apiRulesReorder)

	// rule_sets
	mux.HandleFunc("GET /api/rule_sets", s.apiRuleSetsList)
	mux.HandleFunc("POST /api/rule_sets", s.apiRuleSetsAdd)
	mux.HandleFunc("DELETE /api/rule_sets/{tag}", s.apiRuleSetsDelete)

	// presets
	mux.HandleFunc("GET /api/presets", s.apiPresetsList)
	mux.HandleFunc("POST /api/presets/{name}/apply", s.apiPresetsApply)
	mux.HandleFunc("POST /api/presets/{name}/preview", s.apiPresetsPreview)

	// validate / preview / apply
	mux.HandleFunc("POST /api/validate", s.apiValidate)
	mux.HandleFunc("GET /api/preview", s.apiPreview)
	mux.HandleFunc("POST /api/apply", s.apiApply)

	// SPA: статика из embed.FS + fallback на index.html для SPA-маршрутов.
	// Все JS/CSS/vendor-файлы отдаются как есть; неизвестные пути → index.html.
	fsys := routingUIFS()
	fileServer := http.FileServer(http.FS(fsys))

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		urlPath := strings.TrimPrefix(r.URL.Path, "/")
		if urlPath == "" || urlPath == "index.html" {
			// Отдаём index.html напрямую
			spaServeFile(w, r, fsys, "index.html")
			return
		}
		// Если файл существует в embed — отдаём его
		if _, err := fs.Stat(fsys, urlPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Всё остальное — SPA fallback на index.html
		spaServeFile(w, r, fsys, "index.html")
	})
}

// spaServeFile отдаёт конкретный файл из embed.FS без redirect-логики
// http.FileServer (которая делает 301 для путей вида /index.html → /).
func spaServeFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	http.ServeFileFS(w, r, fsys, name)
}

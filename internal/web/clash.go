package web

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/version"
)

// singboxClashAPIAddr — адрес experimental.clash_api в sing-box (см. шаблон
// tun.json.tmpl). Запросы Zashboard к /proxies, /connections, /traffic, /logs
// и т.д. проксируются на этот адрес — sing-box сам отдаёт реальные данные.
const singboxClashAPIAddr = "http://127.0.0.1:9094"

// registerClashRoutes регистрирует роуты на порту 9090: SPA (Zashboard) +
// reverse-proxy всех Clash-API запросов в sing-box experimental.clash_api.
func registerClashRoutes(mux *http.ServeMux, s *Server) {
	spa := newSPAHandler()
	clashProxy := newClashAPIProxy()

	mux.Handle("GET /{$}", s.clashRoot(spa))

	// /config.js — runtime-конфиг встроенного MetaCubeXD/Zashboard SPA.
	// Подменяем статический файл из assets: defaultBackendURL = тот же origin,
	// иначе SPA при первом открытии показывает экран ввода backend и secret.
	mux.HandleFunc("GET /config.js", s.clashSPAConfig)

	// /version — собственный handler: совмещает sign-craze build info
	// с признаком "premium=false" (Zashboard ждёт это поле).
	mux.HandleFunc("GET /version", s.clashVersion)

	// Все остальные Clash-API эндпоинты проксируем в sing-box clash_api:
	// /configs, /proxies, /providers/*, /connections, /traffic, /logs,
	// /group, /rules, /cache, /dns/query — реальные данные из ядра.
	mux.Handle("/configs", clashProxy)
	mux.Handle("/proxies", clashProxy)
	mux.Handle("/proxies/", clashProxy)
	mux.Handle("/providers/", clashProxy)
	mux.Handle("/connections", clashProxy)
	mux.Handle("/connections/", clashProxy)
	mux.Handle("/traffic", clashProxy)
	mux.Handle("/memory", clashProxy)
	mux.Handle("/logs", clashProxy)
	mux.Handle("/group", clashProxy)
	mux.Handle("/group/", clashProxy)
	mux.Handle("/rules", clashProxy)
	mux.Handle("/rules/", clashProxy)
	mux.Handle("/cache", clashProxy)
	mux.Handle("/cache/", clashProxy)
	mux.Handle("/dns/query", clashProxy)
	mux.Handle("/profile/", clashProxy)
	mux.Handle("/script/", clashProxy)

	// SPA fallback: все остальные пути → встроенный Zashboard
	mux.Handle("/", spa)
}

// newClashAPIProxy создаёт ReverseProxy на sing-box clash_api.
// Поддерживает HTTP и WebSocket (httputil.ReverseProxy с Go 1.20+ работает
// прозрачно для Upgrade: websocket).
func newClashAPIProxy() http.Handler {
	target, err := url.Parse(singboxClashAPIAddr)
	if err != nil {
		panic(err)
	}
	rp := httputil.NewSingleHostReverseProxy(target)
	// Sing-box clash_api может отвечать 502/503 если sing-box ещё не запущен —
	// возвращаем понятный JSON, чтобы SPA показал "backend offline" а не
	// «Network Error».
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"message":"sing-box clash_api недоступен (port 9094)"}`)) //nolint:errcheck
	}
	return rp
}

// clashSPAConfig отдаёт config.js с defaultBackendURL = "http://<host>" —
// MetaCubeXD/Zashboard подключаются автоматически без формы ввода.
// Secret пустой: внешний доступ к 9090 закрыт правилом INPUT DROP на WAN_IF.
func (s *Server) clashSPAConfig(w http.ResponseWriter, r *http.Request) {
	host := r.Host // "192.168.1.1:9090"
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("window.__METACUBEXD_CONFIG__ = { defaultBackendURL: 'http://" + host + "', defaultSecret: '' };\n")) //nolint:errcheck
}

// clashRoot отдаёт SPA-страницу Zashboard, если клиент — браузер
// (Accept содержит text/html), иначе классический Clash hello-JSON.
// Это устраняет UX-проблему: ввод http://host:9090/ в браузере раньше
// возвращал {"hello":"clash"} JSON вместо UI.
func (s *Server) clashRoot(spa http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			spa.ServeHTTP(w, r)
			return
		}
		s.clashHello(w, r)
	})
}

func (s *Server) clashHello(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{
		"hello":   "clash",
		"version": "sign-craze/" + version.String(),
	})
}

func (s *Server) clashVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"version": version.String(),
		"premium": false,
	})
}

package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/version"
)

// registerClashRoutes регистрирует Clash-совместимые маршруты на порту 9090.
func registerClashRoutes(mux *http.ServeMux, s *Server) {
	spa := newSPAHandler()
	mux.Handle("GET /{$}", s.clashRoot(spa))
	mux.HandleFunc("GET /version", s.clashVersion)
	mux.HandleFunc("GET /configs", s.clashConfigs)
	mux.HandleFunc("GET /proxies", s.clashProxies)
	mux.HandleFunc("GET /connections", s.clashConnections)
	mux.HandleFunc("GET /traffic", s.clashTrafficWS)
	mux.HandleFunc("GET /logs", s.clashLogsWS)

	// /config.js — runtime-конфиг встроенного MetaCubeXD/Zashboard SPA.
	// Подменяем статический файл из assets, чтобы defaultBackendURL указывал
	// на тот же origin, с которого открыта страница (LAN IP роутера). Иначе
	// SPA при первом открытии показывает экран ввода backend и просит secret.
	mux.HandleFunc("GET /config.js", s.clashSPAConfig)

	// SPA fallback: все незарегистрированные пути → встроенный Zashboard
	mux.Handle("/", spa)
}

// clashSPAConfig отдаёт config.js с defaultBackendURL = "http://<host>" —
// MetaCubeXD/Zashboard подключаются автоматически без формы ввода.
// Secret пустой: внешний доступ к 9090 закрыт правилом INPUT DROP на WAN_IF.
func (s *Server) clashSPAConfig(w http.ResponseWriter, r *http.Request) {
	host := r.Host // "192.168.1.1:9090"
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("window.__METACUBEXD_CONFIG__ = { defaultBackendURL: 'http://" + host + "', defaultSecret: '' };\n"))
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

func (s *Server) clashConfigs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"mode":      "proxy",
		"port":      7895,
		"log-level": "info",
	})
}

func (s *Server) clashProxies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"proxies": map[string]any{}})
}

func (s *Server) clashConnections(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"downloadTotal": 0,
		"uploadTotal":   0,
		"connections":   []any{},
	})
}

func (s *Server) clashTrafficWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	frame, marshalErr := json.Marshal(map[string]int{"up": 0, "down": 0})
	if marshalErr != nil {
		return
	}

	// 5s write deadline ловит застрявших клиентов (slowloris) на 128MB роутере
	// без накопления горутин в TCP-буферах (safety-fixes #15).
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second)) //nolint:errcheck // best-effort, реальная ошибка ловится в SendText
			if sendErr := conn.SendText(frame); sendErr != nil {
				return
			}
		}
	}
}

func (s *Server) clashLogsWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrade(w, r)
	if err != nil {
		return
	}
	// Держим соединение открытым до ошибки; реальный поток логов — будущая задача.
	conn.Close()
}


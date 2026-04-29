package web

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/version"
)

// registerClashRoutes регистрирует Clash-совместимые маршруты на порту 9090.
func registerClashRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("GET /{$}", s.clashHello)
	mux.HandleFunc("GET /version", s.clashVersion)
	mux.HandleFunc("GET /configs", s.clashConfigs)
	mux.HandleFunc("GET /proxies", s.clashProxies)
	mux.HandleFunc("GET /connections", s.clashConnections)
	mux.HandleFunc("GET /traffic", s.clashTrafficWS)
	mux.HandleFunc("GET /logs", s.clashLogsWS)

	// SPA fallback: все незарегистрированные пути → встроенный Zashboard
	mux.Handle("/", newSPAHandler())
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

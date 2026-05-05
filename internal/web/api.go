package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// maxRequestBody — лимит размера тела POST-запросов admin API.
// Защита от DoS: 50MB malformed JSON убивает 128MB роутер (safety-fixes #7).
const maxRequestBody = 1 << 20 // 1 MB

// registerAdminRoutes регистрирует admin REST маршруты на порту 9091.
func registerAdminRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("GET /{$}", s.adminLanding)
	mux.HandleFunc("GET /api/status", s.apiStatus)
	mux.HandleFunc("GET /api/config", s.apiConfigGet)
	mux.HandleFunc("POST /api/config", s.apiConfigPost)
	mux.HandleFunc("GET /api/ports", s.apiPortsList)
	mux.HandleFunc("POST /api/ports", s.apiPortsAdd)
	mux.HandleFunc("DELETE /api/ports/{port}", s.apiPortsDel)
	mux.HandleFunc("GET /api/excludes", s.apiExcludesList)
	mux.HandleFunc("POST /api/excludes", s.apiExcludesAdd)
	mux.HandleFunc("DELETE /api/excludes/{cidr}", s.apiExcludesDel)
	mux.HandleFunc("GET /api/dpi/targets", s.apiDPITargetsGet)
	mux.HandleFunc("PUT /api/dpi/targets", s.apiDPITargetsPut)
	mux.HandleFunc("GET /api/dpi/presets", s.apiDPIPresetsList)
	mux.HandleFunc("POST /api/dpi/presets/{name}/apply", s.apiDPIPresetApply)
}

// adminLanding отдаёт минимальную HTML-страницу на GET / порта 9091.
// Сам admin REST не имеет UI; страница объясняет, где живут Zashboard (:9090)
// и Routing Editor (:9092), а также показывает доступные API-эндпоинты.
func (s *Server) adminLanding(w http.ResponseWriter, r *http.Request) {
	host, _, splitErr := net.SplitHostPort(r.Host)
	if splitErr != nil {
		host = r.Host
	}
	port := s.routingUIPort
	if port == 0 {
		port = 9092
	}
	page := fmt.Sprintf(`<!doctype html>
<html lang="ru"><head><meta charset="utf-8">
<title>sign-craze · admin REST API</title>
<style>body{font-family:system-ui,sans-serif;max-width:48rem;margin:2rem auto;padding:0 1rem;line-height:1.5}code{background:#f3f3f3;padding:.1em .3em;border-radius:3px}a{color:#0366d6}</style>
</head><body>
<h1>sign-craze · admin REST API</h1>
<p>Этот порт (<code>:9091</code>) — REST API без графического UI. Для управления используйте:</p>
<ul>
  <li><a href="http://%[1]s:9090/ui/">Zashboard</a> (Clash-совместимый дашборд) на <code>:9090</code></li>
  <li><a href="http://%[1]s:%[2]d/">Routing Editor</a> на <code>:%[2]d</code></li>
</ul>
<h2>Endpoints на :9091</h2>
<ul>
  <li><code>GET /api/status</code> — состояние сервиса</li>
  <li><code>GET /api/config</code>, <code>POST /api/config</code> — конфиг sing-box (raw JSON)</li>
  <li><code>GET /api/ports</code>, <code>POST /api/ports</code>, <code>DELETE /api/ports/{port}</code> — proxy-порты</li>
  <li><code>GET /api/excludes</code>, <code>POST /api/excludes</code>, <code>DELETE /api/excludes/{cidr}</code> — bypass-CIDR</li>
  <li><code>GET /api/dpi/targets</code>, <code>PUT /api/dpi/targets</code> — список доменов для selective DPI (nfqws2 --hostlist)</li>
  <li><code>GET /api/dpi/presets</code>, <code>POST /api/dpi/presets/{name}/apply</code> — встроенные пресеты (discord, youtube, discord-youtube)</li>
</ul>
</body></html>`, host, port)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, wErr := w.Write([]byte(page)); wErr != nil {
		slog.Debug("web: запись admin landing", "err", wErr)
	}
}

func (s *Server) apiStatus(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Status == nil {
		writeJSON(w, StatusInfo{})
		return
	}
	info, err := s.cfg.Status.Status(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, info)
}

func (s *Server) apiConfigGet(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Config == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	data, err := s.cfg.Config.ReadConfig(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if _, wErr := w.Write(data); wErr != nil {
		slog.Warn("web: запись config response", "err", wErr)
	}
}

func (s *Server) apiConfigPost(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Config == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		slog.Debug("web: decode config", "err", err)
		http.Error(w, "неверный JSON", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Config.WriteConfig(r.Context(), raw); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiPortsList(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ports == nil {
		writeJSON(w, []int{})
		return
	}
	ports, err := s.cfg.Ports.ListPorts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, ports)
}

func (s *Server) apiPortsAdd(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ports == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	var body struct {
		Port int `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		slog.Debug("web: decode port", "err", err)
		http.Error(w, "неверный JSON", http.StatusBadRequest)
		return
	}
	if body.Port < 1 || body.Port > 65535 {
		http.Error(w, "порт вне диапазона 1-65535", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Ports.AddPort(r.Context(), body.Port); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) apiPortsDel(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Ports == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	portStr := r.PathValue("port")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "неверный порт", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Ports.DeletePort(r.Context(), port); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiExcludesList(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Excludes == nil {
		writeJSON(w, []string{})
		return
	}
	excludes, err := s.cfg.Excludes.ListExcludes(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, excludes)
}

func (s *Server) apiExcludesAdd(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Excludes == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	var body struct {
		CIDR string `json:"cidr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		slog.Debug("web: decode exclude", "err", err)
		http.Error(w, "неверный JSON", http.StatusBadRequest)
		return
	}
	if body.CIDR == "" {
		http.Error(w, "cidr обязателен", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Excludes.AddExclude(r.Context(), body.CIDR); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) apiDPITargetsGet(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DPITargets == nil {
		writeJSON(w, []string{})
		return
	}
	targets, err := s.cfg.DPITargets.ListTargets(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if targets == nil {
		targets = []string{}
	}
	writeJSON(w, targets)
}

func (s *Server) apiDPITargetsPut(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DPITargets == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	var body struct {
		Targets []string `json:"targets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		slog.Debug("web: decode dpi targets", "err", err)
		http.Error(w, "неверный JSON", http.StatusBadRequest)
		return
	}
	if err := s.cfg.DPITargets.SetTargets(r.Context(), body.Targets); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) apiDPIPresetsList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, BuiltinDPIPresets)
}

func (s *Server) apiDPIPresetApply(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DPITargets == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	name := r.PathValue("name")
	var p *DPIPreset
	for i := range BuiltinDPIPresets {
		if BuiltinDPIPresets[i].Name == name {
			p = &BuiltinDPIPresets[i]
			break
		}
	}
	if p == nil {
		http.Error(w, "preset не найден", http.StatusNotFound)
		return
	}
	if err := s.cfg.DPITargets.SetTargets(r.Context(), p.Targets); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, p)
}

func (s *Server) apiExcludesDel(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Excludes == nil {
		http.Error(w, "не настроено", http.StatusNotImplemented)
		return
	}
	// PathValue возвращает URL-encoded значение; декодировать не нужно — "/" в CIDR не встречается
	cidr := r.PathValue("cidr")
	cidr = strings.ReplaceAll(cidr, "%2F", "/") // на случай URL-кодирования слэша
	if cidr == "" {
		http.Error(w, "cidr обязателен", http.StatusBadRequest)
		return
	}
	if err := s.cfg.Excludes.DeleteExclude(r.Context(), cidr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

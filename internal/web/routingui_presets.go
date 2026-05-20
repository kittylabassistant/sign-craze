package web

import (
	"encoding/json"
	"net/http"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/preset"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// presetApplyResponse возвращается из POST /api/presets/{name}/apply
// и POST /api/presets/{name}/preview.
// Warnings — incompatibility-сигналы (refilter rule_set для xray и т.д.),
// не блокируют apply.
type presetApplyResponse struct {
	Config   *types.RoutingConfig `json:"config"`
	Warnings []string             `json:"warnings,omitempty"`
}

// deepCopyRoutingConfig — JSON round-trip копия. Используется в preview,
// чтобы preset.ApplyToConfig не мутировал реальный cfg.
func deepCopyRoutingConfig(cfg *types.RoutingConfig) (*types.RoutingConfig, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var out types.RoutingConfig
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Server) apiPresetsList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, preset.BuiltinPresets)
}

// resolveVPNTag возвращает имя VPN-outbound для подстановки на место
// preset.VPNPlaceholder. Пустая строка означает, что VPN-outbound не
// сконфигурирован — caller должен вернуть 412.
func (s *Server) resolveVPNTag() string {
	if s.cfg.RoutingUI == nil || s.cfg.RoutingUI.DefaultOutboundTag == nil {
		return ""
	}
	return s.cfg.RoutingUI.DefaultOutboundTag()
}

// activeGeoFormat возвращает GeoFormat активного ядра через RoutingUIDeps.
// Fallback на GeoSRS (sing-box) если deps не настроены — обеспечивает
// backward compat в тестах, которые не предоставляют ActiveGeoFormat.
func (s *Server) activeGeoFormat() core.GeoFormat {
	if s.cfg.RoutingUI != nil && s.cfg.RoutingUI.ActiveGeoFormat != nil {
		return s.cfg.RoutingUI.ActiveGeoFormat()
	}
	return core.GeoSRS
}

// apiPresetsApply — POST /api/presets/{name}/apply
// Legacy endpoint: применяет пресет в режиме add (AS-IS) и сразу пишет в
// routing.json. UI больше не вызывает этот endpoint — он использует
// /preview + PUT /api/routing. Оставлен для backward compatibility.
//
// Per-core URL: rule_set URLs резолвятся через preset.RuleSetSources под активный
// c.GeoFormat() — sing-box (.srs), mihomo (.mrs), xray (translation в matcher,
// URL пустой). Несовместимости surfacedвыше через warnings в response body.
//
// Преобразования:
//   - Outbound == "{vpn}" заменяется на DefaultOutboundTag (тег первого outbound state).
//     При отсутствии VPN-outbound возвращается 412 Precondition Failed.
//   - Final пресета применяется только если в текущем конфиге Final пустой.
func (s *Server) apiPresetsApply(w http.ResponseWriter, r *http.Request) {
	cfg, p, vpnTag, ok := s.preparePresetApply(w, r)
	if !ok {
		return
	}
	warnings := preset.ApplyToConfig(cfg, p, vpnTag, s.activeGeoFormat(), preset.ModeAdd)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, presetApplyResponse{Config: cfg, Warnings: warnings})
}

// preparePresetApply выполняет общие проверки для apply/preview:
// resolve preset, fail-fast 412 для {vpn} без outbound, load cfg.
func (s *Server) preparePresetApply(w http.ResponseWriter, r *http.Request) (*types.RoutingConfig, *preset.Preset, string, bool) {
	name := r.PathValue("name")
	p := preset.Find(name)
	if p == nil {
		http.Error(w, "preset не найден", http.StatusNotFound)
		return nil, nil, "", false
	}
	vpnTag := s.resolveVPNTag()
	if preset.UsesVPN(p) && vpnTag == "" {
		http.Error(w,
			"preset требует VPN-outbound: добавьте VLESS/Trojan/Shadowsocks через --install или /api/outbounds, затем повторите apply",
			http.StatusPreconditionFailed)
		return nil, nil, "", false
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, nil, "", false
	}
	return cfg, p, vpnTag, true
}

// apiPresetsPreview — POST /api/presets/{name}/preview
// Тело: {"mode": "add" | "replace"} (default: "add").
// Применяет пресет к КОПИИ конфига и возвращает результат БЕЗ записи на диск.
// Используется UI для черновика — после preview клиент видит draft, может
// нажать Сохранить (PUT /api/routing) или Отмена.
func (s *Server) apiPresetsPreview(w http.ResponseWriter, r *http.Request) {
	cfg, p, vpnTag, ok := s.preparePresetApply(w, r)
	if !ok {
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if r.ContentLength > 0 {
		if !decodeJSON(w, r, &body) {
			return
		}
	}
	mode := body.Mode
	if mode == "" {
		mode = preset.ModeAdd
	}
	if mode != preset.ModeAdd && mode != preset.ModeReplace {
		http.Error(w, "mode должен быть \"add\" или \"replace\"", http.StatusBadRequest)
		return
	}
	preview, err := deepCopyRoutingConfig(cfg)
	if err != nil {
		http.Error(w, "preview copy: "+err.Error(), http.StatusInternalServerError)
		return
	}
	warnings := preset.ApplyToConfig(preview, p, vpnTag, s.activeGeoFormat(), mode)
	writeJSON(w, presetApplyResponse{Config: preview, Warnings: warnings})
}

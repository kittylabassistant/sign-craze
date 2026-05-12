package web

import (
	"encoding/json"
	"net/http"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// presetVPNPlaceholder — спецтег в preset.Rules[].Outbound, заменяемый при apply
// на актуальный VPN-outbound из RoutingUIDeps.DefaultOutboundTag (== state.Outbounds[0].Tag).
// Без placeholder'а пресеты были бы прибиты к тегу "proxy", который не существует
// в реальном sign-craze деплое (тег формируется из имени VLESS-конфига оператора).
const presetVPNPlaceholder = "{vpn}"

// ruleSetSource — мульти-форматный источник rule_set. URL выбирается per-core
// при apiPresetsApply через resolveRuleSetURL(tag, c.GeoFormat()).
//
// Источники:
//   - SRS: SagerNet/sing-{geosite,geoip} (rule-set ветка, daily обновление).
//   - DAT: пусто — xray использует translation RouteRule.RuleSet → geosite:/geoip:
//     matchers (см. internal/core/xray/render_rules.go). Префикс geosite-/geoip- в
//     Tag — единственный требуемый идентификатор; URL не нужен.
//   - MRS: MetaCubeX/meta-rules-dat (raw release ветка).
//
// Refilter-blocked-domains: SRS + MRS из 1andrevich/Re-filter-lists releases.
// Для xray (DAT) эквивалента нет — rule пропускается с warning через Validator.
type ruleSetSource struct {
	Tag      string
	SRS      string
	DAT      string // пусто для xray — translation в matcher делает render_rules.go
	MRS      string
	Behavior string // "domain" | "ipcidr" | "classical" — для mihomo rule-providers
}

var ruleSetSources = []ruleSetSource{
	{
		Tag:      "geosite-youtube",
		SRS:      "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-youtube.srs",
		MRS:      "https://github.com/MetaCubeX/meta-rules-dat/raw/meta/geo/geosite/youtube.mrs",
		Behavior: "domain",
	},
	{
		Tag:      "geosite-discord",
		SRS:      "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-discord.srs",
		MRS:      "https://github.com/MetaCubeX/meta-rules-dat/raw/meta/geo/geosite/discord.mrs",
		Behavior: "domain",
	},
	{
		Tag:      "geosite-category-ads-all",
		SRS:      "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads-all.srs",
		MRS:      "https://github.com/MetaCubeX/meta-rules-dat/raw/meta/geo/geosite/category-ads-all.mrs",
		Behavior: "domain",
	},
	{
		Tag:      "geoip-ru",
		SRS:      "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs",
		MRS:      "https://github.com/MetaCubeX/meta-rules-dat/raw/meta/geo/geoip/ru.mrs",
		Behavior: "ipcidr",
	},
	{
		Tag:      "refilter-blocked-domains",
		SRS:      "https://github.com/1andrevich/Re-filter-lists/releases/latest/download/ruleset-domain-refilter_domains.srs",
		MRS:      "https://github.com/1andrevich/Re-filter-lists/releases/latest/download/ruleset-domain-refilter_domains.mrs",
		Behavior: "domain",
		// DAT пусто — xray не имеет .dat-эквивалента для refilter.
		// rule пропускается с warning при render.
	},
}

// resolveRuleSetURL возвращает URL под активный формат + ok.
// ok=false означает "нет эквивалента для этого ядра" — caller должен emit warning
// и skip rule_set entry (rule переводится в matcher отдельно для xray).
//
// Для xray (GeoDAT) URL всегда пустой, но ok=true если префикс Tag совпадает с
// geosite-/geoip-. Это сигнал "rule_set entry skip, но rule с этим tag будет
// translated render_rules.go в matcher". Refilter (без префикса) → ok=false.
func resolveRuleSetURL(tag string, format core.GeoFormat) (url string, ok bool) {
	for _, src := range ruleSetSources {
		if src.Tag != tag {
			continue
		}
		switch format {
		case core.GeoSRS:
			return src.SRS, src.SRS != ""
		case core.GeoMRS:
			return src.MRS, src.MRS != ""
		case core.GeoDAT:
			// xray: URL не используется, но rule валиден если префикс известен.
			isGeosite := len(tag) > 8 && tag[:8] == "geosite-"
			isGeoip := len(tag) > 6 && tag[:6] == "geoip-"
			return "", isGeosite || isGeoip
		}
		return "", false
	}
	return "", false
}

// preset — преднастроенный набор правил, применяемый поверх текущего routing.json.
//
// RuleSets хранятся как logical-имена (Tag); URL и Format заполняются при apply
// через resolveRuleSetURL под активный c.GeoFormat(). Это делает preset
// core-agnostic — один и тот же preset работает на sing-box (.srs), mihomo (.mrs)
// и xray (translation в matcher).
type preset struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Source      string            `json:"source,omitempty"` // ссылка на community-источник
	Rules       []types.RouteRule `json:"rules"`
	RuleSetTags []string          `json:"rule_set_tags,omitempty"` // logical tags, URL резолвится при apply
	// Final — желаемый final outbound. Применяется только если в текущем
	// конфиге Final пустой (пресет не перезаписывает явный выбор оператора).
	// Поддерживается placeholder "{vpn}" — резолвится в DefaultOutboundTag.
	Final string `json:"final,omitempty"`
}

// builtinPresets — встроенные пресеты. RuleSet URLs резолвятся в apply через
// translation table выше.
var builtinPresets = []preset{
	{
		Name: "sign-craze-default",
		Description: "Стандартная маршрутизация sign-craze: блокированные домены → VPN (TUN/TProxy), " +
			"YouTube/Discord → direct (DPI обход через nfqws2 на firewall-уровне), " +
			"BitTorrent → direct, остальной трафик → direct.",
		Source: "https://github.com/SagerNet/sing-geosite, https://github.com/1andrevich/Re-filter-lists",
		Rules: []types.RouteRule{
			{Protocol: []string{"bittorrent"}, Outbound: "direct"},
			{RuleSet: []string{"geosite-youtube"}, Outbound: "direct"},
			{RuleSet: []string{"geosite-discord"}, Outbound: "direct"},
			{RuleSet: []string{"refilter-blocked-domains"}, Outbound: presetVPNPlaceholder},
		},
		RuleSetTags: []string{"geosite-youtube", "geosite-discord", "refilter-blocked-domains"},
		Final:       "direct",
	},
	{
		Name:        "block-ads",
		Description: "Блокировать рекламу (geosite-category-ads-all из v2fly).",
		Source:      "https://github.com/SagerNet/sing-geosite",
		Rules: []types.RouteRule{
			{RuleSet: []string{"geosite-category-ads-all"}, Action: "reject"},
		},
		RuleSetTags: []string{"geosite-category-ads-all"},
	},
	{
		Name:        "ru-direct",
		Description: "Российские IP (geoip-ru) идут напрямую, минуя VPN.",
		Source:      "https://github.com/SagerNet/sing-geoip",
		Rules: []types.RouteRule{
			{RuleSet: []string{"geoip-ru"}, Outbound: "direct"},
		},
		RuleSetTags: []string{"geoip-ru"},
	},
	{
		Name: "ru-direct-rest-vpn",
		Description: "Российские IP (geoip-ru) → direct, остальной трафик → VPN (final). " +
			"Требует VPN-outbound в state. Final применяется только если ещё не задан.",
		Source: "https://github.com/SagerNet/sing-geoip",
		Rules: []types.RouteRule{
			{RuleSet: []string{"geoip-ru"}, Outbound: "direct"},
		},
		RuleSetTags: []string{"geoip-ru"},
		Final:       presetVPNPlaceholder,
	},
	{
		Name:        "blocked-vpn",
		Description: "Заблокированные в РФ домены (Re-filter) → через VPN.",
		Source:      "https://github.com/1andrevich/Re-filter-lists",
		Rules: []types.RouteRule{
			{RuleSet: []string{"refilter-blocked-domains"}, Outbound: presetVPNPlaceholder},
		},
		RuleSetTags: []string{"refilter-blocked-domains"},
	},
	{
		Name:        "discord-vpn",
		Description: "Discord (geosite-discord) → через VPN.",
		Source:      "https://github.com/SagerNet/sing-geosite",
		Rules: []types.RouteRule{
			{RuleSet: []string{"geosite-discord"}, Outbound: presetVPNPlaceholder},
		},
		RuleSetTags: []string{"geosite-discord"},
	},
	{
		Name:        "torrents-direct",
		Description: "BitTorrent трафик (определяется sniff'ом) идёт напрямую, не через VPN.",
		Rules: []types.RouteRule{
			{Protocol: []string{"bittorrent"}, Outbound: "direct"},
		},
	},
	{
		Name:        "block-bogon-udp",
		Description: "Блокировать NetBIOS/SMB UDP-порты (135, 137-139) — лишний шум LAN.",
		Rules: []types.RouteRule{
			{Network: "udp", Port: []uint16{135, 137, 138, 139}, Action: "reject"},
		},
	},
}

// presetApplyResponse возвращается из POST /api/presets/{name}/apply
// и POST /api/presets/{name}/preview.
// Warnings — incompatibility-сигналы (refilter rule_set для xray и т.д.),
// не блокируют apply.
type presetApplyResponse struct {
	Config   *types.RoutingConfig `json:"config"`
	Warnings []string             `json:"warnings,omitempty"`
}

// Режимы применения пресета.
const (
	presetModeAdd     = "add"     // AS-IS: аддитивно добавить к существующему cfg.
	presetModeReplace = "replace" // TO-BE: очистить Rules, force-set Final.
)

// applyPresetToConfig применяет пресет p к cfg in-place под указанный mode.
// vpnTag используется для подстановки на место presetVPNPlaceholder.
// format определяет источник rule_set URL.
//
// mode="add" (AS-IS): Rules append, RuleSets — недостающие добавляются (dedupe по tag),
// Final ставится только если cfg.Final == "".
//
// mode="replace" (TO-BE): Rules = nil → append preset Rules, RuleSets — недостающие
// добавляются (custom RuleSets не очищаются — по решению пользователя),
// Final = resolved preset Final (force overwrite).
//
// Возвращает warnings — несовместимости rule_set с активным ядром (refilter+xray и т.п.).
func applyPresetToConfig(cfg *types.RoutingConfig, p *preset, vpnTag string,
	format core.GeoFormat, mode string,
) []string {
	var warnings []string

	if mode == presetModeReplace {
		cfg.Rules = nil
	}

	existingTags := map[string]bool{}
	for _, rs := range cfg.RuleSets {
		existingTags[rs.Tag] = true
	}

	for _, tag := range p.RuleSetTags {
		if existingTags[tag] {
			continue
		}
		url, ok := resolveRuleSetURL(tag, format)
		if !ok {
			warnings = append(warnings,
				"rule_set \""+tag+"\": нет совместимого URL для активного ядра, rule_set entry пропущен")
			continue
		}
		// Для xray (GeoDAT) URL пустой — translation работает напрямую через
		// RouteRule.RuleSet → geosite:/geoip: matcher в xray.Render.
		if url == "" {
			existingTags[tag] = true
			continue
		}
		cfg.RuleSets = append(cfg.RuleSets, types.RuleSetRef{
			Tag:            tag,
			Type:           "remote",
			Format:         "binary",
			URL:            url,
			DownloadDetour: "direct",
			UpdateInterval: "24h0m0s",
		})
		existingTags[tag] = true
	}

	for _, rule := range p.Rules {
		if rule.Outbound == presetVPNPlaceholder {
			rule.Outbound = vpnTag
		}
		cfg.Rules = append(cfg.Rules, rule)
	}

	if p.Final != "" {
		resolvedFinal := p.Final
		if resolvedFinal == presetVPNPlaceholder {
			resolvedFinal = vpnTag
		}
		if mode == presetModeReplace || cfg.Final == "" {
			cfg.Final = resolvedFinal
		}
	}

	return warnings
}

// findPreset возвращает указатель на встроенный пресет по имени или nil.
func findPreset(name string) *preset {
	for i := range builtinPresets {
		if builtinPresets[i].Name == name {
			return &builtinPresets[i]
		}
	}
	return nil
}

// deepCopyRoutingConfig — JSON round-trip копия. Используется в preview,
// чтобы applyPresetToConfig не мутировал реальный cfg.
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
	writeJSON(w, builtinPresets)
}

// resolveVPNTag возвращает имя VPN-outbound для подстановки на место
// presetVPNPlaceholder. Пустая строка означает, что VPN-outbound не
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
// Per-core URL: rule_set URLs резолвятся через ruleSetSources под активный
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
	warnings := applyPresetToConfig(cfg, p, vpnTag, s.activeGeoFormat(), presetModeAdd)
	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, presetApplyResponse{Config: cfg, Warnings: warnings})
}

// preparePresetApply выполняет общие проверки для apply/preview:
// resolve preset, fail-fast 412 для {vpn} без outbound, load cfg.
func (s *Server) preparePresetApply(w http.ResponseWriter, r *http.Request) (*types.RoutingConfig, *preset, string, bool) {
	name := r.PathValue("name")
	p := findPreset(name)
	if p == nil {
		http.Error(w, "preset не найден", http.StatusNotFound)
		return nil, nil, "", false
	}
	vpnTag := s.resolveVPNTag()
	if presetUsesVPN(p) && vpnTag == "" {
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
		mode = presetModeAdd
	}
	if mode != presetModeAdd && mode != presetModeReplace {
		http.Error(w, "mode должен быть \"add\" или \"replace\"", http.StatusBadRequest)
		return
	}
	preview, err := deepCopyRoutingConfig(cfg)
	if err != nil {
		http.Error(w, "preview copy: "+err.Error(), http.StatusInternalServerError)
		return
	}
	warnings := applyPresetToConfig(preview, p, vpnTag, s.activeGeoFormat(), mode)
	writeJSON(w, presetApplyResponse{Config: preview, Warnings: warnings})
}

// presetUsesVPN сообщает, опирается ли пресет на VPN-outbound (placeholder).
func presetUsesVPN(p *preset) bool {
	for _, r := range p.Rules {
		if r.Outbound == presetVPNPlaceholder {
			return true
		}
	}
	return p.Final == presetVPNPlaceholder
}

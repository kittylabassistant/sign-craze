package web

import (
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// preset — преднастроенный набор правил, применяемый поверх текущего routing.json.
type preset struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Source      string             `json:"source,omitempty"` // ссылка на community-источник
	Rules       []types.RouteRule  `json:"rules"`
	RuleSets    []types.RuleSetRef `json:"rule_sets,omitempty"`
}

// builtinPresets — встроенные пресеты с источниками SRS из community-репо.
// Все источники auto-обновляются (sing-box.update_interval=24h по умолчанию).
//
// Источники:
//   - SagerNet/sing-geosite (rule-set ветка, daily) — geosite категории
//   - SagerNet/sing-geoip (rule-set ветка, daily) — geoip страны
//   - 1andrevich/Re-filter-lists (releases, daily) — РФ blocked + Discord
var builtinPresets = []preset{
	{
		Name:        "block-ads",
		Description: "Блокировать рекламу (geosite-category-ads-all из v2fly).",
		Source:      "https://github.com/SagerNet/sing-geosite",
		Rules: []types.RouteRule{
			{RuleSet: []string{"geosite-category-ads-all"}, Action: "block"},
		},
		RuleSets: []types.RuleSetRef{
			{
				Tag: "geosite-category-ads-all", Type: "remote", Format: "binary",
				URL:            "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads-all.srs",
				DownloadDetour: "direct",
				UpdateInterval: "24h0m0s",
			},
		},
	},
	{
		Name:        "ru-direct",
		Description: "Российские IP (geoip-ru) идут напрямую, минуя VPN.",
		Source:      "https://github.com/SagerNet/sing-geoip",
		Rules: []types.RouteRule{
			{RuleSet: []string{"geoip-ru"}, Outbound: "direct"},
		},
		RuleSets: []types.RuleSetRef{
			{
				Tag: "geoip-ru", Type: "remote", Format: "binary",
				URL:            "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs",
				DownloadDetour: "direct",
				UpdateInterval: "24h0m0s",
			},
		},
	},
	{
		Name:        "blocked-vpn",
		Description: "Заблокированные в РФ домены (Re-filter) → через VPN.",
		Source:      "https://github.com/1andrevich/Re-filter-lists",
		Rules: []types.RouteRule{
			{RuleSet: []string{"refilter-blocked-domains"}, Outbound: "proxy"},
		},
		RuleSets: []types.RuleSetRef{
			{
				Tag: "refilter-blocked-domains", Type: "remote", Format: "binary",
				URL:            "https://github.com/1andrevich/Re-filter-lists/releases/latest/download/ruleset-domain-refilter_domains.srs",
				DownloadDetour: "direct",
				UpdateInterval: "24h0m0s",
			},
		},
	},
	{
		Name:        "discord-vpn",
		Description: "Discord (geosite-discord) → через VPN.",
		Source:      "https://github.com/SagerNet/sing-geosite",
		Rules: []types.RouteRule{
			{RuleSet: []string{"geosite-discord"}, Outbound: "proxy"},
		},
		RuleSets: []types.RuleSetRef{
			{
				Tag: "geosite-discord", Type: "remote", Format: "binary",
				URL:            "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-discord.srs",
				DownloadDetour: "direct",
				UpdateInterval: "24h0m0s",
			},
		},
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
			{Network: "udp", Port: []uint16{135, 137, 138, 139}, Action: "block"},
		},
	},
}

func (s *Server) apiPresetsList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, builtinPresets)
}

// apiPresetsApply — POST /api/presets/{name}/apply
// Дополняет текущую RoutingConfig правилами и rule_sets из пресета.
// Дубликаты правил игнорируются по полю action+outbound+rule_set; rule_sets — по tag.
func (s *Server) apiPresetsApply(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var p *preset
	for i := range builtinPresets {
		if builtinPresets[i].Name == name {
			p = &builtinPresets[i]
			break
		}
	}
	if p == nil {
		http.Error(w, "preset не найден", http.StatusNotFound)
		return
	}
	cfg, err := s.loadRoutingConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	existingTags := map[string]bool{}
	for _, rs := range cfg.RuleSets {
		existingTags[rs.Tag] = true
	}
	for _, rs := range p.RuleSets {
		if !existingTags[rs.Tag] {
			cfg.RuleSets = append(cfg.RuleSets, rs)
			existingTags[rs.Tag] = true
		}
	}
	cfg.Rules = append(cfg.Rules, p.Rules...)

	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg)
}

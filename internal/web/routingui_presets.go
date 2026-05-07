package web

import (
	"net/http"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// presetVPNPlaceholder — спецтег в preset.Rules[].Outbound, заменяемый при apply
// на актуальный VPN-outbound из RoutingUIDeps.DefaultOutboundTag (== state.Outbounds[0].Tag).
// Без placeholder'а пресеты были бы прибиты к тегу "proxy", который не существует
// в реальном sign-craze деплое (тег формируется из имени VLESS-конфига оператора).
const presetVPNPlaceholder = "{vpn}"

// preset — преднастроенный набор правил, применяемый поверх текущего routing.json.
type preset struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Source      string             `json:"source,omitempty"` // ссылка на community-источник
	Rules       []types.RouteRule  `json:"rules"`
	RuleSets    []types.RuleSetRef `json:"rule_sets,omitempty"`
	// Final — желаемый final outbound. Применяется только если в текущем
	// конфиге Final пустой (пресет не перезаписывает явный выбор оператора).
	// Поддерживается placeholder "{vpn}" — резолвится в DefaultOutboundTag.
	Final string `json:"final,omitempty"`
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
		Name: "sign-craze-default",
		Description: "Стандартная маршрутизация sign-craze: блокированные домены → VPN (TUN), " +
			"YouTube/Discord → direct (DPI обход через nfqws2 на firewall-уровне), " +
			"BitTorrent → direct, остальной трафик → direct.",
		Source: "https://github.com/SagerNet/sing-geosite, https://github.com/1andrevich/Re-filter-lists",
		Rules: []types.RouteRule{
			// BitTorrent ловится sniff'ом и идёт мимо VPN.
			{Protocol: []string{"bittorrent"}, Outbound: "direct"},
			// YouTube и Discord направляем на direct: трафик не уйдёт в TUN,
			// а на firewall-уровне керовские пакеты с keenMark попадают в
			// NFQUEUE → nfqws2 фрагментирует TLS ClientHello для обхода DPI.
			{RuleSet: []string{"geosite-youtube"}, Outbound: "direct"},
			{RuleSet: []string{"geosite-discord"}, Outbound: "direct"},
			// Заблокированные в РФ домены (Re-filter) → VPN-outbound.
			{RuleSet: []string{"refilter-blocked-domains"}, Outbound: presetVPNPlaceholder},
		},
		RuleSets: []types.RuleSetRef{
			{
				Tag: "geosite-youtube", Type: "remote", Format: "binary",
				URL:            "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-youtube.srs",
				DownloadDetour: "direct",
				UpdateInterval: "24h0m0s",
			},
			{
				Tag: "geosite-discord", Type: "remote", Format: "binary",
				URL:            "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-discord.srs",
				DownloadDetour: "direct",
				UpdateInterval: "24h0m0s",
			},
			{
				Tag: "refilter-blocked-domains", Type: "remote", Format: "binary",
				URL:            "https://github.com/1andrevich/Re-filter-lists/releases/latest/download/ruleset-domain-refilter_domains.srs",
				DownloadDetour: "direct",
				UpdateInterval: "24h0m0s",
			},
		},
		Final: "direct",
	},
	{
		Name:        "block-ads",
		Description: "Блокировать рекламу (geosite-category-ads-all из v2fly).",
		Source:      "https://github.com/SagerNet/sing-geosite",
		Rules: []types.RouteRule{
			{RuleSet: []string{"geosite-category-ads-all"}, Action: "reject"},
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
			{RuleSet: []string{"refilter-blocked-domains"}, Outbound: presetVPNPlaceholder},
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
			{RuleSet: []string{"geosite-discord"}, Outbound: presetVPNPlaceholder},
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
			{Network: "udp", Port: []uint16{135, 137, 138, 139}, Action: "reject"},
		},
	},
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

// apiPresetsApply — POST /api/presets/{name}/apply
// Дополняет текущую RoutingConfig правилами и rule_sets из пресета.
// Дубликаты правил игнорируются по полю action+outbound+rule_set; rule_sets — по tag.
//
// Преобразования:
//   - Outbound == "{vpn}" заменяется на DefaultOutboundTag (тег первого outbound state).
//     При отсутствии VPN-outbound возвращается 412 Precondition Failed.
//   - Final пресета применяется только если в текущем конфиге Final пустой.
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

	// Резолв placeholder VPN до загрузки конфига — fail fast.
	vpnTag := s.resolveVPNTag()
	if presetUsesVPN(p) && vpnTag == "" {
		http.Error(w,
			"preset требует VPN-outbound: добавьте VLESS/Trojan/Shadowsocks через --install или /api/outbounds, затем повторите apply",
			http.StatusPreconditionFailed)
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

	for _, rule := range p.Rules {
		if rule.Outbound == presetVPNPlaceholder {
			rule.Outbound = vpnTag
		}
		cfg.Rules = append(cfg.Rules, rule)
	}

	if cfg.Final == "" && p.Final != "" {
		final := p.Final
		if final == presetVPNPlaceholder {
			final = vpnTag
		}
		cfg.Final = final
	}

	if err := s.saveRoutingConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg)
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

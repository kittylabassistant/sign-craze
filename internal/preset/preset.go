// Package preset определяет встроенные routing-пресеты sign-craze и логику их
// применения к routing.json. Пакет используется и web UI, и CLI (--install --preset).
package preset

import (
	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// VPNPlaceholder — спецтег в Preset.Rules[].Outbound, заменяемый при apply
// на тег VPN-outbound (обычно state.Outbounds[0].Tag). Без placeholder'а пресеты
// были бы прибиты к тегу "proxy", который не существует в реальных конфигурациях
// sign-craze (тег формируется из имени VLESS-конфига оператора).
const VPNPlaceholder = "{vpn}"

// Режимы apply.
const (
	ModeAdd     = "add"     // AS-IS: аддитивно добавить к существующему cfg.
	ModeReplace = "replace" // TO-BE: очистить Rules, force-set Final.
)

// RuleSetSource — мульти-форматный источник rule_set. URL выбирается per-core
// при apply через ResolveRuleSetURL(tag, format).
//
// Источники:
//   - SRS: SagerNet/sing-{geosite,geoip} (rule-set ветка, daily обновление).
//   - MRS: MetaCubeX/meta-rules-dat (raw release ветка).
//
// Для xray (GeoDAT) URL не нужен — translation через RouteRule.RuleSet → geosite:/geoip:
// matchers в internal/core/xray/render_rules.go; поле Tag — единственный идентификатор.
// Refilter-blocked-domains: SRS + MRS из 1andrevich/Re-filter-lists releases.
// Для xray эквивалента нет — rule пропускается с warning через Validator.
type RuleSetSource struct {
	Tag string
	SRS string
	MRS string
}

// RuleSetSources — список источников rule_set, используемых пресетами.
var RuleSetSources = []RuleSetSource{
	{
		Tag: "geosite-youtube",
		SRS: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-youtube.srs",
		MRS: "https://github.com/MetaCubeX/meta-rules-dat/raw/meta/geo/geosite/youtube.mrs",
	},
	{
		Tag: "geosite-discord",
		SRS: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-discord.srs",
		MRS: "https://github.com/MetaCubeX/meta-rules-dat/raw/meta/geo/geosite/discord.mrs",
	},
	{
		Tag: "geosite-category-ads-all",
		SRS: "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ads-all.srs",
		MRS: "https://github.com/MetaCubeX/meta-rules-dat/raw/meta/geo/geosite/category-ads-all.mrs",
	},
	{
		Tag: "geoip-ru",
		SRS: "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs",
		MRS: "https://github.com/MetaCubeX/meta-rules-dat/raw/meta/geo/geoip/ru.mrs",
	},
	{
		Tag: "refilter-blocked-domains",
		SRS: "https://github.com/1andrevich/Re-filter-lists/releases/latest/download/ruleset-domain-refilter_domains.srs",
		MRS: "https://github.com/1andrevich/Re-filter-lists/releases/latest/download/ruleset-domain-refilter_domains.mrs",
	},
}

// Preset — преднастроенный набор правил, применяемый поверх текущего routing.json.
//
// RuleSets хранятся как logical-имена (Tag); URL и Format заполняются при apply
// через ResolveRuleSetURL под активный core.GeoFormat(). Это делает preset
// core-agnostic — один и тот же preset работает на sing-box (.srs), mihomo (.mrs)
// и xray (translation в matcher).
type Preset struct {
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

// BuiltinPresets — встроенные пресеты sign-craze. RuleSet URLs резолвятся в apply через
// translation table выше.
var BuiltinPresets = []Preset{
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
			{RuleSet: []string{"refilter-blocked-domains"}, Outbound: VPNPlaceholder},
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
		Final:       VPNPlaceholder,
	},
	{
		Name:        "blocked-vpn",
		Description: "Заблокированные в РФ домены (Re-filter) → через VPN.",
		Source:      "https://github.com/1andrevich/Re-filter-lists",
		Rules: []types.RouteRule{
			{RuleSet: []string{"refilter-blocked-domains"}, Outbound: VPNPlaceholder},
		},
		RuleSetTags: []string{"refilter-blocked-domains"},
	},
	{
		Name:        "discord-vpn",
		Description: "Discord (geosite-discord) → через VPN.",
		Source:      "https://github.com/SagerNet/sing-geosite",
		Rules: []types.RouteRule{
			{RuleSet: []string{"geosite-discord"}, Outbound: VPNPlaceholder},
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

// Find возвращает указатель на встроенный пресет по имени или nil.
func Find(name string) *Preset {
	for i := range BuiltinPresets {
		if BuiltinPresets[i].Name == name {
			return &BuiltinPresets[i]
		}
	}
	return nil
}

// List возвращает копию списка встроенных пресетов.
func List() []Preset { return append([]Preset(nil), BuiltinPresets...) }

// ResolveRuleSetURL возвращает URL под активный формат + ok.
// ok=false означает "нет эквивалента для этого ядра" — caller должен emit warning
// и skip rule_set entry (rule переводится в matcher отдельно для xray).
//
// Для xray (GeoDAT) URL всегда пустой, но ok=true если префикс Tag совпадает с
// geosite-/geoip-. Это сигнал "rule_set entry skip, но rule с этим tag будет
// translated render_rules.go в matcher". Refilter (без префикса) → ok=false.
func ResolveRuleSetURL(tag string, format core.GeoFormat) (url string, ok bool) {
	for _, src := range RuleSetSources {
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

// ApplyToConfig применяет preset p к cfg in-place под указанный mode.
// vpnTag используется для подстановки на место VPNPlaceholder.
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
func ApplyToConfig(cfg *types.RoutingConfig, p *Preset, vpnTag string,
	format core.GeoFormat, mode string,
) []string {
	var warnings []string

	if mode == ModeReplace {
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
		url, ok := ResolveRuleSetURL(tag, format)
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
		if rule.Outbound == VPNPlaceholder {
			rule.Outbound = vpnTag
		}
		cfg.Rules = append(cfg.Rules, rule)
	}

	if p.Final != "" {
		resolvedFinal := p.Final
		if resolvedFinal == VPNPlaceholder {
			resolvedFinal = vpnTag
		}
		if mode == ModeReplace || cfg.Final == "" {
			cfg.Final = resolvedFinal
		}
	}

	return warnings
}

// UsesVPN сообщает, опирается ли preset на VPN-outbound (placeholder).
func UsesVPN(p *Preset) bool {
	for _, r := range p.Rules {
		if r.Outbound == VPNPlaceholder {
			return true
		}
	}
	return p.Final == VPNPlaceholder
}

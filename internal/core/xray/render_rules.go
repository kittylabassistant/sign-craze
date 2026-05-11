package xray

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TranslateRoutingRules — публичная обёртка над renderXrayRoutingRules для
// использования из Validator (web layer): собирает warnings для surfac в UI.
// defaultTag — не используется внутри translation (зарезервирован для future
// fallback логики); xray.Render самостоятельно добавляет catch-all при пустом Final.
func TranslateRoutingRules(rc *types.RoutingConfig, defaultTag string) ([]map[string]any, []string) {
	_ = defaultTag // зарезервирован, см. комментарий
	return renderXrayRoutingRules(rc)
}

// renderXrayRoutingRules транслирует RoutingConfig.Rules в xray routing.rules.
// Final → catch-all rule с inboundTag=tproxy-in в конце.
// Catch-all fallback при пустом Final добавляется на уровне Render (не здесь).
//
// Translation map:
//   - Domain/DomainSuffix/DomainKeyword/DomainRegex → domain[] с префиксами
//     domain:full: / domain: / domain:keyword: / regexp:
//   - IPCIDR → ip[]
//   - SourceIPCIDR → source[]
//   - Port (uint16 slice) → "80,443" (CSV string)
//   - PortRange ("1000:2000") → "1000-2000" (xray использует "-" вместо ":")
//   - Network → "tcp"/"udp" (или unset для both)
//   - Protocol → protocol[]
//   - Inbound → inboundTag[]
//   - RuleSet["geosite-X"] → domain["geosite:X"] (translation)
//   - RuleSet["geoip-X"]   → ip["geoip:X"] (translation)
//   - RuleSet без префикса → пропускается с warning
//   - Action="reject" → outboundTag="block"
//   - Action="route"|"" + Outbound → outboundTag=Outbound
//   - Action="sniff"/"resolve"/"hijack-dns"/"block" → пропускается с warning
//     (xray делает sniff на уровне inbound, DNS — отдельный outbound type "dns")
//
// Возвращает: [rules, warnings]. Если rules пустой и Final пустой — caller должен
// добавить fallback catch-all "tproxy-in → defaultTag" (так делает Render).
func renderXrayRoutingRules(rc *types.RoutingConfig) ([]map[string]any, []string) {
	var rules []map[string]any
	var warnings []string

	for i, rr := range rc.Rules {
		rendered, w := renderXrayRule(rr)
		warnings = append(warnings, prefixWarnings(fmt.Sprintf("rules[%d]", i), w)...)
		if rendered != nil {
			rules = append(rules, rendered)
		}
	}

	// Final → catch-all rule в конце. Если Final пустой — Render добавит
	// fallback "tproxy-in → defaultTag" (defaultTag из p.DefaultTag).
	if rc.Final != "" {
		rules = append(rules, map[string]any{
			"type":        "field",
			"inboundTag":  []string{"tproxy-in"},
			"outboundTag": rc.Final,
		})
	}

	return rules, warnings
}

// renderXrayRule транслирует одно правило. Возвращает map (или nil если правило
// пропущено) + список warnings.
func renderXrayRule(r types.RouteRule) (map[string]any, []string) {
	var warnings []string

	// Resolve action → outboundTag
	var outboundTag string
	switch r.Action {
	case "reject", "block":
		outboundTag = "block"
	case "route", "":
		outboundTag = r.Outbound
	case "sniff", "resolve", "hijack-dns":
		warnings = append(warnings,
			fmt.Sprintf("action %q: xray не поддерживает на route-level (используется на inbound), правило пропущено", r.Action))
		return nil, warnings
	default:
		warnings = append(warnings, fmt.Sprintf("action %q: неизвестное значение, правило пропущено", r.Action))
		return nil, warnings
	}

	// Собираем matchers
	var domains, ips []string
	for _, d := range r.Domain {
		domains = append(domains, "domain:full:"+d)
	}
	for _, d := range r.DomainSuffix {
		domains = append(domains, "domain:"+d)
	}
	for _, d := range r.DomainKeyword {
		domains = append(domains, "domain:keyword:"+d)
	}
	for _, d := range r.DomainRegex {
		domains = append(domains, "regexp:"+d)
	}
	ips = append(ips, r.IPCIDR...)

	// RuleSet translation: geosite-X → "geosite:X", geoip-X → "geoip:X".
	// Без префикса — пропускаем с warning (xray не имеет rule_set механизма).
	for _, tag := range r.RuleSet {
		switch {
		case strings.HasPrefix(tag, "geosite-"):
			domains = append(domains, "geosite:"+strings.TrimPrefix(tag, "geosite-"))
		case strings.HasPrefix(tag, "geoip-"):
			ips = append(ips, "geoip:"+strings.TrimPrefix(tag, "geoip-"))
		default:
			warnings = append(warnings,
				fmt.Sprintf("rule_set %q: для xray поддерживаются только теги с префиксом geosite-/geoip- (например geosite-youtube)", tag))
		}
	}

	out := map[string]any{"type": "field"}
	if len(domains) > 0 {
		out["domain"] = domains
	}
	if len(ips) > 0 {
		out["ip"] = ips
	}
	if len(r.SourceIPCIDR) > 0 {
		out["source"] = append([]string(nil), r.SourceIPCIDR...)
	}
	if r.Network != "" {
		out["network"] = r.Network
	}
	if len(r.Protocol) > 0 {
		out["protocol"] = append([]string(nil), r.Protocol...)
	}
	if len(r.Inbound) > 0 {
		out["inboundTag"] = append([]string(nil), r.Inbound...)
	}

	// Ports: уикальный slice + CSV.
	var portParts []string
	for _, p := range r.Port {
		portParts = append(portParts, strconv.FormatUint(uint64(p), 10))
	}
	for _, pr := range r.PortRange {
		// xray использует "1000-2000" (с "-"), а не "1000:2000".
		portParts = append(portParts, strings.Replace(pr, ":", "-", 1))
	}
	if len(portParts) > 0 {
		out["port"] = strings.Join(portParts, ",")
	}

	hasConditions := len(domains) > 0 || len(ips) > 0 || len(r.SourceIPCIDR) > 0 ||
		r.Network != "" || len(r.Protocol) > 0 || len(r.Inbound) > 0 || len(portParts) > 0

	// Если user указал RuleSet, но все entries пропущены (без префикса geosite-/geoip-)
	// и других matchers нет — rule превратился бы в catch-all, что не было намерением.
	// Пропускаем с warning'ом который уже добавлен про rule_set.
	if len(r.RuleSet) > 0 && !hasConditions {
		return nil, warnings
	}

	// outbound пустой без conditions — невалидный rule.
	if outboundTag == "" {
		if hasConditions {
			warnings = append(warnings, "outbound пустой при наличии conditions, правило пропущено")
		}
		return nil, warnings
	}

	out["outboundTag"] = outboundTag
	return out, warnings
}

// prefixWarnings добавляет префикс (например "rules[0]") к каждой warning-строке.
func prefixWarnings(prefix string, ws []string) []string {
	if len(ws) == 0 {
		return nil
	}
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = prefix + ": " + w
	}
	return out
}

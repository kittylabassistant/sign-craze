package mihomo

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// RuleProvider — одно entry для секции rule-providers в config.yaml.
// Имя совпадает с tag в RULE-SET,<tag>,<action> правиле.
type RuleProvider struct {
	Tag      string
	Type     string // "http" — единственный supported для remote
	Behavior string // "classical" (универсальный, поддерживает domain+ip+process matchers)
	Format   string // "mrs" — единственный supported для mihomo
	URL      string
	Path     string // локальный путь относительно ConfigDir
	Interval int    // секунды между обновлениями
}

// TranslateRoutingRules транслирует RoutingConfig в mihomo rule-strings +
// rule-providers map. Final → MATCH,<final> в конце.
//
// Translation map:
//   - Domain        → DOMAIN,x,action
//   - DomainSuffix  → DOMAIN-SUFFIX,x,action
//   - DomainKeyword → DOMAIN-KEYWORD,x,action
//   - DomainRegex   → DOMAIN-REGEX,x,action
//   - IPCIDR (v4)   → IP-CIDR,x/n,action,no-resolve
//   - IPCIDR (v6)   → IP-CIDR6,x/n,action,no-resolve
//   - SourceIPCIDR  → SRC-IP-CIDR,x/n,action
//   - Port (uint16) → DST-PORT,80,action
//   - PortRange     → DST-PORT,1000-2000,action (mihomo поддерживает "-")
//   - Network       → NETWORK,udp,action
//   - Protocol      → пропуск с warning (нет matcher на rule-level)
//   - RuleSet       → RULE-SET,tag,action + provider entry
//
// Action mapping:
//   - "reject" / "block" → REJECT
//   - "route" / "" + Outbound → <Outbound> (или DIRECT для outbound=="direct")
//   - "sniff" / "resolve" / "hijack-dns" → пропуск с warning
//
// Если RuleSet URL не .mrs (например .srs или пустой) — provider не создаётся,
// rule пропускается с warning. Пользователь видит в UI через Validator.
func TranslateRoutingRules(rc *types.RoutingConfig, defaultTag string) (rules []string, providers []RuleProvider, warnings []string) {
	if rc == nil {
		return nil, nil, nil
	}

	// providersByTag нужен для де-дупликации — несколько правил могут ссылаться
	// на один rule_set, в config.yaml провайдер должен быть один.
	providersByTag := make(map[string]RuleProvider, len(rc.RuleSets))

	// Pre-build provider map из RuleSets (только .mrs URL).
	for _, rs := range rc.RuleSets {
		switch {
		case rs.URL == "":
			warnings = append(warnings,
				fmt.Sprintf("rule_set %q: пустой URL — provider пропущен (apply preset для активного ядра?)", rs.Tag))
			continue
		case !strings.HasSuffix(rs.URL, ".mrs"):
			warnings = append(warnings,
				fmt.Sprintf("rule_set %q: формат URL не .mrs (получено %q) — mihomo требует .mrs, provider пропущен",
					rs.Tag, fileExt(rs.URL)))
			continue
		}
		providersByTag[rs.Tag] = RuleProvider{
			Tag:      rs.Tag,
			Type:     "http",
			Behavior: "classical",
			Format:   "mrs",
			URL:      rs.URL,
			Path:     "./ruleset/" + rs.Tag + ".mrs",
			Interval: parseInterval(rs.UpdateInterval),
		}
	}

	for i, rr := range rc.Rules {
		action, w := resolveAction(rr)
		warnings = append(warnings, prefixWarnings(fmt.Sprintf("rules[%d]", i), w)...)
		if action == "" {
			continue
		}
		ruleLines, w := emitRuleLines(rr, action, providersByTag)
		warnings = append(warnings, prefixWarnings(fmt.Sprintf("rules[%d]", i), w)...)
		rules = append(rules, ruleLines...)
	}

	// Final → MATCH,<final> в конце.
	final := rc.Final
	if final == "" {
		final = defaultTag
	}
	if final != "" {
		rules = append(rules, "MATCH,"+normalizeAction(final))
	}

	// Provider list для config.yaml.tmpl — отсортирован по Tag для детерминизма.
	for _, rs := range rc.RuleSets {
		if p, ok := providersByTag[rs.Tag]; ok {
			providers = append(providers, p)
			delete(providersByTag, rs.Tag) // против дубликатов
		}
	}

	return rules, providers, warnings
}

// resolveAction возвращает mihomo action-строку (REJECT/DIRECT/<tag>) и warnings.
// Пустая строка означает "пропустить правило".
func resolveAction(r types.RouteRule) (string, []string) {
	var warnings []string
	switch r.Action {
	case "reject", "block":
		return "REJECT", nil
	case "route", "":
		if r.Outbound == "" {
			warnings = append(warnings, "outbound пустой при action=route, правило пропущено")
			return "", warnings
		}
		return normalizeAction(r.Outbound), nil
	case "sniff", "resolve", "hijack-dns":
		warnings = append(warnings,
			fmt.Sprintf("action %q: mihomo не поддерживает на rule-level (sniff на listener, dns в dns:), правило пропущено", r.Action))
		return "", warnings
	default:
		warnings = append(warnings, fmt.Sprintf("action %q: неизвестное значение, правило пропущено", r.Action))
		return "", warnings
	}
}

// normalizeAction переводит outbound tag в mihomo built-in action если нужно.
// "direct" → "DIRECT" (mihomo резервирует upper-case для built-in), "block" → "REJECT".
// Прочие теги passthrough — должны совпадать с именем proxy/proxy-group в config.yaml.
func normalizeAction(outbound string) string {
	switch outbound {
	case "direct":
		return "DIRECT"
	case "block":
		return "REJECT"
	}
	return outbound
}

// emitRuleLines генерирует mihomo rule-строки для одного RouteRule.
// Каждый matcher → отдельная строка (mihomo не поддерживает агрегацию domain+ip
// в одном правиле, в отличие от sing-box).
func emitRuleLines(r types.RouteRule, action string, providers map[string]RuleProvider) ([]string, []string) {
	var lines []string
	var warnings []string

	for _, d := range r.Domain {
		lines = append(lines, fmt.Sprintf("DOMAIN,%s,%s", d, action))
	}
	for _, d := range r.DomainSuffix {
		// mihomo DOMAIN-SUFFIX matches both bare and dotted forms; trim leading dot
		// для соответствия sing-box semantics (".x.com" == "x.com").
		lines = append(lines, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", strings.TrimPrefix(d, "."), action))
	}
	for _, d := range r.DomainKeyword {
		lines = append(lines, fmt.Sprintf("DOMAIN-KEYWORD,%s,%s", d, action))
	}
	for _, d := range r.DomainRegex {
		lines = append(lines, fmt.Sprintf("DOMAIN-REGEX,%s,%s", d, action))
	}

	for _, cidr := range r.IPCIDR {
		ruleType := "IP-CIDR"
		if prefix, err := netip.ParsePrefix(cidr); err == nil && prefix.Addr().Is6() {
			ruleType = "IP-CIDR6"
		}
		lines = append(lines, fmt.Sprintf("%s,%s,%s,no-resolve", ruleType, cidr, action))
	}
	for _, cidr := range r.SourceIPCIDR {
		lines = append(lines, fmt.Sprintf("SRC-IP-CIDR,%s,%s,no-resolve", cidr, action))
	}

	for _, port := range r.Port {
		lines = append(lines, fmt.Sprintf("DST-PORT,%s,%s", strconv.FormatUint(uint64(port), 10), action))
	}
	for _, pr := range r.PortRange {
		// "1000:2000" → "1000-2000" (mihomo формат)
		lines = append(lines, fmt.Sprintf("DST-PORT,%s,%s", strings.Replace(pr, ":", "-", 1), action))
	}

	if r.Network != "" {
		lines = append(lines, fmt.Sprintf("NETWORK,%s,%s", r.Network, action))
	}

	if len(r.Protocol) > 0 {
		warnings = append(warnings,
			fmt.Sprintf("protocol matcher %v: mihomo не поддерживает на rule-level (используется sniffer на inbound), matcher пропущен", r.Protocol))
	}

	if len(r.Inbound) > 0 {
		warnings = append(warnings,
			fmt.Sprintf("inbound matcher %v: mihomo не поддерживает named inbound matchers, matcher пропущен", r.Inbound))
	}

	for _, tag := range r.RuleSet {
		if _, ok := providers[tag]; !ok {
			warnings = append(warnings,
				fmt.Sprintf("rule_set %q: provider не зарегистрирован (формат не .mrs или URL пустой), matcher пропущен", tag))
			continue
		}
		lines = append(lines, fmt.Sprintf("RULE-SET,%s,%s", tag, action))
	}

	return lines, warnings
}

// fileExt возвращает суффикс пути после последней точки, для diagnostic warnings.
func fileExt(url string) string {
	if idx := strings.LastIndex(url, "."); idx != -1 {
		return url[idx:]
	}
	return "(no ext)"
}

// parseInterval парсит UpdateInterval ("24h0m0s") в секунды.
// При ошибке возвращает 86400 (24h) — разумный дефолт.
func parseInterval(s string) int {
	if s == "" {
		return 86400
	}
	// Простой парсер: ищем числа+unit, h/m/s.
	var total int
	var num int
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			num = num*10 + int(c-'0')
		case c == 'h':
			total += num * 3600
			num = 0
		case c == 'm':
			total += num * 60
			num = 0
		case c == 's':
			total += num
			num = 0
		}
	}
	if total <= 0 {
		return 86400
	}
	return total
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

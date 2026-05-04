package singbox

import (
	"encoding/json"
	"fmt"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// effectiveModel — данные для шаблона tun.json.tmpl. Унифицирует legacy и новый путь.
// BaseRules и UserRules хранятся как строки (уже сериализованный JSON),
// чтобы шаблон выводил их напрямую через {{ . }} без лишнего экранирования.
type effectiveModel struct {
	LogLevel         string
	TUNInterfaceName string
	TUNAddresses     []string
	TUNMTU           int

	Inbounds  []types.Inbound  // если пусто — шаблон рендерит default TUN
	Outbounds []types.Outbound // служебный "direct" добавляется программно в шаблоне
	BaseRules []string         // resolve, sniff, hijack-dns — уже сериализованный JSON
	UserRules []string         // пользовательские правила — уже сериализованный JSON
	RuleSets  []types.RuleSetRef
	Final     string

	DNSDirectRuleSets []string // legacy путь: GeoSiteDirect → local DNS
}

// marshalRule сериализует значение в JSON-строку для хранения в BaseRules/UserRules.
func marshalRule(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// buildEffectiveModel строит effectiveModel из ConfigParams, объединяя
// legacy-путь (Routing/Outbounds) и новый путь (RoutingConfig).
func buildEffectiveModel(p ConfigParams) (effectiveModel, error) {
	m := effectiveModel{
		LogLevel:         p.LogLevel,
		TUNInterfaceName: p.TUNInterfaceName,
		TUNAddresses:     p.TUNAddresses,
		TUNMTU:           p.TUNMTU,
	}

	// определяем тег базового inbound для base rules
	var baseInbound string
	if p.RoutingConfig != nil && len(p.RoutingConfig.Inbounds) > 0 {
		m.Inbounds = p.RoutingConfig.Inbounds
		baseInbound = p.RoutingConfig.Inbounds[0].Tag
	} else {
		baseInbound = "tun-in"
	}

	// base rules — всегда присутствуют (resolve, sniff, hijack-dns)
	base := []map[string]any{
		{"inbound": baseInbound, "action": "resolve", "strategy": "prefer_ipv4"},
		{"inbound": baseInbound, "action": "sniff"},
		{"inbound": baseInbound, "protocol": "dns", "action": "hijack-dns"},
	}
	for _, b := range base {
		s, err := marshalRule(b)
		if err != nil {
			return m, fmt.Errorf("marshal base rule: %w", err)
		}
		m.BaseRules = append(m.BaseRules, s)
	}

	if p.RoutingConfig != nil {
		// новый путь: RoutingConfig имеет приоритет
		m.Outbounds = p.RoutingConfig.Outbounds
		for _, r := range p.RoutingConfig.Rules {
			s, err := marshalRule(r)
			if err != nil {
				return m, fmt.Errorf("marshal user rule: %w", err)
			}
			m.UserRules = append(m.UserRules, s)
		}
		m.RuleSets = p.RoutingConfig.RuleSets
		if p.RoutingConfig.Final != "" {
			m.Final = p.RoutingConfig.Final
		} else {
			m.Final = p.DefaultOutboundTag
		}
	} else {
		// legacy путь: Routing + Outbounds
		m.Outbounds = p.Outbounds
		if len(p.Routing.GeoSiteDirect) > 0 {
			s, err := marshalRule(map[string]any{
				"rule_set": p.Routing.GeoSiteDirect,
				"outbound": "direct",
			})
			if err != nil {
				return m, fmt.Errorf("legacy GeoSiteDirect rule: %w", err)
			}
			m.UserRules = append(m.UserRules, s)
		}
		if len(p.Routing.GeoSiteProxy) > 0 {
			s, err := marshalRule(map[string]any{
				"rule_set": p.Routing.GeoSiteProxy,
				"outbound": p.DefaultOutboundTag,
			})
			if err != nil {
				return m, fmt.Errorf("legacy GeoSiteProxy rule: %w", err)
			}
			m.UserRules = append(m.UserRules, s)
		}
		m.RuleSets = buildRuleSets(p.Routing)
		if p.Routing.FinalOutbound != "" {
			m.Final = p.Routing.FinalOutbound
		} else {
			m.Final = p.DefaultOutboundTag
		}
		m.DNSDirectRuleSets = p.Routing.GeoSiteDirect
	}

	return m, nil
}

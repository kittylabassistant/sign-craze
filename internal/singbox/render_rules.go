package singbox

import (
	"encoding/json"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/routing"
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
	Outbounds []types.Outbound // служебный "direct" добавляется программно в шаблоне, если HasDirect=false
	HasDirect bool             // true → пользователь уже определил outbound с tag=direct, шаблон не дублирует
	BaseRules []string         // resolve, sniff, hijack-dns — уже сериализованный JSON
	UserRules []string         // пользовательские правила — уже сериализованный JSON
	RuleSets  []types.RuleSetRef
	Final     string
}

// outboundsHaveDirect возвращает true, если среди списка есть outbound с tag=direct.
// Нужен для подавления auto-direct в шаблоне tun.json.tmpl, иначе sing-box check
// падает с "duplicate outbound/endpoint tag: direct".
func outboundsHaveDirect(obs []types.Outbound) bool {
	for _, o := range obs {
		if o.Tag == "direct" {
			return true
		}
	}
	return false
}

// marshalRule сериализует значение в JSON-строку для хранения в BaseRules/UserRules.
func marshalRule(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// normalizeRouteRule приводит RouteRule к формату, понятному sing-box ≥1.11.
// Преобразует legacy `action: "block"` → `"reject"` (sing-box убрал поддержку
// "block" в route.rules — оно вызывает FATAL "unknown rule action: block").
// Сохранён для обратной совместимости с уже записанными routing.json.
func normalizeRouteRule(r types.RouteRule) types.RouteRule {
	if r.Action == "block" {
		r.Action = "reject"
	}
	return r
}

// normalizeInbound приводит Inbound.Settings к формату sing-box ≥1.12.
// inet4_address/inet6_address удалены, заменены на единое поле address []string.
// Без миграции sing-box 1.12+ падает FATAL "legacy tun address fields are deprecated".
// Применяется к копии Settings — исходный объект не мутируется.
func normalizeInbound(in types.Inbound) types.Inbound {
	if in.Type != "tun" || len(in.Settings) == 0 {
		return in
	}
	settings := make(map[string]any, len(in.Settings))
	var legacy []string
	for k, v := range in.Settings {
		switch k {
		case "inet4_address", "inet6_address":
			if s, ok := v.(string); ok && s != "" {
				legacy = append(legacy, s)
			}
		default:
			settings[k] = v
		}
	}
	if len(legacy) > 0 {
		if existing, ok := settings["address"]; ok {
			if arr, ok := existing.([]string); ok {
				legacy = append(arr, legacy...)
			} else if arr, ok := existing.([]any); ok {
				for _, x := range arr {
					if s, ok := x.(string); ok {
						legacy = append(legacy, s)
					}
				}
			}
		}
		settings["address"] = legacy
	}
	in.Settings = settings
	return in
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
		normalized := make([]types.Inbound, len(p.RoutingConfig.Inbounds))
		for i, in := range p.RoutingConfig.Inbounds {
			normalized[i] = normalizeInbound(in)
		}
		m.Inbounds = normalized
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
		m.HasDirect = outboundsHaveDirect(m.Outbounds)
		for _, r := range p.RoutingConfig.Rules {
			s, err := marshalRule(normalizeRouteRule(r))
			if err != nil {
				return m, fmt.Errorf("marshal user rule: %w", err)
			}
			m.UserRules = append(m.UserRules, s)
		}
		// Применяем фильтр: удаляем unreferenced rule_sets из конфига.
		// Это предотвращает загрузку sing-box rule_sets которые не используются
		// ни в одном route rule (инцидент 2026-05-12: unused rule_sets → 404 → клиенты теряют интернет).
		routing.FilterUnusedRuleSets(p.RoutingConfig)
		m.RuleSets = p.RoutingConfig.RuleSets
		if p.RoutingConfig.Final != "" {
			m.Final = p.RoutingConfig.Final
		} else {
			m.Final = p.DefaultOutboundTag
		}
	} else {
		// Нет RoutingConfig: используем только Outbounds + DefaultOutboundTag.
		// Валидация Outbounds выполнена выше в Render().
		m.Outbounds = p.Outbounds
		m.HasDirect = outboundsHaveDirect(m.Outbounds)
		m.Final = p.DefaultOutboundTag
	}

	return m, nil
}

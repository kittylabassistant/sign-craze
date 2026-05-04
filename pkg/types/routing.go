package types

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// inboundTagRe — переиспользуем outboundTagRe (тот же формат тега).
var inboundTagRe = outboundTagRe

// validActions — допустимые значения action в RouteRule.
var validActions = map[string]bool{
	"":           true,
	"route":      true,
	"block":      true,
	"sniff":      true,
	"resolve":    true,
	"hijack-dns": true,
	"reject":     true,
}

// routeActionNeedsOutbound — действия, при которых Outbound обязателен
// (если есть хотя бы одно условие).
var routeActionNeedsOutbound = map[string]bool{
	"":      true,
	"route": true,
}

// RoutingConfig — пользовательская routing-конфигурация sing-box,
// загружается из /opt/etc/sign-craze/routing.json. Если nil — sign-craze
// использует legacy RoutingRules (backward compat).
type RoutingConfig struct {
	Version   int          `json:"version"`
	Inbounds  []Inbound    `json:"inbounds,omitempty"`
	Outbounds []Outbound   `json:"outbounds,omitempty"`
	Rules     []RouteRule  `json:"rules,omitempty"`
	RuleSets  []RuleSetRef `json:"rule_sets,omitempty"`
	Final     string       `json:"final,omitempty"`
}

// Inbound — sing-box inbound (tun, tproxy, mixed, socks, http и т.д.).
// Settings мерджатся в top-level JSON через MarshalJSON, как у Outbound.
type Inbound struct {
	Tag      string         `json:"tag"`
	Type     string         `json:"type"`
	Settings map[string]any `json:"-"`
}

// MarshalJSON эмитит Inbound в формате sing-box: tag/type + Settings
// мерджатся как top-level поля.
func (in Inbound) MarshalJSON() ([]byte, error) {
	out := map[string]any{
		"tag":  in.Tag,
		"type": in.Type,
	}
	for k, v := range in.Settings {
		out[k] = v
	}
	return json.Marshal(out)
}

// UnmarshalJSON парсит Inbound из JSON, симметрично MarshalJSON: pop tag, pop type,
// остаток помещается в Settings.
func (in *Inbound) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	pop := func(key string, dst any) error {
		v, ok := raw[key]
		if !ok {
			return nil
		}
		delete(raw, key)
		return json.Unmarshal(v, dst)
	}
	if err := pop("tag", &in.Tag); err != nil {
		return fmt.Errorf("inbound.tag: %w", err)
	}
	if err := pop("type", &in.Type); err != nil {
		return fmt.Errorf("inbound.type: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}
	in.Settings = make(map[string]any, len(raw))
	for k, v := range raw {
		var x any
		if err := json.Unmarshal(v, &x); err != nil {
			return fmt.Errorf("inbound.%s: %w", k, err)
		}
		in.Settings[k] = x
	}
	return nil
}

// Validate проверяет обязательные поля Inbound.
func (in Inbound) Validate() error {
	if in.Tag == "" {
		return fmt.Errorf("inbound.Tag не может быть пустым")
	}
	if !inboundTagRe.MatchString(in.Tag) {
		return fmt.Errorf("inbound.Tag %q: допустимы только [a-zA-Z0-9._-], 1-64 символов", in.Tag)
	}
	if in.Type == "" {
		return fmt.Errorf("inbound.Type не может быть пустым")
	}
	return nil
}

// RouteRule — одно правило sing-box route.rules[].
type RouteRule struct {
	Tag           string   `json:"tag,omitempty"`
	Action        string   `json:"action,omitempty"`
	Outbound      string   `json:"outbound,omitempty"`
	Inbound       []string `json:"inbound,omitempty"`
	Network       string   `json:"network,omitempty"`
	Protocol      []string `json:"protocol,omitempty"`
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	DomainRegex   []string `json:"domain_regex,omitempty"`
	IPCIDR        []string `json:"ip_cidr,omitempty"`
	SourceIPCIDR  []string `json:"source_ip_cidr,omitempty"`
	Port          []uint16 `json:"port,omitempty"`
	PortRange     []string `json:"port_range,omitempty"`
	SourcePort    []uint16 `json:"source_port,omitempty"`
	RuleSet       []string `json:"rule_set,omitempty"`
	Invert        bool     `json:"invert,omitempty"`
}

// hasConditions возвращает true, если в правиле задано хотя бы одно условие фильтрации.
func (r RouteRule) hasConditions() bool {
	return len(r.Inbound) > 0 ||
		r.Network != "" ||
		len(r.Protocol) > 0 ||
		len(r.Domain) > 0 ||
		len(r.DomainSuffix) > 0 ||
		len(r.DomainKeyword) > 0 ||
		len(r.DomainRegex) > 0 ||
		len(r.IPCIDR) > 0 ||
		len(r.SourceIPCIDR) > 0 ||
		len(r.Port) > 0 ||
		len(r.PortRange) > 0 ||
		len(r.SourcePort) > 0 ||
		len(r.RuleSet) > 0
}

// Validate проверяет корректность RouteRule.
func (r RouteRule) Validate() error {
	// проверяем action
	if !validActions[r.Action] {
		return fmt.Errorf("route_rule.action %q: недопустимое значение (допустимо: route, block, sniff, resolve, hijack-dns, reject)", r.Action)
	}
	// проверяем network
	switch r.Network {
	case "", "tcp", "udp":
	default:
		return fmt.Errorf("route_rule.network %q: недопустимое значение (допустимо: tcp, udp)", r.Network)
	}
	// если action требует outbound и есть условия — outbound обязателен
	if routeActionNeedsOutbound[r.Action] && r.hasConditions() && r.Outbound == "" {
		return fmt.Errorf("route_rule: outbound обязателен при action=%q и наличии условий", r.Action)
	}
	// валидация CIDR-префиксов
	for _, cidr := range r.IPCIDR {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			return fmt.Errorf("route_rule.ip_cidr %q: %w", cidr, err)
		}
	}
	for _, cidr := range r.SourceIPCIDR {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			return fmt.Errorf("route_rule.source_ip_cidr %q: %w", cidr, err)
		}
	}
	// валидация port_range формата "<from>:<to>"
	for _, pr := range r.PortRange {
		if err := validatePortRange(pr); err != nil {
			return fmt.Errorf("route_rule.port_range %q: %w", pr, err)
		}
	}
	// валидация domain_regex
	for _, re := range r.DomainRegex {
		if _, err := regexp.Compile(re); err != nil {
			return fmt.Errorf("route_rule.domain_regex %q: %w", re, err)
		}
	}
	return nil
}

// validatePortRange проверяет формат "<from>:<to>" и диапазон uint16.
func validatePortRange(s string) error {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("ожидается формат \"<from>:<to>\"")
	}
	from, err := strconv.ParseUint(parts[0], 10, 16)
	if err != nil {
		return fmt.Errorf("from=%q: %w", parts[0], err)
	}
	to, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return fmt.Errorf("to=%q: %w", parts[1], err)
	}
	if from == 0 || to == 0 {
		return fmt.Errorf("порт не может быть 0")
	}
	if from > to {
		return fmt.Errorf("from %d > to %d", from, to)
	}
	return nil
}

// RuleSetRef — дескриптор rule_set для секции route.rule_set.
type RuleSetRef struct {
	Tag            string `json:"tag"`
	Type           string `json:"type"`
	Format         string `json:"format,omitempty"`
	URL            string `json:"url,omitempty"`
	Path           string `json:"path,omitempty"`
	DownloadDetour string `json:"download_detour,omitempty"`
	UpdateInterval string `json:"update_interval,omitempty"`
}

// Validate проверяет корректность RuleSetRef.
func (rs RuleSetRef) Validate() error {
	if rs.Tag == "" {
		return fmt.Errorf("rule_set.tag не может быть пустым")
	}
	switch rs.Type {
	case "remote", "local", "inline":
	default:
		return fmt.Errorf("rule_set.type %q: недопустимое значение (допустимо: remote, local, inline)", rs.Type)
	}
	if rs.Type == "remote" && rs.URL == "" {
		return fmt.Errorf("rule_set.url обязателен для type=remote (tag=%q)", rs.Tag)
	}
	if rs.Type == "local" && rs.Path == "" {
		return fmt.Errorf("rule_set.path обязателен для type=local (tag=%q)", rs.Tag)
	}
	return nil
}

// Validate проверяет корректность RoutingConfig.
func (c *RoutingConfig) Validate() error {
	if c.Version <= 0 {
		return fmt.Errorf("routing_config.version должен быть > 0, получено %d", c.Version)
	}

	// валидация inbounds + уникальность тегов
	inboundTags := make(map[string]bool, len(c.Inbounds))
	for i, ib := range c.Inbounds {
		if err := ib.Validate(); err != nil {
			return fmt.Errorf("routing_config.inbounds[%d]: %w", i, err)
		}
		if inboundTags[ib.Tag] {
			return fmt.Errorf("routing_config.inbounds: дублирующийся тег %q", ib.Tag)
		}
		inboundTags[ib.Tag] = true
	}

	// валидация outbounds + уникальность тегов
	outboundTags := make(map[string]bool, len(c.Outbounds))
	for i, ob := range c.Outbounds {
		if err := ob.Validate(); err != nil {
			return fmt.Errorf("routing_config.outbounds[%d]: %w", i, err)
		}
		if outboundTags[ob.Tag] {
			return fmt.Errorf("routing_config.outbounds: дублирующийся тег %q", ob.Tag)
		}
		outboundTags[ob.Tag] = true
	}

	// валидация rules
	for i, rule := range c.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("routing_config.rules[%d]: %w", i, err)
		}
	}

	// валидация rule_sets + уникальность тегов
	ruleSetTags := make(map[string]bool, len(c.RuleSets))
	for i, rs := range c.RuleSets {
		if err := rs.Validate(); err != nil {
			return fmt.Errorf("routing_config.rule_sets[%d]: %w", i, err)
		}
		if ruleSetTags[rs.Tag] {
			return fmt.Errorf("routing_config.rule_sets: дублирующийся тег %q", rs.Tag)
		}
		ruleSetTags[rs.Tag] = true
	}

	// проверка Final
	if c.Final != "" {
		builtinFinal := c.Final == "direct" || c.Final == "block"
		if !builtinFinal && !outboundTags[c.Final] {
			return fmt.Errorf("routing_config.final=%q: нет outbound с таким тегом", c.Final)
		}
	}

	return nil
}

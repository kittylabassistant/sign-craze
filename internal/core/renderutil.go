package core

import "github.com/kittylabassistant/sign-craze/pkg/types"

// EffectiveOutbounds возвращает фактический список outbound'ов для рендера конфига ядра.
// Приоритет: RoutingConfig.Outbounds (если непустой) → rp.Outbounds.
// Используется xray и mihomo адаптерами для единообразного выбора outbound-списка.
func EffectiveOutbounds(rp types.CoreRenderParams) []types.Outbound {
	if rp.RoutingConfig != nil && len(rp.RoutingConfig.Outbounds) > 0 {
		return rp.RoutingConfig.Outbounds
	}
	return rp.Outbounds
}

// EffectiveDefaultTag возвращает финальный тег outbound'а для рендера конфига ядра.
// Приоритет: rp.DefaultOutboundTag → RoutingConfig.Final → первый outbound.
// Используется xray и mihomo адаптерами для единообразного определения тега по умолчанию.
func EffectiveDefaultTag(rp types.CoreRenderParams, outs []types.Outbound) string {
	if rp.DefaultOutboundTag != "" {
		return rp.DefaultOutboundTag
	}
	if rp.RoutingConfig != nil && rp.RoutingConfig.Final != "" {
		return rp.RoutingConfig.Final
	}
	if len(outs) > 0 {
		return outs[0].Tag
	}
	return ""
}

// PrefixWarnings добавляет префикс (например "rules[0]") к каждой warning-строке.
// Используется рендерами правил xray и mihomo для унификации формата предупреждений.
func PrefixWarnings(prefix string, ws []string) []string {
	if len(ws) == 0 {
		return nil
	}
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = prefix + ": " + w
	}
	return out
}

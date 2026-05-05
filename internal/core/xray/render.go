package xray

import (
	"encoding/json"
	"fmt"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// ConfigParams — параметры для генерации xray config.json.
type ConfigParams struct {
	// LogLevel — уровень логирования xray ("warning" по умолчанию).
	LogLevel string
	// TProxyPort — порт TPROXY sign-craze (7895).
	TProxyPort uint16
	// FWMark — fwmark sign-craze (0x53 = 83).
	FWMark uint32
	// Outbounds — список outbound'ов (legacy, используется для tag/server/port).
	Outbounds []types.Outbound
	// Canonicals — canonical-описания, ключ = Outbound.Tag.
	Canonicals map[string]types.Canonical
	// DefaultTag — тег первого proxy outbound для routing rule.
	DefaultTag string
}

// DefaultConfigParams возвращает параметры по умолчанию.
func DefaultConfigParams() ConfigParams {
	return ConfigParams{
		LogLevel:   "warning",
		TProxyPort: 7895,
		FWMark:     0x53, // 83
	}
}

// xrayConfig — корневая структура xray config.json.
type xrayConfig struct {
	Log       map[string]any   `json:"log"`
	DNS       map[string]any   `json:"dns,omitempty"`
	Inbounds  []map[string]any `json:"inbounds"`
	Outbounds []map[string]any `json:"outbounds"`
	Routing   map[string]any   `json:"routing"`
}

// Render генерирует xray config.json как []byte (json.MarshalIndent).
// Возвращает ошибку при unsupported протоколе или отсутствии canonical.
func Render(p ConfigParams) ([]byte, error) {
	logLevel := p.LogLevel
	if logLevel == "" {
		logLevel = "warning"
	}

	// Строим outbound-список
	outbounds := make([]map[string]any, 0, len(p.Outbounds)+2)
	for _, ob := range p.Outbounds {
		c, ok := p.Canonicals[ob.Tag]
		if !ok {
			return nil, fmt.Errorf("xray render: outbound %q: canonical отсутствует, требуется парсер D.1", ob.Tag)
		}
		if err := Validate(c); err != nil {
			return nil, fmt.Errorf("xray render: outbound %q: %w", ob.Tag, err)
		}
		outbounds = append(outbounds, renderOutbound(ob, c))
	}

	// Добавляем служебные direct/block, если их нет в списке
	hasDirect := false
	hasBlock := false
	for _, ob := range p.Outbounds {
		if ob.Tag == "direct" {
			hasDirect = true
		}
		if ob.Tag == "block" {
			hasBlock = true
		}
	}
	if !hasDirect {
		outbounds = append(outbounds, map[string]any{
			"tag":      "direct",
			"protocol": "freedom",
		})
	}
	if !hasBlock {
		outbounds = append(outbounds, map[string]any{
			"tag":      "block",
			"protocol": "blackhole",
		})
	}

	// Строим inbound: tproxy через dokodemo-door
	inbounds := []map[string]any{
		buildTProxyInbound(p.TProxyPort, p.FWMark),
	}

	// Строим routing
	defaultTag := p.DefaultTag
	if defaultTag == "" && len(p.Outbounds) > 0 {
		defaultTag = p.Outbounds[0].Tag
	}

	routing := map[string]any{
		"domainStrategy": "AsIs",
		"rules": []any{
			map[string]any{
				"type":        "field",
				"inboundTag":  []string{"tproxy-in"},
				"outboundTag": defaultTag,
			},
		},
	}

	cfg := xrayConfig{
		Log: map[string]any{
			"loglevel": logLevel,
		},
		Inbounds:  inbounds,
		Outbounds: outbounds,
		Routing:   routing,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("xray render: %w", err)
	}
	return data, nil
}

// buildTProxyInbound строит inbound dokodemo-door с TPROXY и sniffing.
func buildTProxyInbound(port uint16, fwmark uint32) map[string]any {
	return map[string]any{
		"tag":      "tproxy-in",
		"port":     port,
		"protocol": "dokodemo-door",
		"settings": map[string]any{
			"network":        "tcp,udp",
			"followRedirect": true,
		},
		"streamSettings": map[string]any{
			"sockopt": map[string]any{
				"tproxy": "tproxy",
				"mark":   fwmark,
			},
		},
		"sniffing": map[string]any{
			"enabled":      true,
			"destOverride": []string{"http", "tls", "quic"},
		},
	}
}

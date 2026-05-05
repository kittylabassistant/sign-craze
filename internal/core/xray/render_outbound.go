package xray

import (
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// xrayProtocolName маппит canonical Protocol в имя protocol для xray.
// Большинство имён совпадают; исключения — direct (xray: freedom) и
// block (xray: blackhole).
func xrayProtocolName(p types.Protocol) string {
	switch p {
	case types.ProtocolDirect:
		return "freedom"
	case types.ProtocolBlock:
		return "blackhole"
	default:
		return string(p)
	}
}

// renderOutbound строит map[string]any для одного outbound в формате xray config.json.
func renderOutbound(ob types.Outbound, c types.Canonical) map[string]any {
	out := map[string]any{
		"tag":      ob.Tag,
		"protocol": xrayProtocolName(c.Protocol),
	}

	switch c.Protocol {
	case types.ProtocolDirect:
		// freedom не требует settings
	case types.ProtocolBlock:
		out["settings"] = map[string]any{
			"response": map[string]any{"type": "none"},
		}
	case types.ProtocolVLESS:
		out["settings"] = renderVLESSSettings(ob, c)
		out["streamSettings"] = renderStreamSettings(c)
	case types.ProtocolVMess:
		out["settings"] = renderVMessSettings(ob, c)
		out["streamSettings"] = renderStreamSettings(c)
	case types.ProtocolShadowsocks:
		out["settings"] = renderShadowsocksSettings(ob, c)
	case types.ProtocolTrojan:
		out["settings"] = renderTrojanSettings(ob, c)
		out["streamSettings"] = renderStreamSettings(c)
	default:
		// прочие протоколы: базовый вариант с settings из Outbound.Settings
		if ob.Settings != nil {
			out["settings"] = ob.Settings
		}
	}

	return out
}

// renderVLESSSettings строит блок settings для VLESS outbound.
func renderVLESSSettings(ob types.Outbound, c types.Canonical) map[string]any {
	user := map[string]any{
		"id":         "",
		"encryption": "none",
	}
	if c.Proto != nil {
		if c.Proto.UUID != "" {
			user["id"] = c.Proto.UUID
		}
		if c.Proto.Encryption != "" {
			user["encryption"] = c.Proto.Encryption
		}
		if c.Proto.Flow != "" {
			user["flow"] = c.Proto.Flow
		}
	}

	return map[string]any{
		"vnext": []any{
			map[string]any{
				"address": ob.Server,
				"port":    uint16(ob.Port),
				"users":   []any{user},
			},
		},
	}
}

// renderVMessSettings строит блок settings для VMess outbound.
func renderVMessSettings(ob types.Outbound, c types.Canonical) map[string]any {
	user := map[string]any{
		"id":      "",
		"alterId": 0,
		"security": "auto",
	}
	if c.Proto != nil {
		if c.Proto.UUID != "" {
			user["id"] = c.Proto.UUID
		}
		if c.Proto.AlterID != 0 {
			user["alterId"] = c.Proto.AlterID
		}
	}

	return map[string]any{
		"vnext": []any{
			map[string]any{
				"address": ob.Server,
				"port":    uint16(ob.Port),
				"users":   []any{user},
			},
		},
	}
}

// renderShadowsocksSettings строит блок settings для Shadowsocks outbound.
func renderShadowsocksSettings(ob types.Outbound, c types.Canonical) map[string]any {
	srv := map[string]any{
		"address":  ob.Server,
		"port":     uint16(ob.Port),
		"method":   "aes-256-gcm",
		"password": "",
	}
	if c.Proto != nil {
		if c.Proto.Method != "" {
			srv["method"] = c.Proto.Method
		}
		if c.Proto.Password != "" {
			srv["password"] = c.Proto.Password
		}
	}

	return map[string]any{
		"servers": []any{srv},
	}
}

// renderTrojanSettings строит блок settings для Trojan outbound.
func renderTrojanSettings(ob types.Outbound, c types.Canonical) map[string]any {
	srv := map[string]any{
		"address":  ob.Server,
		"port":     uint16(ob.Port),
		"password": "",
	}
	if c.Proto != nil && c.Proto.Password != "" {
		srv["password"] = c.Proto.Password
	}

	return map[string]any{
		"servers": []any{srv},
	}
}

// renderStreamSettings строит блок streamSettings для outbound.
// Включает network, security (none/tls/reality), transportSettings.
func renderStreamSettings(c types.Canonical) map[string]any {
	ss := map[string]any{}

	// Определяем network и добавляем transport-специфичные настройки
	network := "tcp"
	if c.Transport != nil {
		network = renderTransport(c.Transport, ss)
	}
	ss["network"] = network

	// Определяем security и TLS-настройки
	security := "none"
	if c.TLS != nil && c.TLS.Enabled {
		if c.TLS.Reality != nil && c.TLS.Reality.Enabled {
			security = "reality"
			ss["realitySettings"] = renderReality(c.TLS)
		} else {
			security = "tls"
			ss["tlsSettings"] = renderTLS(c.TLS)
		}
	}
	ss["security"] = security

	return ss
}

// renderTransport добавляет transport-специфичные поля в ss и возвращает network-строку.
func renderTransport(t *types.Transport, ss map[string]any) string {
	switch t.Kind {
	case types.TransportWS:
		wsSettings := map[string]any{}
		if t.Path != "" {
			wsSettings["path"] = t.Path
		}
		if t.Host != "" {
			wsSettings["host"] = t.Host
		}
		ss["wsSettings"] = wsSettings
		return "ws"

	case types.TransportGRPC:
		grpcSettings := map[string]any{}
		if t.ServiceName != "" {
			grpcSettings["serviceName"] = t.ServiceName
		}
		ss["grpcSettings"] = grpcSettings
		return "grpc"

	case types.TransportHTTP:
		httpSettings := map[string]any{}
		if t.Path != "" {
			httpSettings["path"] = t.Path
		}
		if t.Host != "" {
			httpSettings["host"] = t.Host
		}
		ss["httpSettings"] = httpSettings
		return "h2"

	case types.TransportXHTTP:
		xhttpSettings := map[string]any{}
		if t.Path != "" {
			xhttpSettings["path"] = t.Path
		}
		if t.Host != "" {
			xhttpSettings["host"] = t.Host
		}
		// Маппинг XHTTPMode → строку режима
		switch t.Mode {
		case types.XHTTPStreamUp:
			xhttpSettings["mode"] = "stream-up"
		case types.XHTTPStreamOne:
			xhttpSettings["mode"] = "stream-one"
		case types.XHTTPPacketUp:
			xhttpSettings["mode"] = "packet-up"
		}
		ss["xhttpSettings"] = xhttpSettings
		return "xhttp"

	case types.TransportTCP:
		if t.HeaderType != "" {
			ss["tcpSettings"] = map[string]any{
				"header": map[string]any{"type": t.HeaderType},
			}
		}
		return "tcp"

	default:
		return "tcp"
	}
}

// renderTLS строит блок tlsSettings для стандартного TLS (не Reality).
func renderTLS(tls *types.TLSConfig) map[string]any {
	tlsSettings := map[string]any{}
	if tls.ServerName != "" {
		tlsSettings["serverName"] = tls.ServerName
	}
	if len(tls.ALPN) > 0 {
		tlsSettings["alpn"] = tls.ALPN
	}
	if tls.Insecure {
		tlsSettings["allowInsecure"] = true
	}
	if tls.UTLS != nil && tls.UTLS.Enabled && tls.UTLS.Fingerprint != "" {
		tlsSettings["fingerprint"] = tls.UTLS.Fingerprint
	}
	return tlsSettings
}

// renderReality строит блок realitySettings для VLESS Reality.
func renderReality(tls *types.TLSConfig) map[string]any {
	r := tls.Reality
	realitySettings := map[string]any{}
	if r.PublicKey != "" {
		realitySettings["publicKey"] = r.PublicKey
	}
	if r.ShortID != "" {
		realitySettings["shortId"] = r.ShortID
	}
	if r.SpiderX != "" {
		realitySettings["spiderX"] = r.SpiderX
	}
	if r.MLDSA65Verify != "" {
		realitySettings["mldsa65Verify"] = r.MLDSA65Verify
	}
	if tls.ServerName != "" {
		realitySettings["serverName"] = tls.ServerName
	}
	if tls.UTLS != nil && tls.UTLS.Enabled && tls.UTLS.Fingerprint != "" {
		realitySettings["fingerprint"] = tls.UTLS.Fingerprint
	}
	return realitySettings
}

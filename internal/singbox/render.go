package singbox

import (
	"fmt"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// renderOutboundJSON строит map[string]any, представляющий outbound-блок
// в формате sing-box config.json.
//
// Два пути:
//   - Canonical path (o.Protocol != ""): структурный рендер из canonical-полей
//     Outbound.Protocol/Transport/TLS/Proto. Используется для всех outbound'ов,
//     добавленных через parserURL (v0.4.0+).
//   - Legacy path (o.Protocol == ""): flatten Outbound.Settings как top-level поля.
//     Используется для старых state.json до v0.4.0, где canonical не заполнен.
func renderOutboundJSON(o types.Outbound) (map[string]any, error) {
	if o.Protocol != "" {
		return renderCanonical(o)
	}
	return renderLegacySettings(o), nil
}

// renderLegacySettings — legacy-путь: base-поля + flatten Settings.
// Воспроизводит поведение удалённого Outbound.MarshalJSON для sing-box.
func renderLegacySettings(o types.Outbound) map[string]any {
	out := map[string]any{
		"tag":  o.Tag,
		"type": o.Type,
	}
	if o.Server != "" {
		out["server"] = o.Server
	}
	if o.Port != 0 {
		out["server_port"] = uint16(o.Port)
	}
	// flatten Settings как top-level поля (sing-box формат)
	for k, v := range o.Settings {
		out[k] = v
	}
	return out
}

// renderCanonical — структурный рендер canonical outbound → sing-box JSON.
// Поддерживаемые протоколы: vless, vmess, shadowsocks, trojan, hysteria2, tuic,
// wireguard, socks5, http, direct, block.
func renderCanonical(o types.Outbound) (map[string]any, error) {
	out := map[string]any{
		"tag":  o.Tag,
		"type": singboxTypeName(o.Protocol),
	}

	switch o.Protocol {
	case types.ProtocolDirect, types.ProtocolBlock:
		// direct/block не требуют дополнительных полей в sing-box

	case types.ProtocolVLESS:
		if err := renderVLESS(out, o); err != nil {
			return nil, err
		}

	case types.ProtocolVMess:
		renderVMess(out, o)

	case types.ProtocolShadowsocks:
		renderShadowsocks(out, o)

	case types.ProtocolTrojan:
		renderTrojan(out, o)

	case types.ProtocolHysteria2:
		renderHysteria2(out, o)

	case types.ProtocolTUIC:
		renderTUIC(out, o)

	case types.ProtocolWireGuard:
		renderWireGuard(out, o)

	case types.ProtocolSocks5:
		out["server"] = o.Server
		out["server_port"] = uint16(o.Port)
		if o.Proto != nil && o.Proto.Password != "" {
			out["password"] = o.Proto.Password
		}

	case types.ProtocolHTTP:
		out["server"] = o.Server
		out["server_port"] = uint16(o.Port)
		if o.Proto != nil && o.Proto.Password != "" {
			out["password"] = o.Proto.Password
		}

	default:
		return nil, fmt.Errorf("singbox render: неподдерживаемый протокол %q", o.Protocol)
	}

	return out, nil
}

// singboxTypeName маппит canonical Protocol в тип outbound для sing-box.
// Большинство имён совпадают; исключения — shadowsocks, socks5, http.
func singboxTypeName(p types.Protocol) string {
	switch p {
	case types.ProtocolShadowsocks:
		return "shadowsocks"
	case types.ProtocolSocks5:
		return "socks"
	case types.ProtocolHTTP:
		return "http"
	default:
		return string(p)
	}
}

// renderVLESS заполняет out для VLESS (включая transport и TLS/Reality).
func renderVLESS(out map[string]any, o types.Outbound) error {
	out["server"] = o.Server
	out["server_port"] = uint16(o.Port)
	if o.Proto != nil {
		out["uuid"] = o.Proto.UUID
		if o.Proto.Flow != "" {
			out["flow"] = o.Proto.Flow
		}
		if o.Proto.PacketEnc != "" {
			out["packet_encoding"] = o.Proto.PacketEnc
		}
	}
	applyTransport(out, o.Transport)
	applyTLS(out, o.TLS)
	return nil
}

// renderVMess заполняет out для VMess.
func renderVMess(out map[string]any, o types.Outbound) {
	out["server"] = o.Server
	out["server_port"] = uint16(o.Port)
	if o.Proto != nil {
		out["uuid"] = o.Proto.UUID
		if o.Proto.AlterID != 0 {
			out["alter_id"] = o.Proto.AlterID
		}
		if o.Proto.Encryption != "" {
			out["security"] = o.Proto.Encryption
		} else {
			out["security"] = "auto"
		}
		if o.Proto.PacketEnc != "" {
			out["packet_encoding"] = o.Proto.PacketEnc
		}
	}
	applyTransport(out, o.Transport)
	applyTLS(out, o.TLS)
}

// renderShadowsocks заполняет out для Shadowsocks / SS2022.
func renderShadowsocks(out map[string]any, o types.Outbound) {
	out["server"] = o.Server
	out["server_port"] = uint16(o.Port)
	if o.Proto != nil {
		if o.Proto.Method != "" {
			out["method"] = o.Proto.Method
		}
		if o.Proto.Password != "" {
			out["password"] = o.Proto.Password
		}
	}
}

// renderTrojan заполняет out для Trojan.
func renderTrojan(out map[string]any, o types.Outbound) {
	out["server"] = o.Server
	out["server_port"] = uint16(o.Port)
	if o.Proto != nil && o.Proto.Password != "" {
		out["password"] = o.Proto.Password
	}
	applyTransport(out, o.Transport)
	applyTLS(out, o.TLS)
}

// renderHysteria2 заполняет out для Hysteria 2.
func renderHysteria2(out map[string]any, o types.Outbound) {
	out["server"] = o.Server
	out["server_port"] = uint16(o.Port)
	if o.Proto != nil {
		if o.Proto.Password != "" {
			out["password"] = o.Proto.Password
		}
		if o.Proto.Hysteria2Obfs != "" && o.Proto.Hysteria2Obfs != "none" {
			out["obfs"] = map[string]any{
				"type":     o.Proto.Hysteria2Obfs,
				"password": o.Proto.Hysteria2ObfsPassword,
			}
		}
		if o.Proto.Hysteria2UpMbps > 0 {
			out["up_mbps"] = o.Proto.Hysteria2UpMbps
		}
		if o.Proto.Hysteria2DownMbps > 0 {
			out["down_mbps"] = o.Proto.Hysteria2DownMbps
		}
	}
	applyTLS(out, o.TLS)
}

// renderTUIC заполняет out для TUIC v5.
func renderTUIC(out map[string]any, o types.Outbound) {
	out["server"] = o.Server
	out["server_port"] = uint16(o.Port)
	if o.Proto != nil {
		out["uuid"] = o.Proto.UUID
		if o.Proto.Password != "" {
			out["password"] = o.Proto.Password
		}
		if o.Proto.TUICCongestion != "" {
			out["congestion_control"] = o.Proto.TUICCongestion
		}
		if o.Proto.TUICUDPRelay != "" {
			out["udp_relay_mode"] = o.Proto.TUICUDPRelay
		}
	}
	applyTLS(out, o.TLS)
}

// renderWireGuard заполняет out для WireGuard клиента.
func renderWireGuard(out map[string]any, o types.Outbound) {
	if o.Proto != nil {
		if o.Proto.WGPrivateKey != "" {
			out["private_key"] = o.Proto.WGPrivateKey
		}
		if len(o.Proto.WGAddresses) > 0 {
			out["address"] = o.Proto.WGAddresses
		}
		peers := []map[string]any{}
		peer := map[string]any{}
		if o.Server != "" {
			peer["server"] = o.Server
			peer["server_port"] = uint16(o.Port)
		}
		if o.Proto.WGPublicKey != "" {
			peer["public_key"] = o.Proto.WGPublicKey
		}
		if o.Proto.WGPreshared != "" {
			peer["pre_shared_key"] = o.Proto.WGPreshared
		}
		if len(peer) > 0 {
			peers = append(peers, peer)
			out["peers"] = peers
		}
	}
}

// applyTransport добавляет transport-блок sing-box в out.
// sing-box использует поле "transport" как объект с type + настройками.
func applyTransport(out map[string]any, t *types.Transport) {
	if t == nil || t.Kind == "" || t.Kind == types.TransportTCP {
		return
	}

	transport := map[string]any{}
	switch t.Kind {
	case types.TransportWS:
		transport["type"] = "ws"
		if t.Path != "" {
			transport["path"] = t.Path
		}
		if t.Host != "" {
			transport["headers"] = map[string]any{"Host": t.Host}
		}

	case types.TransportGRPC:
		transport["type"] = "grpc"
		if t.ServiceName != "" {
			transport["service_name"] = t.ServiceName
		}

	case types.TransportHTTP:
		transport["type"] = "http"
		if t.Path != "" {
			transport["path"] = t.Path
		}
		if t.Host != "" {
			transport["host"] = []string{t.Host}
		}

	case types.TransportXHTTP:
		transport["type"] = "xhttp"
		if t.Path != "" {
			transport["path"] = t.Path
		}
		if t.Host != "" {
			transport["host"] = t.Host
		}
		if t.Mode != types.XHTTPNone {
			transport["mode"] = string(t.Mode)
		}

	default:
		return
	}

	out["transport"] = transport
}

// applyTLS добавляет tls-блок sing-box в out.
// Reality конфигурируется через tls.reality подблок.
func applyTLS(out map[string]any, tls *types.TLSConfig) {
	if tls == nil || !tls.Enabled {
		return
	}

	tlsBlock := map[string]any{
		"enabled": true,
	}
	if tls.ServerName != "" {
		tlsBlock["server_name"] = tls.ServerName
	}
	if len(tls.ALPN) > 0 {
		tlsBlock["alpn"] = tls.ALPN
	}
	if tls.Insecure {
		tlsBlock["insecure"] = true
	}
	if tls.UTLS != nil && tls.UTLS.Enabled && tls.UTLS.Fingerprint != "" {
		tlsBlock["utls"] = map[string]any{
			"enabled":     true,
			"fingerprint": tls.UTLS.Fingerprint,
		}
	}
	if tls.Reality != nil && tls.Reality.Enabled {
		reality := map[string]any{
			"enabled": true,
		}
		if tls.Reality.PublicKey != "" {
			reality["public_key"] = tls.Reality.PublicKey
		}
		if tls.Reality.ShortID != "" {
			reality["short_id"] = tls.Reality.ShortID
		}
		// spider_x — xray-only поле; sing-box reality его не поддерживает.
		// Validate() пропускает spx-кейс (xray-only feature не блокирует
		// общее использование URL), здесь — молча игнорируем поле.
		tlsBlock["reality"] = reality
	}

	out["tls"] = tlsBlock
}

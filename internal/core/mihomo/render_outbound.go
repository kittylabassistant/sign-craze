package mihomo

import (
	"fmt"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// renderProxyEntry строит map[string]any — промежуточное представление
// mihomo proxy-entry — из canonical-описания outbound. Результат
// используется в YAML-сериализации через render.go.
//
// Поддерживаемые протоколы: vless, vmess, shadowsocks, trojan,
// hysteria2, tuic, wireguard, socks5, http.
func renderProxyEntry(ob types.Outbound, c types.Canonical) (map[string]any, error) {
	if !c.IsSet() {
		return nil, fmt.Errorf("outbound %q: canonical не задан", ob.Tag)
	}

	entry := map[string]any{
		"name":   ob.Tag,
		"server": ob.Server,
		"port":   uint16(ob.Port),
	}

	switch c.Protocol {
	case types.ProtocolVLESS:
		if err := renderVLESS(entry, ob, c); err != nil {
			return nil, err
		}
	case types.ProtocolVMess:
		renderVMess(entry, ob, c)
	case types.ProtocolShadowsocks:
		renderShadowsocks(entry, ob, c)
	case types.ProtocolTrojan:
		renderTrojan(entry, ob, c)
	case types.ProtocolHysteria2:
		renderHysteria2(entry, ob, c)
	case types.ProtocolTUIC:
		renderTUIC(entry, ob, c)
	case types.ProtocolWireGuard:
		renderWireGuard(entry, ob, c)
	case types.ProtocolSocks5:
		entry["type"] = "socks5"
		applyAuth(entry, c)
	case types.ProtocolHTTP:
		entry["type"] = "http"
		applyAuth(entry, c)
	case types.ProtocolDirect, types.ProtocolBlock:
		// direct/block — не proxy-entry, вызывающий код должен пропускать их
		return nil, fmt.Errorf("outbound %q: протокол %q не является прокси", ob.Tag, c.Protocol)
	default:
		return nil, fmt.Errorf("outbound %q: неизвестный протокол %q", ob.Tag, c.Protocol)
	}

	return entry, nil
}

// renderVLESS заполняет entry для VLESS.
func renderVLESS(entry map[string]any, ob types.Outbound, c types.Canonical) error {
	// Предварительная валидация mihomo-ограничений
	if err := Validate(c); err != nil {
		return fmt.Errorf("outbound %q: %w", ob.Tag, err)
	}

	entry["type"] = "vless"
	if c.Proto != nil {
		entry["uuid"] = c.Proto.UUID
		if c.Proto.Flow != "" {
			entry["flow"] = c.Proto.Flow
		}
	}

	applyTransport(entry, c)
	applyTLS(entry, c)
	return nil
}

// renderVMess заполняет entry для VMess.
func renderVMess(entry map[string]any, _ types.Outbound, c types.Canonical) {
	entry["type"] = "vmess"
	if c.Proto != nil {
		entry["uuid"] = c.Proto.UUID
		entry["alterId"] = c.Proto.AlterID
		if c.Proto.Encryption != "" {
			entry["cipher"] = c.Proto.Encryption
		} else {
			entry["cipher"] = "auto"
		}
	}
	applyTransport(entry, c)
	applyTLS(entry, c)
}

// renderShadowsocks заполняет entry для Shadowsocks / SS2022.
func renderShadowsocks(entry map[string]any, _ types.Outbound, c types.Canonical) {
	entry["type"] = "ss"
	if c.Proto != nil {
		entry["cipher"] = c.Proto.Method
		entry["password"] = c.Proto.Password
	}
}

// renderTrojan заполняет entry для Trojan.
func renderTrojan(entry map[string]any, _ types.Outbound, c types.Canonical) {
	entry["type"] = "trojan"
	if c.Proto != nil {
		entry["password"] = c.Proto.Password
	}
	applyTransport(entry, c)
	applyTLS(entry, c)
}

// renderHysteria2 заполняет entry для Hysteria 2.
func renderHysteria2(entry map[string]any, _ types.Outbound, c types.Canonical) {
	entry["type"] = "hysteria2"
	if c.Proto != nil {
		entry["password"] = c.Proto.Password
		if c.Proto.Hysteria2Obfs != "" && c.Proto.Hysteria2Obfs != "none" {
			entry["obfs"] = c.Proto.Hysteria2Obfs
			entry["obfs-password"] = c.Proto.Hysteria2ObfsPassword
		}
		if c.Proto.Hysteria2UpMbps > 0 {
			entry["up"] = c.Proto.Hysteria2UpMbps
		}
		if c.Proto.Hysteria2DownMbps > 0 {
			entry["down"] = c.Proto.Hysteria2DownMbps
		}
	}
	// TLS SNI для hysteria2
	if c.TLS != nil {
		if c.TLS.ServerName != "" {
			entry["sni"] = c.TLS.ServerName
		}
		entry["skip-cert-verify"] = c.TLS.Insecure
	}
}

// renderTUIC заполняет entry для TUIC v5.
func renderTUIC(entry map[string]any, _ types.Outbound, c types.Canonical) {
	entry["type"] = "tuic"
	if c.Proto != nil {
		entry["uuid"] = c.Proto.UUID
		entry["password"] = c.Proto.Password
		if c.Proto.TUICCongestion != "" {
			entry["congestion-controller"] = c.Proto.TUICCongestion
		} else {
			entry["congestion-controller"] = "bbr"
		}
		if c.Proto.TUICUDPRelay != "" {
			entry["udp-relay-mode"] = c.Proto.TUICUDPRelay
		} else {
			entry["udp-relay-mode"] = "native"
		}
	}
	applyTLS(entry, c)
}

// renderWireGuard заполняет entry для WireGuard.
func renderWireGuard(entry map[string]any, _ types.Outbound, c types.Canonical) {
	entry["type"] = "wireguard"
	if c.Proto != nil {
		entry["private-key"] = c.Proto.WGPrivateKey
		entry["public-key"] = c.Proto.WGPublicKey
		if c.Proto.WGPreshared != "" {
			entry["preshared-key"] = c.Proto.WGPreshared
		}
		if len(c.Proto.WGAddresses) > 0 {
			entry["ip"] = c.Proto.WGAddresses[0]
		}
	}
}

// applyAuth добавляет username/password для socks5/http если заданы.
func applyAuth(entry map[string]any, c types.Canonical) {
	if c.Proto != nil && c.Proto.Password != "" {
		entry["password"] = c.Proto.Password
	}
}

// applyTransport добавляет network + transport-opts в зависимости от вида транспорта.
func applyTransport(entry map[string]any, c types.Canonical) {
	if c.Transport == nil || c.Transport.Kind == "" || c.Transport.Kind == types.TransportTCP {
		return
	}
	switch c.Transport.Kind {
	case types.TransportWS:
		entry["network"] = "ws"
		wsOpts := map[string]any{}
		if c.Transport.Path != "" {
			wsOpts["path"] = c.Transport.Path
		}
		if c.Transport.Host != "" {
			wsOpts["headers"] = map[string]any{"Host": c.Transport.Host}
		}
		if len(wsOpts) > 0 {
			entry["ws-opts"] = wsOpts
		}
	case types.TransportGRPC:
		entry["network"] = "grpc"
		if c.Transport.ServiceName != "" {
			entry["grpc-opts"] = map[string]any{
				"grpc-service-name": c.Transport.ServiceName,
			}
		}
	case types.TransportHTTP:
		// HTTP/2 в mihomo — network: h2
		entry["network"] = "h2"
		h2Opts := map[string]any{}
		if c.Transport.Path != "" {
			h2Opts["path"] = c.Transport.Path
		}
		if c.Transport.Host != "" {
			h2Opts["host"] = []string{c.Transport.Host}
		}
		if len(h2Opts) > 0 {
			entry["h2-opts"] = h2Opts
		}
	case types.TransportXHTTP:
		entry["network"] = "xhttp"
		xhttpOpts := map[string]any{}
		if c.Transport.Path != "" {
			xhttpOpts["path"] = c.Transport.Path
		}
		if c.Transport.Host != "" {
			xhttpOpts["host"] = c.Transport.Host
		}
		if c.Transport.Mode != types.XHTTPNone {
			xhttpOpts["mode"] = string(c.Transport.Mode)
		}
		if len(xhttpOpts) > 0 {
			entry["xhttp-opts"] = xhttpOpts
		}
	}
}

// applyTLS добавляет tls/servername/skip-cert-verify/reality-opts/client-fingerprint.
func applyTLS(entry map[string]any, c types.Canonical) {
	if c.TLS == nil || !c.TLS.Enabled {
		return
	}
	entry["tls"] = true
	if c.TLS.ServerName != "" {
		entry["servername"] = c.TLS.ServerName
	}
	if c.TLS.Insecure {
		entry["skip-cert-verify"] = true
	}

	// Reality
	if c.TLS.Reality != nil && c.TLS.Reality.Enabled {
		realityOpts := map[string]any{
			"public-key": c.TLS.Reality.PublicKey,
			"short-id":   c.TLS.Reality.ShortID,
		}
		entry["reality-opts"] = realityOpts
	}

	// uTLS fingerprint — mihomo использует client-fingerprint как top-level
	if c.TLS.UTLS != nil && c.TLS.UTLS.Enabled && c.TLS.UTLS.Fingerprint != "" {
		entry["client-fingerprint"] = c.TLS.UTLS.Fingerprint
	}
}

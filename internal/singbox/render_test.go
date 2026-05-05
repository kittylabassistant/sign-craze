package singbox

import (
	"encoding/json"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestRenderOutboundJSON_LegacySettingsFlatten — legacy-путь (Protocol == ""):
// Settings flatten-ятся как top-level поля (sing-box формат).
func TestRenderOutboundJSON_LegacySettingsFlatten(t *testing.T) {
	o := types.Outbound{
		Tag:    "vless-legacy",
		Type:   "vless",
		Server: "srv",
		Port:   443,
		Settings: map[string]any{
			"uuid": "legacy-uuid",
			"flow": "xtls-rprx-vision",
			"tls": map[string]any{
				"enabled": true,
				"reality": map[string]any{
					"enabled":    true,
					"public_key": "PK-legacy",
				},
			},
		},
	}
	m, err := renderOutboundJSON(o)
	if err != nil {
		t.Fatalf("renderOutboundJSON: %v", err)
	}

	if m["tag"] != "vless-legacy" || m["type"] != "vless" || m["server"] != "srv" {
		t.Errorf("базовые поля: %+v", m)
	}
	// Settings flatten-ятся
	if _, has := m["settings"]; has {
		t.Errorf("'settings' wrapper не должен присутствовать: %+v", m)
	}
	if m["uuid"] != "legacy-uuid" {
		t.Errorf("uuid не на top-level: %+v", m)
	}
	if m["flow"] != "xtls-rprx-vision" {
		t.Errorf("flow не на top-level: %+v", m)
	}
	tls, ok := m["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls не объект: %+v", m)
	}
	reality, _ := tls["reality"].(map[string]any)
	if reality["public_key"] != "PK-legacy" {
		t.Errorf("tls.reality.public_key не сохранён: %+v", reality)
	}
}

// TestRenderOutboundJSON_CanonicalVLESS_Reality — canonical VLESS+Reality.
func TestRenderOutboundJSON_CanonicalVLESS_Reality(t *testing.T) {
	o := types.Outbound{
		Tag:      "vless-reality",
		Type:     "vless",
		Server:   "example.com",
		Port:     443,
		Protocol: types.ProtocolVLESS,
		TLS: &types.TLSConfig{
			Enabled:    true,
			ServerName: "example.com",
			UTLS:       &types.UTLSConfig{Enabled: true, Fingerprint: "chrome"},
			Reality: &types.RealityConfig{
				Enabled:   true,
				PublicKey: "pbk-test",
				ShortID:   "deadbeef00",
			},
		},
		Proto: &types.ProtoOpts{
			UUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			Flow: "xtls-rprx-vision",
		},
	}
	m, err := renderOutboundJSON(o)
	if err != nil {
		t.Fatalf("renderOutboundJSON: %v", err)
	}

	if m["type"] != "vless" {
		t.Errorf("type = %v, ожидалось vless", m["type"])
	}
	if m["server"] != "example.com" || m["server_port"] == nil {
		t.Errorf("server/server_port: %+v", m)
	}
	if m["uuid"] != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("uuid: %+v", m)
	}
	if m["flow"] != "xtls-rprx-vision" {
		t.Errorf("flow: %+v", m)
	}
	tls, ok := m["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls не объект: %+v", m)
	}
	if tls["enabled"] != true {
		t.Errorf("tls.enabled: %+v", tls)
	}
	reality, ok := tls["reality"].(map[string]any)
	if !ok {
		t.Fatalf("tls.reality не объект: %+v", tls)
	}
	if reality["public_key"] != "pbk-test" {
		t.Errorf("reality.public_key: %+v", reality)
	}
	if reality["short_id"] != "deadbeef00" {
		t.Errorf("reality.short_id: %+v", reality)
	}
	utls, ok := tls["utls"].(map[string]any)
	if !ok {
		t.Fatalf("tls.utls не объект: %+v", tls)
	}
	if utls["fingerprint"] != "chrome" {
		t.Errorf("utls.fingerprint: %+v", utls)
	}
	// transport не должен присутствовать (raw TCP)
	if _, has := m["transport"]; has {
		t.Errorf("transport не должен присутствовать при TCP: %+v", m)
	}
}

// TestRenderOutboundJSON_CanonicalVMess_WS_TLS — canonical VMess+WebSocket+TLS.
func TestRenderOutboundJSON_CanonicalVMess_WS_TLS(t *testing.T) {
	o := types.Outbound{
		Tag:      "vmess-ws",
		Type:     "vmess",
		Server:   "ws.example.com",
		Port:     443,
		Protocol: types.ProtocolVMess,
		Transport: &types.Transport{
			Kind: types.TransportWS,
			Path: "/wsapi",
			Host: "ws.example.com",
		},
		TLS: &types.TLSConfig{
			Enabled:    true,
			ServerName: "ws.example.com",
		},
		Proto: &types.ProtoOpts{
			UUID:    "vmess-uuid-0000",
			AlterID: 0,
		},
	}
	m, err := renderOutboundJSON(o)
	if err != nil {
		t.Fatalf("renderOutboundJSON: %v", err)
	}

	if m["type"] != "vmess" {
		t.Errorf("type = %v, ожидалось vmess", m["type"])
	}
	if m["uuid"] != "vmess-uuid-0000" {
		t.Errorf("uuid: %+v", m)
	}
	if m["security"] != "auto" {
		t.Errorf("security: %+v", m)
	}

	transport, ok := m["transport"].(map[string]any)
	if !ok {
		t.Fatalf("transport не объект: %+v", m)
	}
	if transport["type"] != "ws" {
		t.Errorf("transport.type = %v, ожидалось ws", transport["type"])
	}
	if transport["path"] != "/wsapi" {
		t.Errorf("transport.path: %+v", transport)
	}
	headers, ok := transport["headers"].(map[string]any)
	if !ok {
		t.Fatalf("transport.headers не объект: %+v", transport)
	}
	if headers["Host"] != "ws.example.com" {
		t.Errorf("transport.headers.Host: %+v", headers)
	}

	tls, ok := m["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls не объект: %+v", m)
	}
	if tls["server_name"] != "ws.example.com" {
		t.Errorf("tls.server_name: %+v", tls)
	}
}

// TestRenderOutboundJSON_CanonicalShadowsocks — canonical Shadowsocks.
func TestRenderOutboundJSON_CanonicalShadowsocks(t *testing.T) {
	o := types.Outbound{
		Tag:      "ss-out",
		Type:     "shadowsocks",
		Server:   "ss.example.com",
		Port:     8080,
		Protocol: types.ProtocolShadowsocks,
		Proto: &types.ProtoOpts{
			Method:   "aes-256-gcm",
			Password: "s3cr3t-password",
		},
	}
	m, err := renderOutboundJSON(o)
	if err != nil {
		t.Fatalf("renderOutboundJSON: %v", err)
	}

	if m["type"] != "shadowsocks" {
		t.Errorf("type = %v, ожидалось shadowsocks", m["type"])
	}
	if m["server"] != "ss.example.com" {
		t.Errorf("server: %+v", m)
	}
	if m["method"] != "aes-256-gcm" {
		t.Errorf("method: %+v", m)
	}
	if m["password"] != "s3cr3t-password" {
		t.Errorf("password: %+v", m)
	}
	// tls и transport не должны присутствовать
	if _, has := m["tls"]; has {
		t.Errorf("tls не должен присутствовать для SS: %+v", m)
	}
}

// TestRenderOutboundJSON_CanonicalTrojan_GRPC — canonical Trojan+gRPC+TLS.
func TestRenderOutboundJSON_CanonicalTrojan_GRPC(t *testing.T) {
	o := types.Outbound{
		Tag:      "trojan-grpc",
		Type:     "trojan",
		Server:   "trojan.example.com",
		Port:     443,
		Protocol: types.ProtocolTrojan,
		Transport: &types.Transport{
			Kind:        types.TransportGRPC,
			ServiceName: "grpc.service.name",
		},
		TLS: &types.TLSConfig{
			Enabled:    true,
			ServerName: "trojan.example.com",
		},
		Proto: &types.ProtoOpts{
			Password: "trojan-password",
		},
	}
	m, err := renderOutboundJSON(o)
	if err != nil {
		t.Fatalf("renderOutboundJSON: %v", err)
	}

	if m["type"] != "trojan" {
		t.Errorf("type = %v, ожидалось trojan", m["type"])
	}
	if m["password"] != "trojan-password" {
		t.Errorf("password: %+v", m)
	}

	transport, ok := m["transport"].(map[string]any)
	if !ok {
		t.Fatalf("transport не объект: %+v", m)
	}
	if transport["type"] != "grpc" {
		t.Errorf("transport.type = %v, ожидалось grpc", transport["type"])
	}
	if transport["service_name"] != "grpc.service.name" {
		t.Errorf("transport.service_name: %+v", transport)
	}

	tls, ok := m["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls не объект: %+v", m)
	}
	if tls["enabled"] != true {
		t.Errorf("tls.enabled: %+v", tls)
	}
}

// TestRenderOutboundJSON_ValidJSON — renderOutboundJSON всегда даёт валидный JSON
// для всех canonical-путей.
func TestRenderOutboundJSON_ValidJSON(t *testing.T) {
	cases := []types.Outbound{
		{Tag: "direct", Type: "direct", Protocol: types.ProtocolDirect},
		{Tag: "block", Type: "block", Protocol: types.ProtocolBlock},
		{
			Tag:      "ss",
			Type:     "shadowsocks",
			Server:   "1.2.3.4",
			Port:     8388,
			Protocol: types.ProtocolShadowsocks,
			Proto:    &types.ProtoOpts{Method: "chacha20-poly1305", Password: "pass"},
		},
		{
			Tag:      "vmess",
			Type:     "vmess",
			Server:   "1.2.3.4",
			Port:     443,
			Protocol: types.ProtocolVMess,
			Proto:    &types.ProtoOpts{UUID: "test-uuid"},
		},
		{
			Tag:      "vless",
			Type:     "vless",
			Server:   "1.2.3.4",
			Port:     443,
			Protocol: types.ProtocolVLESS,
			Proto:    &types.ProtoOpts{UUID: "test-uuid"},
			TLS:      &types.TLSConfig{Enabled: true},
		},
	}
	for _, o := range cases {
		t.Run(string(o.Protocol), func(t *testing.T) {
			m, err := renderOutboundJSON(o)
			if err != nil {
				t.Fatalf("renderOutboundJSON(%q): %v", o.Protocol, err)
			}
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			if !json.Valid(b) {
				t.Errorf("невалидный JSON для %q: %s", o.Protocol, b)
			}
		})
	}
}

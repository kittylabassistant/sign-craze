package proxyparse

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestParseCanonical_VLESS_Reality_PQ — PQ-VLESS с mldsa65Verify + encryption=mlkem768x25519plus.
func TestParseCanonical_VLESS_Reality_PQ(t *testing.T) {
	rawURL := "vless://pq-uuid@pq.example.com:443" +
		"?security=reality&encryption=mlkem768x25519plus" +
		"&pbk=PUBLIC_KEY_PQ&sid=ab12cd&mldsa65Verify=VERIFY_KEY&mldsa65Seed=SEED_VAL" +
		"&fp=chrome&sni=target.example.com" +
		"#pq-tag"

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	// Outbound
	if ob.Tag != "pq-tag" {
		t.Errorf("Tag = %q, ожидалось pq-tag", ob.Tag)
	}
	if ob.Server != "pq.example.com" || ob.Port != 443 {
		t.Errorf("Server/Port = %q:%d", ob.Server, ob.Port)
	}

	// Protocol
	if canon.Protocol != types.ProtocolVLESS {
		t.Errorf("Protocol = %q", canon.Protocol)
	}

	// Proto.Encryption
	if canon.Proto == nil {
		t.Fatal("Proto = nil")
	}
	if canon.Proto.Encryption != "mlkem768x25519plus" {
		t.Errorf("Proto.Encryption = %q, ожидалось mlkem768x25519plus", canon.Proto.Encryption)
	}

	// TLS.Reality.MLDSA65Verify
	if canon.TLS == nil || canon.TLS.Reality == nil {
		t.Fatal("TLS или TLS.Reality = nil")
	}
	if !canon.TLS.Reality.Enabled {
		t.Error("TLS.Reality.Enabled должно быть true")
	}
	if canon.TLS.Reality.MLDSA65Verify != "VERIFY_KEY" {
		t.Errorf("MLDSA65Verify = %q", canon.TLS.Reality.MLDSA65Verify)
	}
	if canon.TLS.Reality.MLDSA65Seed != "SEED_VAL" {
		t.Errorf("MLDSA65Seed = %q", canon.TLS.Reality.MLDSA65Seed)
	}
	if canon.TLS.Reality.PublicKey != "PUBLIC_KEY_PQ" {
		t.Errorf("PublicKey = %q", canon.TLS.Reality.PublicKey)
	}
	if canon.TLS.Reality.ShortID != "ab12cd" {
		t.Errorf("ShortID = %q", canon.TLS.Reality.ShortID)
	}
}

// TestParseCanonical_VLESS_XHTTP_PacketUp — XHTTP с mode=packet-up и path.
func TestParseCanonical_VLESS_XHTTP_PacketUp(t *testing.T) {
	rawURL := "vless://xhttp-uuid@xhttp.example.com:443" +
		"?type=xhttp&mode=packet-up&path=%2Fv2&security=none"

	_, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if canon.Transport == nil {
		t.Fatal("Transport = nil")
	}
	if canon.Transport.Kind != types.TransportXHTTP {
		t.Errorf("Transport.Kind = %q, ожидалось xhttp", canon.Transport.Kind)
	}
	if canon.Transport.Mode != types.XHTTPPacketUp {
		t.Errorf("Transport.Mode = %q, ожидалось packet-up", canon.Transport.Mode)
	}
	if canon.Transport.Path != "/v2" {
		t.Errorf("Transport.Path = %q, ожидалось /v2", canon.Transport.Path)
	}
	// security=none → TLS = nil
	if canon.TLS != nil {
		t.Errorf("TLS должен быть nil при security=none, получено %+v", canon.TLS)
	}
}

// TestParseCanonical_VLESS_Vision_UDP443 — flow=xtls-rprx-vision-udp443.
func TestParseCanonical_VLESS_Vision_UDP443(t *testing.T) {
	rawURL := "vless://vision-uuid@vision.example.com:443" +
		"?flow=xtls-rprx-vision-udp443&security=tls&sni=vision.example.com"

	_, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if canon.Proto == nil {
		t.Fatal("Proto = nil")
	}
	if canon.Proto.Flow != "xtls-rprx-vision-udp443" {
		t.Errorf("Proto.Flow = %q, ожидалось xtls-rprx-vision-udp443", canon.Proto.Flow)
	}
	if canon.Proto.UUID != "vision-uuid" {
		t.Errorf("Proto.UUID = %q", canon.Proto.UUID)
	}
}

// TestParseCanonical_VMess_Standard — стандартный VMess base64 JSON.
func TestParseCanonical_VMess_Standard(t *testing.T) {
	raw := map[string]any{
		"v":    "2",
		"ps":   "my-vmess",
		"add":  "vmess.example.com",
		"port": "443",
		"id":   "vmess-uuid-here",
		"aid":  "0",
		"net":  "ws",
		"path": "/ws-path",
		"host": "cdn.example.com",
		"tls":  "tls",
		"sni":  "vmess.example.com",
	}
	js, _ := json.Marshal(raw)
	rawURL := "vmess://" + base64.StdEncoding.EncodeToString(js)

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if ob.Tag != "my-vmess" {
		t.Errorf("Tag = %q, ожидалось my-vmess", ob.Tag)
	}
	if ob.Server != "vmess.example.com" || ob.Port != 443 {
		t.Errorf("Server/Port = %q:%d", ob.Server, ob.Port)
	}
	if canon.Protocol != types.ProtocolVMess {
		t.Errorf("Protocol = %q", canon.Protocol)
	}
	if canon.Proto == nil || canon.Proto.UUID != "vmess-uuid-here" {
		t.Errorf("Proto.UUID = %q", canon.Proto.UUID)
	}
	if canon.Transport == nil || canon.Transport.Kind != types.TransportWS {
		t.Errorf("Transport.Kind ожидалось ws")
	}
	if canon.Transport.Path != "/ws-path" {
		t.Errorf("Transport.Path = %q", canon.Transport.Path)
	}
	if canon.Transport.Host != "cdn.example.com" {
		t.Errorf("Transport.Host = %q", canon.Transport.Host)
	}
	if canon.TLS == nil || !canon.TLS.Enabled {
		t.Error("TLS.Enabled должно быть true")
	}
	if canon.TLS.ServerName != "vmess.example.com" {
		t.Errorf("TLS.ServerName = %q", canon.TLS.ServerName)
	}
}

// TestParseCanonical_Shadowsocks_2022 — SS2022 метод 2022-blake3-aes-256-gcm.
func TestParseCanonical_Shadowsocks_2022(t *testing.T) {
	rawURL := "ss://2022-blake3-aes-256-gcm:secret-password@ss.example.com:8388#ss2022-tag"

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if ob.Tag != "ss2022-tag" {
		t.Errorf("Tag = %q, ожидалось ss2022-tag", ob.Tag)
	}
	if ob.Server != "ss.example.com" || ob.Port != 8388 {
		t.Errorf("Server/Port = %q:%d", ob.Server, ob.Port)
	}
	if canon.Protocol != types.ProtocolShadowsocks {
		t.Errorf("Protocol = %q", canon.Protocol)
	}
	if canon.Proto == nil {
		t.Fatal("Proto = nil")
	}
	if canon.Proto.Method != "2022-blake3-aes-256-gcm" {
		t.Errorf("Method = %q", canon.Proto.Method)
	}
	if canon.Proto.Password != "secret-password" {
		t.Errorf("Password = %q", canon.Proto.Password)
	}
}

// TestParseCanonical_Trojan_TLS_AlwaysEnabled — Trojan без security= → TLS.Enabled=true.
func TestParseCanonical_Trojan_TLS_AlwaysEnabled(t *testing.T) {
	rawURL := "trojan://trojan-pass@trojan.example.com:443?sni=trojan.example.com"

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if ob.Type != "trojan" {
		t.Errorf("Type = %q, ожидалось trojan", ob.Type)
	}
	if canon.Protocol != types.ProtocolTrojan {
		t.Errorf("Protocol = %q", canon.Protocol)
	}
	if canon.TLS == nil || !canon.TLS.Enabled {
		t.Error("TLS.Enabled должно быть true для Trojan всегда")
	}
	if canon.TLS.ServerName != "trojan.example.com" {
		t.Errorf("TLS.ServerName = %q", canon.TLS.ServerName)
	}
	if canon.Proto == nil || canon.Proto.Password != "trojan-pass" {
		t.Errorf("Proto.Password = %q", canon.Proto.Password)
	}
}

// TestParseCanonical_Hysteria2_Obfs — obfs=salamander + обфускация.
func TestParseCanonical_Hysteria2_Obfs(t *testing.T) {
	rawURL := "hysteria2://hy2-password@hy2.example.com:443" +
		"?obfs=salamander&obfs-password=obfs-secret&sni=hy2.example.com&insecure=1&up=100&down=300" +
		"#hy2-node"

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if ob.Tag != "hy2-node" {
		t.Errorf("Tag = %q", ob.Tag)
	}
	if canon.Protocol != types.ProtocolHysteria2 {
		t.Errorf("Protocol = %q", canon.Protocol)
	}
	if canon.Proto == nil {
		t.Fatal("Proto = nil")
	}
	if canon.Proto.Hysteria2Obfs != "salamander" {
		t.Errorf("Hysteria2Obfs = %q", canon.Proto.Hysteria2Obfs)
	}
	if canon.Proto.Hysteria2ObfsPassword != "obfs-secret" {
		t.Errorf("Hysteria2ObfsPassword = %q", canon.Proto.Hysteria2ObfsPassword)
	}
	if canon.Proto.Hysteria2UpMbps != 100 {
		t.Errorf("UpMbps = %d", canon.Proto.Hysteria2UpMbps)
	}
	if canon.Proto.Hysteria2DownMbps != 300 {
		t.Errorf("DownMbps = %d", canon.Proto.Hysteria2DownMbps)
	}
	if canon.TLS == nil || !canon.TLS.Enabled {
		t.Error("TLS.Enabled должно быть true")
	}
	if !canon.TLS.Insecure {
		t.Error("TLS.Insecure должно быть true при insecure=1")
	}
}

// TestParseCanonical_TUIC_v5 — TUIC uuid:password@host:port.
func TestParseCanonical_TUIC_v5(t *testing.T) {
	rawURL := "tuic://tuic-uuid:tuic-pass@tuic.example.com:443" +
		"?congestion_control=bbr&udp_relay_mode=native&alpn=h3"

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if ob.Type != "tuic" {
		t.Errorf("Type = %q", ob.Type)
	}
	if canon.Protocol != types.ProtocolTUIC {
		t.Errorf("Protocol = %q", canon.Protocol)
	}
	if canon.Proto == nil {
		t.Fatal("Proto = nil")
	}
	if canon.Proto.UUID != "tuic-uuid" {
		t.Errorf("UUID = %q", canon.Proto.UUID)
	}
	if canon.Proto.Password != "tuic-pass" {
		t.Errorf("Password = %q", canon.Proto.Password)
	}
	if canon.Proto.TUICCongestion != "bbr" {
		t.Errorf("TUICCongestion = %q", canon.Proto.TUICCongestion)
	}
	if canon.Proto.TUICUDPRelay != "native" {
		t.Errorf("TUICUDPRelay = %q", canon.Proto.TUICUDPRelay)
	}
	if canon.TLS == nil || !canon.TLS.Enabled {
		t.Error("TLS.Enabled должно быть true для TUIC")
	}
	if len(canon.TLS.ALPN) != 1 || canon.TLS.ALPN[0] != "h3" {
		t.Errorf("TLS.ALPN = %v, ожидалось [h3]", canon.TLS.ALPN)
	}
}

// TestParseCanonical_Socks5_Auth — userinfo → Proto.UUID + Password.
func TestParseCanonical_Socks5_Auth(t *testing.T) {
	rawURL := "socks5://alice:wonderland@10.0.0.1:1080"

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if ob.Server != "10.0.0.1" || ob.Port != 1080 {
		t.Errorf("Server/Port = %q:%d", ob.Server, ob.Port)
	}
	if canon.Protocol != types.ProtocolSocks5 {
		t.Errorf("Protocol = %q", canon.Protocol)
	}
	if canon.Proto == nil {
		t.Fatal("Proto = nil")
	}
	if canon.Proto.UUID != "alice" {
		t.Errorf("UUID (username) = %q", canon.Proto.UUID)
	}
	if canon.Proto.Password != "wonderland" {
		t.Errorf("Password = %q", canon.Proto.Password)
	}
}

// TestParseCanonical_InvalidURL_Errors — пустые и невалидные URL возвращают ошибки.
func TestParseCanonical_InvalidURL_Errors(t *testing.T) {
	cases := []struct {
		in      string
		wantErr string
	}{
		{"", "пустая строка"},
		{"unknown://x", "неизвестная схема"},
		{"vless://server.com:443", "отсутствует UUID"},
		{"vmess://!!!", "base64"},
		{"socks5://noport", "отсутствует"},
	}

	for _, c := range cases {
		_, _, err := ParseCanonical(c.in)
		if err == nil {
			t.Errorf("ParseCanonical(%q): ожидалась ошибка", c.in)
			continue
		}
		if !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("ParseCanonical(%q): ошибка %q не содержит %q", c.in, err.Error(), c.wantErr)
		}
	}
}

// TestParseCanonical_TagFromFragment — fragment декодируется в Outbound.Tag.
func TestParseCanonical_TagFromFragment(t *testing.T) {
	// Тег с пробелом (URL-encoded как %20)
	rawURL := "vless://tag-uuid@tag.example.com:443?security=none#my%20tag"

	ob, _, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}
	if ob.Tag != "my tag" {
		t.Errorf("Tag = %q, ожидалось 'my tag' (декодирован из %%20)", ob.Tag)
	}
}

// TestParseCanonical_TagFromFragment_Empty — пустой fragment → fallback tag.
func TestParseCanonical_TagFromFragment_Empty(t *testing.T) {
	rawURL := "vless://no-tag-uuid@notag.example.com:443?security=none"

	ob, _, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}
	if ob.Tag != "vless-out" {
		t.Errorf("Tag = %q, ожидалось vless-out", ob.Tag)
	}
}

// TestParseCanonical_RealityWithoutSecurity_Error — pbk без security=reality → ошибка.
func TestParseCanonical_RealityWithoutSecurity_Error(t *testing.T) {
	cases := []string{
		// pbk без security вообще
		"vless://check-uuid@check.example.com:443?pbk=some-key",
		// pbk с security=tls (не reality)
		"vless://check-uuid@check.example.com:443?pbk=some-key&security=tls",
	}

	for _, rawURL := range cases {
		_, _, err := ParseCanonical(rawURL)
		if err == nil {
			t.Errorf("ParseCanonical(%q): ожидалась ошибка для pbk без security=reality", rawURL)
		}
	}
}

// TestParseCanonical_VLESS_XHTTP_NoMode — XHTTP без mode → XHTTPNone (валидно).
func TestParseCanonical_VLESS_XHTTP_NoMode(t *testing.T) {
	rawURL := "vless://xhttp-nomde-uuid@xhttp.example.com:443?type=xhttp&path=/api&security=none"

	_, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if canon.Transport == nil {
		t.Fatal("Transport = nil")
	}
	if canon.Transport.Kind != types.TransportXHTTP {
		t.Errorf("Kind = %q, ожидалось xhttp", canon.Transport.Kind)
	}
	if canon.Transport.Mode != types.XHTTPNone {
		t.Errorf("Mode = %q, ожидалось пусто (XHTTPNone)", canon.Transport.Mode)
	}
}

// TestParseCanonical_HTTP_TLS — https:// → TLS.Enabled=true + userinfo.
func TestParseCanonical_HTTP_TLS(t *testing.T) {
	rawURL := "https://user:pass@proxy.example.com:8443"

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if ob.Type != "http" {
		t.Errorf("Type = %q", ob.Type)
	}
	if canon.Protocol != types.ProtocolHTTP {
		t.Errorf("Protocol = %q", canon.Protocol)
	}
	if canon.TLS == nil || !canon.TLS.Enabled {
		t.Error("TLS.Enabled должно быть true для https://")
	}
	if canon.Proto == nil || canon.Proto.UUID != "user" {
		t.Errorf("Proto.UUID = %q", canon.Proto.UUID)
	}
	if canon.Proto.Password != "pass" {
		t.Errorf("Proto.Password = %q", canon.Proto.Password)
	}
}

// TestParseCanonical_VLESS_TCP_NoTransport — type=tcp не создаёт Transport.
func TestParseCanonical_VLESS_TCP_NoTransport(t *testing.T) {
	rawURL := "vless://tcp-uuid@tcp.example.com:443?type=tcp&security=tls&sni=tcp.example.com"

	_, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if canon.Transport != nil {
		t.Errorf("Transport должен быть nil для type=tcp, получено %+v", canon.Transport)
	}
}

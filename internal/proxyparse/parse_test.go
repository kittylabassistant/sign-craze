package proxyparse

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestParse_Socks5_Basic(t *testing.T) {
	o, err := Parse("socks5://1.2.3.4:1080")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if o.Type != "socks" || o.Server != "1.2.3.4" || o.Port != 1080 {
		t.Errorf("неверный Outbound: %+v", o)
	}
}

func TestParse_Socks5_WithCreds(t *testing.T) {
	o, err := Parse("socks5://alice:pass@10.0.0.1:1080")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if o.Settings["username"] != "alice" || o.Settings["password"] != "pass" {
		t.Errorf("креды не распарсены: %+v", o.Settings)
	}
}

func TestParse_HTTP(t *testing.T) {
	o, err := Parse("http://proxy.example.com:8080")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if o.Type != "http" || o.Server != "proxy.example.com" || o.Port != 8080 {
		t.Errorf("неверный Outbound: %+v", o)
	}
}

func TestParse_HTTPS_TLSEnabled(t *testing.T) {
	o, err := Parse("https://proxy:443")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if o.Settings == nil {
		t.Fatal("ожидалось Settings.tls")
	}
	tls, ok := o.Settings["tls"].(map[string]any)
	if !ok || tls["enabled"] != true {
		t.Errorf("ожидалось tls.enabled=true, получено %+v", o.Settings)
	}
}

func TestParse_VLESS(t *testing.T) {
	o, err := Parse("vless://abc-uuid@server.com:443?encryption=none&flow=xtls-rprx-vision")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if o.Type != "vless" || o.Server != "server.com" || o.Port != 443 {
		t.Errorf("неверные основные поля: %+v", o)
	}
	if o.Settings["uuid"] != "abc-uuid" || o.Settings["flow"] != "xtls-rprx-vision" {
		t.Errorf("неверные Settings: %+v", o.Settings)
	}
}

// TestParseVLESS_Reality — Reality-параметры (pbk/sid/fp) маппятся в tls.reality + tls.utls.
func TestParseVLESS_Reality(t *testing.T) {
	url := "vless://uuid-1@srv:443?security=reality&encryption=none&flow=xtls-rprx-vision" +
		"&pbk=PUBLIC_KEY&sid=01ab&fp=chrome&sni=example.com&type=tcp"
	o, err := Parse(url)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	tls, ok := o.Settings["tls"].(map[string]any)
	if !ok {
		t.Fatalf("settings.tls не объект: %+v", o.Settings["tls"])
	}
	if tls["enabled"] != true {
		t.Errorf("tls.enabled должно быть true: %+v", tls)
	}
	if tls["server_name"] != "example.com" {
		t.Errorf("tls.server_name = %v, ожидалось example.com", tls["server_name"])
	}
	utls, _ := tls["utls"].(map[string]any)
	if utls["enabled"] != true || utls["fingerprint"] != "chrome" {
		t.Errorf("tls.utls некорректен: %+v", utls)
	}
	reality, _ := tls["reality"].(map[string]any)
	if reality["enabled"] != true || reality["public_key"] != "PUBLIC_KEY" || reality["short_id"] != "01ab" {
		t.Errorf("tls.reality некорректен: %+v", reality)
	}
}

// TestParseVLESS_PostQuantum — mldsa65 поля для PQ-режима reality.
// Поле encryption из URL игнорируется: в sing-box VLESS outbound его нет
// (см. parse.go комментарий + https://sing-box.sagernet.org/configuration/outbound/vless/).
func TestParseVLESS_PostQuantum(t *testing.T) {
	url := "vless://uuid-1@srv:443?security=reality&encryption=mlkem768x25519plus" +
		"&pbk=PK&sid=ab&mldsa65Verify=VERIFY_KEY&mldsa65Seed=SEED_VAL&fp=chrome"
	o, err := Parse(url)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if _, ok := o.Settings["encryption"]; ok {
		t.Errorf("encryption не должен попадать в settings: %+v", o.Settings)
	}

	tls, _ := o.Settings["tls"].(map[string]any)
	reality, _ := tls["reality"].(map[string]any)
	if reality["mldsa65_verify"] != "VERIFY_KEY" {
		t.Errorf("mldsa65_verify не сохранён: %+v", reality)
	}
	if reality["mldsa65_seed"] != "SEED_VAL" {
		t.Errorf("mldsa65_seed не сохранён: %+v", reality)
	}
}

// TestParseVLESS_XHTTP — type=xhttp + path/host → transport.type=xhttp.
func TestParseVLESS_XHTTP(t *testing.T) {
	url := "vless://uuid-1@srv:443?type=xhttp&path=%2Ffoo&host=cdn.example.com&encryption=none"
	o, err := Parse(url)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	transport, ok := o.Settings["transport"].(map[string]any)
	if !ok {
		t.Fatalf("transport отсутствует: %+v", o.Settings)
	}
	if transport["type"] != "xhttp" {
		t.Errorf("transport.type = %v, ожидалось xhttp", transport["type"])
	}
	if transport["path"] != "/foo" {
		t.Errorf("transport.path = %v, ожидалось /foo", transport["path"])
	}
	if transport["host"] != "cdn.example.com" {
		t.Errorf("transport.host = %v", transport["host"])
	}
}

// TestParseVLESS_TCPNoTransport — type=tcp (default) не должен создавать transport.
func TestParseVLESS_TCPNoTransport(t *testing.T) {
	o, err := Parse("vless://uuid-1@srv:443?type=tcp&encryption=none")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, has := o.Settings["transport"]; has {
		t.Errorf("type=tcp не должен создавать transport: %+v", o.Settings)
	}
}

func TestParse_VMess_StandardJSON(t *testing.T) {
	raw := map[string]any{"add": "host.com", "port": "443", "id": "uuid-here", "aid": "0"}
	js, _ := json.Marshal(raw)
	url := "vmess://" + base64.StdEncoding.EncodeToString(js)
	o, err := Parse(url)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if o.Type != "vmess" || o.Server != "host.com" || o.Port != 443 {
		t.Errorf("неверные поля: %+v", o)
	}
	if o.Settings["uuid"] != "uuid-here" {
		t.Errorf("uuid = %v", o.Settings["uuid"])
	}
}

func TestParse_Shadowsocks_Plain(t *testing.T) {
	o, err := Parse("ss://aes-256-gcm:secret@1.1.1.1:8388")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if o.Type != "shadowsocks" || o.Port != 8388 {
		t.Errorf("неверные поля: %+v", o)
	}
	if o.Settings["method"] != "aes-256-gcm" || o.Settings["password"] != "secret" {
		t.Errorf("неверные Settings: %+v", o.Settings)
	}
}

func TestParse_Shadowsocks_Base64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:secret"))
	o, err := Parse("ss://" + encoded + "@2.2.2.2:8388#tag")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if o.Settings["method"] != "aes-256-gcm" || o.Settings["password"] != "secret" {
		t.Errorf("base64 SS не распарсен: %+v", o.Settings)
	}
}

// TestParse_Shadowsocks_RejectsInvalidCipher — неподдерживаемый шифр должен
// отвергаться парсером, а не пробрасываться в config.json для падения sing-box.
func TestParse_Shadowsocks_RejectsInvalidCipher(t *testing.T) {
	for _, bad := range []string{"rot13", "aes-1024-gcm", "rc4"} {
		_, err := Parse("ss://" + bad + ":secret@1.1.1.1:8388")
		if err == nil {
			t.Errorf("ss с шифром %q должен быть отвергнут", bad)
		}
	}
}

// TestParse_Shadowsocks_ValidCiphers — стандартные sing-box шифры пропускаются.
func TestParse_Shadowsocks_ValidCiphers(t *testing.T) {
	good := []string{
		"aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305",
		"2022-blake3-aes-256-gcm",
	}
	for _, g := range good {
		o, err := Parse("ss://" + g + ":pass@1.1.1.1:8388")
		if err != nil {
			t.Errorf("ss с шифром %q должен быть валиден: %v", g, err)
		}
		if o.Settings["method"] != g {
			t.Errorf("method = %v, ожидалось %q", o.Settings["method"], g)
		}
	}
}

func TestParse_Errors(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "пустая"},
		{"unknown://x", "неизвестная схема"},
		{"socks5://nohost", "отсутствует"},
		{"vless://server.com:443", "отсутствует UUID"},
		{"vmess://!!!", "base64"},
	}
	for _, c := range cases {
		_, err := Parse(c.in)
		if err == nil {
			t.Errorf("Parse(%q): ожидалась ошибка", c.in)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("Parse(%q): сообщение %q не содержит %q", c.in, err.Error(), c.want)
		}
	}
}

func TestParseVLESS_SPX_SpiderX(t *testing.T) {
	// spx должен попасть в spider_x, а не short_id
	rawURL := "vless://uuid-x@host:443?security=reality&pbk=PK&spx=%2Fpath&sid=ab&fp=chrome"
	o, err := Parse(rawURL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tls, _ := o.Settings["tls"].(map[string]any)
	reality, _ := tls["reality"].(map[string]any)
	if reality["spider_x"] != "/path" {
		t.Errorf("spider_x = %v, ожидалось /path", reality["spider_x"])
	}
	if reality["short_id"] != "ab" {
		t.Errorf("short_id = %v, ожидалось ab", reality["short_id"])
	}
}

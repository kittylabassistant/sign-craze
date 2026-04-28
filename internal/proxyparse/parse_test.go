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

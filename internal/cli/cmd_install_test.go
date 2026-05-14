package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestWizardURL_VLESS_PopulatesCanonical проверяет, что ParseCanonical применяется
// и canonical-поля (Protocol/TLS/Proto) заполняются в Outbound.
func TestWizardURL_VLESS_PopulatesCanonical(t *testing.T) {
	rawURL := "vless://test-uuid@vless.example.com:443" +
		"?security=reality&pbk=PUBLIC_KEY&sid=ab12&fp=chrome&sni=sni.example.com" +
		"#vless-tag"
	input := rawURL + "\n"

	r := bufio.NewReader(bytes.NewBufferString(input))
	out := &bytes.Buffer{}

	obs, err := wizardURL(r, out)
	if err != nil {
		t.Fatalf("wizardURL: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("ожидался 1 outbound, получено %d", len(obs))
	}
	o := obs[0]

	if o.Protocol != types.ProtocolVLESS {
		t.Errorf("Protocol = %q, ожидалось %q", o.Protocol, types.ProtocolVLESS)
	}
	if o.TLS == nil {
		t.Fatal("TLS = nil, ожидалась Reality TLS")
	}
	if !o.TLS.Enabled {
		t.Error("TLS.Enabled должно быть true")
	}
	if o.TLS.Reality == nil {
		t.Fatal("TLS.Reality = nil")
	}
	if o.TLS.Reality.PublicKey != "PUBLIC_KEY" {
		t.Errorf("TLS.Reality.PublicKey = %q", o.TLS.Reality.PublicKey)
	}
	if o.Proto == nil {
		t.Fatal("Proto = nil")
	}
	if o.Proto.UUID != "test-uuid" {
		t.Errorf("Proto.UUID = %q, ожидалось test-uuid", o.Proto.UUID)
	}
	if o.Tag != "vless-tag" {
		t.Errorf("Tag = %q, ожидалось vless-tag", o.Tag)
	}
	if o.Server != "vless.example.com" {
		t.Errorf("Server = %q", o.Server)
	}
	if o.Port != 443 {
		t.Errorf("Port = %d, ожидалось 443", o.Port)
	}
}

// TestWizardURL_Socks5_LegacyFallback проверяет, что socks5 URL корректно
// обрабатывается (ParseCanonical поддерживает socks5, поэтому canonical-поля заполнены).
// Protocol может быть socks5 — это норма; главное, что Outbound базовые поля заполнены.
func TestWizardURL_Socks5_LegacyFallback(t *testing.T) {
	rawURL := "socks5://alice:pass@10.0.0.1:1080"
	input := rawURL + "\n"

	r := bufio.NewReader(bytes.NewBufferString(input))
	out := &bytes.Buffer{}

	obs, err := wizardURL(r, out)
	if err != nil {
		t.Fatalf("wizardURL socks5: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("ожидался 1 outbound, получено %d", len(obs))
	}
	o := obs[0]

	if o.Type != "socks" {
		t.Errorf("Type = %q, ожидалось socks", o.Type)
	}
	if o.Server != "10.0.0.1" {
		t.Errorf("Server = %q", o.Server)
	}
	if o.Port != 1080 {
		t.Errorf("Port = %d, ожидалось 1080", o.Port)
	}
	// Проверяем вывод: строка "Outbound:" должна присутствовать
	if !strings.Contains(out.String(), "Outbound:") {
		t.Errorf("ожидалась строка Outbound: в выводе, получено: %q", out.String())
	}
	// Protocol заполнен через ParseCanonical (socks5 поддерживается)
	if o.Protocol == "" {
		t.Log("Protocol пустой — socks5 через legacy Parse, это допустимо")
	}
}

// TestDetectCoreFromProxyURL_PQVLESS — PQ-VLESS должен дать recommended=xray.
// Регрессия на случай если кто-то ослабит singbox/mihomo Validate без обновления тестов.
func TestDetectCoreFromProxyURL_PQVLESS(t *testing.T) {
	url := "vless://11111111-1111-1111-1111-111111111111@example.com:443" +
		"?encryption=mlkem768x25519plus&type=tcp&security=reality&pbk=AAA&sid=BB#pq"

	rec, all, ok := detectCoreFromProxyURL(url)
	if !ok {
		t.Fatal("detectCoreFromProxyURL ok=false, ожидалось true для валидного PQ-VLESS")
	}
	if rec != "xray" {
		t.Errorf("recommended=%q, ожидалось xray", rec)
	}
	if len(all) != 1 || all[0] != "xray" {
		t.Errorf("allCompatible=%v, ожидалось [xray]", all)
	}
}

// TestDetectCoreFromProxyURL_PlainVLESS — plain VLESS совместим со всеми, default=sing-box.
func TestDetectCoreFromProxyURL_PlainVLESS(t *testing.T) {
	url := "vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=none&type=tcp#test"

	rec, all, ok := detectCoreFromProxyURL(url)
	if !ok {
		t.Fatal("ok=false, ожидалось true")
	}
	if rec != "sing-box" {
		t.Errorf("recommended=%q, ожидалось sing-box (default при multi-match)", rec)
	}
	if len(all) != 3 {
		t.Errorf("allCompatible=%v, ожидалось 3 ядра", all)
	}
}

// TestDetectCoreFromProxyURL_InvalidURL — невалидный URL даёт ok=false.
func TestDetectCoreFromProxyURL_InvalidURL(t *testing.T) {
	rec, all, ok := detectCoreFromProxyURL("not-a-url")
	if ok {
		t.Errorf("ok=true для невалидного URL, ожидалось false")
	}
	if rec != "" || all != nil {
		t.Errorf("rec=%q all=%v, ожидалось пусто", rec, all)
	}
}

// TestParseProxyURLToOutbound_ReturnsRecommended — расширенная сигнатура
// должна возвращать recommended core вместе с Outbound.
func TestParseProxyURLToOutbound_ReturnsRecommended(t *testing.T) {
	url := "tuic://uuid:pass@example.com:443?congestion_control=bbr&udp_relay_mode=native#tuic"
	o, rec, err := parseProxyURLToOutbound(url)
	if err != nil {
		t.Fatalf("parseProxyURLToOutbound: %v", err)
	}
	if o.Protocol != types.ProtocolTUIC {
		t.Errorf("Protocol=%q, ожидалось %q", o.Protocol, types.ProtocolTUIC)
	}
	// TUIC: совместим sing-box+mihomo, sing-box default → recommended=sing-box.
	if rec != "sing-box" {
		t.Errorf("recommended=%q, ожидалось sing-box (default при наличии)", rec)
	}
}

// TestParseProxyURLToOutbound_NaiveHTTPS — naive+https URL должен дать
// Protocol=ProtocolNaive и NaiveListenPort=18443.
func TestParseProxyURLToOutbound_NaiveHTTPS(t *testing.T) {
	url := "naive+https://alice:secret@proxy.example.com:8443#naive-tag"
	o, _, err := parseProxyURLToOutbound(url)
	if err != nil {
		t.Fatalf("parseProxyURLToOutbound naive+https: %v", err)
	}
	if o.Protocol != types.ProtocolNaive {
		t.Errorf("Protocol=%q, ожидалось %q", o.Protocol, types.ProtocolNaive)
	}
	if o.Proto == nil {
		t.Fatal("Proto = nil, ожидались заполненные NaiveUsername/NaiveListenPort")
	}
	if o.Proto.NaiveListenPort != 18443 {
		t.Errorf("NaiveListenPort=%d, ожидалось 18443", o.Proto.NaiveListenPort)
	}
	if o.Proto.NaiveUsername != "alice" {
		t.Errorf("NaiveUsername=%q, ожидалось alice", o.Proto.NaiveUsername)
	}
	if o.Server != "proxy.example.com" {
		t.Errorf("Server=%q, ожидалось proxy.example.com", o.Server)
	}
	if o.Port != 8443 {
		t.Errorf("Port=%d, ожидалось 8443", o.Port)
	}
}

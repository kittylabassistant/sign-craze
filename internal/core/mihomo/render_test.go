package mihomo

import (
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestRender_VLESS_XHTTP_PacketUp(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{{Tag: "proxy", Type: "vless", Server: "h", Port: 443}}
	p.Canonicals = map[string]types.Canonical{
		"proxy": {
			Protocol:  types.ProtocolVLESS,
			Transport: &types.Transport{Kind: types.TransportXHTTP, Mode: types.XHTTPPacketUp, Path: "/v2"},
			TLS:       &types.TLSConfig{Enabled: true, ServerName: "h"},
			Proto:     &types.ProtoOpts{UUID: "uuid-1"},
		},
	}
	p.DefaultTag = "proxy"
	out, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "network: xhttp") {
		t.Errorf("нет network: xhttp\n%s", s)
	}
	if !strings.Contains(s, "mode: packet-up") {
		t.Errorf("нет mode: packet-up\n%s", s)
	}
	if !strings.Contains(s, "uuid: uuid-1") {
		t.Errorf("нет uuid: uuid-1\n%s", s)
	}
}

func TestRender_Hysteria2_Salamander(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{{Tag: "hy2", Type: "hysteria2", Server: "h", Port: 443}}
	p.Canonicals = map[string]types.Canonical{
		"hy2": {
			Protocol: types.ProtocolHysteria2,
			Proto: &types.ProtoOpts{
				Password:              "pw",
				Hysteria2Obfs:         "salamander",
				Hysteria2ObfsPassword: "ob",
				Hysteria2UpMbps:       100,
				Hysteria2DownMbps:     300,
			},
		},
	}
	p.DefaultTag = "hy2"
	out, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"type: hysteria2", "obfs: salamander", "up: 100", "down: 300"} {
		if !strings.Contains(s, want) {
			t.Errorf("отсутствует %q\n%s", want, s)
		}
	}
}

func TestRender_TUIC_v5(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{{Tag: "tuic", Type: "tuic", Server: "h", Port: 443}}
	p.Canonicals = map[string]types.Canonical{
		"tuic": {
			Protocol: types.ProtocolTUIC,
			Proto: &types.ProtoOpts{
				UUID:           "u",
				Password:       "p",
				TUICCongestion: "bbr",
				TUICUDPRelay:   "native",
			},
		},
	}
	p.DefaultTag = "tuic"
	out, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "type: tuic") {
		t.Errorf("type: tuic missing\n%s", s)
	}
	if !strings.Contains(s, "congestion-controller: bbr") {
		t.Errorf("congestion missing\n%s", s)
	}
}

func TestRender_Shadowsocks_2022(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{{Tag: "ss", Type: "shadowsocks", Server: "h", Port: 443}}
	p.Canonicals = map[string]types.Canonical{
		"ss": {
			Protocol: types.ProtocolShadowsocks,
			Proto:    &types.ProtoOpts{Method: "2022-blake3-aes-256-gcm", Password: "p"},
		},
	}
	p.DefaultTag = "ss"
	out, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "2022-blake3-aes-256-gcm") {
		t.Errorf("cipher missing:\n%s", out)
	}
}

func TestRender_TproxyConfig_HasMark(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{{Tag: "direct", Type: "direct"}}
	p.Canonicals = map[string]types.Canonical{
		"direct": {Protocol: types.ProtocolDirect},
	}
	p.DefaultTag = "direct"
	out, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "tproxy-port: 7895") {
		t.Errorf("tproxy-port missing\n%s", s)
	}
	if !strings.Contains(s, "routing-mark: 83") {
		t.Errorf("routing-mark missing\n%s", s)
	}
}

func TestValidate_Rejects_Vision_UDP443(t *testing.T) {
	err := Validate(types.Canonical{
		Protocol: types.ProtocolVLESS,
		Proto:    &types.ProtoOpts{Flow: "xtls-rprx-vision-udp443"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Vision UDP443") || !strings.Contains(msg, "--core xray") {
		t.Errorf("message не содержит ожидаемое: %v", err)
	}
}

func TestValidate_Rejects_PQ_VLESS(t *testing.T) {
	err := Validate(types.Canonical{
		Protocol: types.ProtocolVLESS,
		Proto:    &types.ProtoOpts{Encryption: "mlkem768x25519plus"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "PQ-VLESS") || !strings.Contains(msg, "--core xray") {
		t.Errorf("message не содержит ожидаемое: %v", err)
	}
}

func TestRender_MissingCanonical_Errors(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{{Tag: "no-canon", Type: "vless", Server: "h", Port: 443}}
	p.Canonicals = map[string]types.Canonical{}
	p.DefaultTag = "no-canon"
	_, err := Render(p)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no-canon") {
		t.Errorf("message должно содержать тег: %v", err)
	}
}

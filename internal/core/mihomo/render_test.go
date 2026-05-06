package mihomo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// mihomoGoldenCases — canonical-случаи для TestRender_GoldenWriteback.
// Каждый элемент описывает имя golden-файла (без .yaml) и соответствующий ConfigParams.
// При UPDATE_GOLDEN=1 файлы перегенерируются; без флага — diff против файлов.
var mihomoGoldenCases = []struct {
	name   string
	params ConfigParams
}{
	{
		name: "hysteria2",
		params: func() ConfigParams {
			p := DefaultConfigParams()
			p.Outbounds = []types.Outbound{{Tag: "hy2-out", Type: "hysteria2", Server: "hy2.example.com", Port: 443}}
			p.Canonicals = map[string]types.Canonical{
				"hy2-out": {
					Protocol: types.ProtocolHysteria2,
					Proto: &types.ProtoOpts{
						Password:          "hy2-password-placeholder",
						Hysteria2UpMbps:   100,
						Hysteria2DownMbps: 300,
					},
					TLS: &types.TLSConfig{
						ServerName: "hy2.example.com",
						Insecure:   false,
					},
				},
			}
			p.DefaultTag = "hy2-out"
			return p
		}(),
	},
	{
		name: "tuic_v5",
		params: func() ConfigParams {
			p := DefaultConfigParams()
			p.Outbounds = []types.Outbound{{Tag: "tuic-out", Type: "tuic", Server: "tuic.example.com", Port: 443}}
			p.Canonicals = map[string]types.Canonical{
				"tuic-out": {
					Protocol: types.ProtocolTUIC,
					Proto: &types.ProtoOpts{
						UUID:           "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
						Password:       "tuic-password-placeholder",
						TUICCongestion: "bbr",
						TUICUDPRelay:   "native",
					},
					TLS: &types.TLSConfig{
						Enabled:    true,
						ServerName: "tuic.example.com",
					},
				},
			}
			p.DefaultTag = "tuic-out"
			return p
		}(),
	},
	{
		name: "vless_reality",
		params: func() ConfigParams {
			p := DefaultConfigParams()
			p.Outbounds = []types.Outbound{{Tag: "vless-reality-out", Type: "vless", Server: "example.com", Port: 443}}
			p.Canonicals = map[string]types.Canonical{
				"vless-reality-out": {
					Protocol: types.ProtocolVLESS,
					Proto: &types.ProtoOpts{
						UUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
						Flow: "xtls-rprx-vision",
					},
					TLS: &types.TLSConfig{
						Enabled:    true,
						ServerName: "example.com",
						UTLS:       &types.UTLSConfig{Enabled: true, Fingerprint: "chrome"},
						Reality: &types.RealityConfig{
							Enabled:   true,
							PublicKey: "mwRiBidYJGzBfYLhvYFCGisjZ1sT4kmjqKCyWHPRtXo",
							ShortID:   "deadbeef00",
						},
					},
				},
			}
			p.DefaultTag = "vless-reality-out"
			return p
		}(),
	},
	{
		name: "shadowsocks",
		params: func() ConfigParams {
			p := DefaultConfigParams()
			p.Outbounds = []types.Outbound{{Tag: "ss-out", Type: "shadowsocks", Server: "ss.example.com", Port: 8388}}
			p.Canonicals = map[string]types.Canonical{
				"ss-out": {
					Protocol: types.ProtocolShadowsocks,
					Proto: &types.ProtoOpts{
						Method:   "chacha20-ietf-poly1305",
						Password: "s3cr3t-password-placeholder",
					},
				},
			}
			p.DefaultTag = "ss-out"
			return p
		}(),
	},
}

// TestRender_GoldenWriteback проверяет соответствие вывода Render() golden-файлам
// в testdata/canonical/.
//
// При UPDATE_GOLDEN=1 golden-файлы перегенерируются. Без флага — байтовый diff
// против файла (YAML выводится детерминированно через marshalYAMLInline).
func TestRender_GoldenWriteback(t *testing.T) {
	update := os.Getenv("UPDATE_GOLDEN") == "1"

	for _, tc := range mihomoGoldenCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.params)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			goldenPath := filepath.Join("testdata", "canonical", tc.name+".yaml")

			if update {
				if writeErr := os.WriteFile(goldenPath, got, 0o644); writeErr != nil {
					t.Fatalf("запись golden-файла %s: %v", goldenPath, writeErr)
				}
				t.Logf("обновлён golden: %s", goldenPath)
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden-файл не найден %s; запустите с UPDATE_GOLDEN=1 для генерации: %v", goldenPath, err)
			}

			// Побайтовый diff (YAML детерминирован через marshalYAMLInline + sort).
			if string(got) != string(want) {
				t.Errorf("Render не совпадает с golden %s:\ngot:\n%s\nwant:\n%s", goldenPath, got, want)
			}
		})
	}
}

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

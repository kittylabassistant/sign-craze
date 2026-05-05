package singbox

import (
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ob      types.Outbound
		wantErr string // подстрока в ошибке; "" — ожидаем nil
	}{
		{
			name: "happy path: обычный VLESS Reality без xray-only полей",
			ob: types.Outbound{
				Tag:      "proxy",
				Type:     "vless",
				Protocol: types.ProtocolVLESS,
				Transport: &types.Transport{
					Kind: types.TransportTCP,
				},
				TLS: &types.TLSConfig{
					Enabled:    true,
					ServerName: "example.com",
					Reality: &types.RealityConfig{
						Enabled:   true,
						PublicKey: "abc123",
						ShortID:   "aa",
						// SpiderX пустой — sing-box совместимо
					},
				},
				Proto: &types.ProtoOpts{
					UUID:       "00000000-0000-0000-0000-000000000000",
					Encryption: "none",
				},
			},
			wantErr: "",
		},
		{
			name: "XHTTP packet-up — только xray",
			ob: types.Outbound{
				Tag:      "proxy",
				Type:     "vless",
				Protocol: types.ProtocolVLESS,
				Transport: &types.Transport{
					Kind: types.TransportXHTTP,
					Mode: types.XHTTPPacketUp,
				},
			},
			wantErr: "XHTTP mode",
		},
		{
			name: "XHTTP stream-up — только xray",
			ob: types.Outbound{
				Tag:      "proxy",
				Type:     "vless",
				Protocol: types.ProtocolVLESS,
				Transport: &types.Transport{
					Kind: types.TransportXHTTP,
					Mode: types.XHTTPStreamUp,
				},
			},
			wantErr: "XHTTP mode",
		},
		{
			name: "XHTTP без mode — допустимо для sing-box",
			ob: types.Outbound{
				Tag:      "proxy",
				Type:     "vless",
				Protocol: types.ProtocolVLESS,
				Transport: &types.Transport{
					Kind: types.TransportXHTTP,
					Mode: types.XHTTPNone,
				},
			},
			wantErr: "",
		},
		{
			name: "Reality spider_x — только xray",
			ob: types.Outbound{
				Tag:      "proxy",
				Type:     "vless",
				Protocol: types.ProtocolVLESS,
				TLS: &types.TLSConfig{
					Enabled: true,
					Reality: &types.RealityConfig{
						Enabled: true,
						SpiderX: "/",
					},
				},
			},
			wantErr: "spider_x",
		},
		{
			name: "PQ-VLESS mlkem768x25519plus — только xray",
			ob: types.Outbound{
				Tag:      "proxy",
				Type:     "vless",
				Protocol: types.ProtocolVLESS,
				Proto: &types.ProtoOpts{
					UUID:       "00000000-0000-0000-0000-000000000000",
					Encryption: "mlkem768x25519plus",
				},
			},
			wantErr: "PQ-VLESS",
		},
		{
			name: "PQ-VLESS с суффиксом .native.0rtt.<base64> — только xray",
			ob: types.Outbound{
				Tag:      "proxy",
				Type:     "vless",
				Protocol: types.ProtocolVLESS,
				Proto: &types.ProtoOpts{
					UUID:       "00000000-0000-0000-0000-000000000000",
					Encryption: "mlkem768x25519plus.native.0rtt.AAAAAAAAAAAAAAAA",
				},
			},
			wantErr: "PQ-VLESS",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(tc.ob)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("ожидали nil, получили: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ожидали ошибку содержащую %q, получили nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("ошибка %q не содержит %q", err.Error(), tc.wantErr)
			}
		})
	}
}

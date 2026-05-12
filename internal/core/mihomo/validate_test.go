package mihomo

import (
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestValidate_VLESS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		c       types.Canonical
		wantErr string // подстрока ошибки; "" — nil
	}{
		{
			name:    "nil proto — допустимо",
			c:       types.Canonical{Protocol: types.ProtocolVLESS},
			wantErr: "",
		},
		{
			name: "обычный VLESS без xray-only полей",
			c: types.Canonical{
				Protocol: types.ProtocolVLESS,
				Proto: &types.ProtoOpts{
					UUID:       "00000000-0000-0000-0000-000000000000",
					Encryption: "none",
				},
			},
			wantErr: "",
		},
		{
			name: "Reality spider_x — mihomo молча игнорирует (xray-only поле, валидация проходит)",
			c: types.Canonical{
				Protocol: types.ProtocolVLESS,
				TLS: &types.TLSConfig{
					Enabled: true,
					Reality: &types.RealityConfig{
						Enabled: true,
						SpiderX: "/",
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
			name: "Vision UDP443 — только xray",
			c: types.Canonical{
				Protocol: types.ProtocolVLESS,
				Proto: &types.ProtoOpts{
					Flow: "xtls-rprx-vision-udp443",
				},
			},
			wantErr: "UDP443",
		},
		{
			name: "PQ-VLESS точное совпадение mlkem768x25519plus",
			c: types.Canonical{
				Protocol: types.ProtocolVLESS,
				Proto: &types.ProtoOpts{
					UUID:       "00000000-0000-0000-0000-000000000000",
					Encryption: "mlkem768x25519plus",
				},
			},
			wantErr: "PQ-VLESS",
		},
		{
			name: "PQ-VLESS с суффиксом .native.0rtt.<base64>",
			c: types.Canonical{
				Protocol: types.ProtocolVLESS,
				Proto: &types.ProtoOpts{
					UUID:       "00000000-0000-0000-0000-000000000000",
					Encryption: "mlkem768x25519plus.native.0rtt.AAAAAAAAAAAAAAAA",
				},
			},
			wantErr: "PQ-VLESS",
		},
		{
			name: "другой протокол — Validate возвращает nil",
			c: types.Canonical{
				Protocol: types.ProtocolVMess,
				Proto: &types.ProtoOpts{
					UUID: "00000000-0000-0000-0000-000000000000",
				},
			},
			wantErr: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(tc.c)
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

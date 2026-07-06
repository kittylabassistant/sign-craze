package core

import (
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestRejectSupervisedPeer — mieru/naive отклоняются с текстом, называющим
// вызывающее ядро и протокол; остальные протоколы пропускаются (nil), решение
// по ним остаётся за ядро-специфичной Validate.
func TestRejectSupervisedPeer(t *testing.T) {
	tests := []struct {
		name     string
		coreName string
		protocol types.Protocol
		wantErr  string // "" — ожидается nil
	}{
		{
			name:     "xray отвергает mieru",
			coreName: "xray",
			protocol: types.ProtocolMieru,
			wantErr:  "xray не поддерживает mieru как supervised peer, используйте --core sing-box",
		},
		{
			name:     "xray отвергает naive",
			coreName: "xray",
			protocol: types.ProtocolNaive,
			wantErr:  "xray не поддерживает naive как supervised peer, используйте --core sing-box",
		},
		{
			name:     "mihomo отвергает mieru",
			coreName: "mihomo",
			protocol: types.ProtocolMieru,
			wantErr:  "mihomo не поддерживает mieru как supervised peer, используйте --core sing-box",
		},
		{
			name:     "mihomo отвергает naive",
			coreName: "mihomo",
			protocol: types.ProtocolNaive,
			wantErr:  "mihomo не поддерживает naive как supervised peer, используйте --core sing-box",
		},
		{
			name:     "прочий протокол пропускается (nil)",
			coreName: "xray",
			protocol: types.ProtocolVLESS,
		},
		{
			name:     "пустой Protocol пропускается (nil)",
			coreName: "mihomo",
			protocol: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RejectSupervisedPeer(tt.coreName, types.Canonical{Protocol: tt.protocol})
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ожидали nil, получили: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ожидали ошибку %q, получили nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("ошибка = %q, ожидалось %q", err.Error(), tt.wantErr)
			}
		})
	}
}

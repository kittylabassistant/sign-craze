package xray

import (
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestValidate покрывает ядро-специфичные отказы xray (TUIC/WireGuard/
// Hysteria2), общий для xray/mihomo reject supervised-peer протоколов
// (mieru/naive — core.RejectSupervisedPeer) и совместимые протоколы (nil).
func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		c       types.Canonical
		wantErr string // подстрока ошибки; "" — nil
	}{
		{
			name:    "TUIC не поддерживается",
			c:       types.Canonical{Protocol: types.ProtocolTUIC},
			wantErr: "xray не поддерживает TUIC v5, используйте --core sing-box или --core mihomo",
		},
		{
			name:    "WireGuard не поддерживается",
			c:       types.Canonical{Protocol: types.ProtocolWireGuard},
			wantErr: "xray не поддерживает WireGuard как outbound, используйте --core sing-box",
		},
		{
			name:    "Hysteria2 не поддерживается",
			c:       types.Canonical{Protocol: types.ProtocolHysteria2},
			wantErr: "xray не поддерживает Hysteria2 нативно, используйте --core sing-box или --core mihomo",
		},
		{
			name:    "mieru отклонён как supervised peer",
			c:       types.Canonical{Protocol: types.ProtocolMieru},
			wantErr: "xray не поддерживает mieru как supervised peer, используйте --core sing-box",
		},
		{
			name:    "naive отклонён как supervised peer",
			c:       types.Canonical{Protocol: types.ProtocolNaive},
			wantErr: "xray не поддерживает naive как supervised peer, используйте --core sing-box",
		},
		{
			name: "VLESS совместим",
			c:    types.Canonical{Protocol: types.ProtocolVLESS},
		},
		{
			name: "пустой Canonical совместим",
			c:    types.Canonical{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.c)
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

// TestValidate_SupervisedPeerCheckedFirst — reject supervised-peer протоколов
// должен срабатывать раньше ядро-специфичного switch, даже если бы у
// mieru/naive совпадал c другим полем Canonical (регресс на порядок вызовов
// после вынесения в core.RejectSupervisedPeer).
func TestValidate_SupervisedPeerCheckedFirst(t *testing.T) {
	c := types.Canonical{
		Protocol: types.ProtocolMieru,
		Proto:    &types.ProtoOpts{MieruUsername: "u"},
	}
	err := Validate(c)
	if err == nil || err.Error() != "xray не поддерживает mieru как supervised peer, используйте --core sing-box" {
		t.Errorf("Validate() = %v, ожидался текст supervised-peer отказа", err)
	}
}

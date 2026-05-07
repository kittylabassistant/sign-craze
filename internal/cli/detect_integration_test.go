package cli

import (
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/proxyparse"
)

// TestDetect_RealCores проверяет, что DetectCompatibleCores на реальных
// ядрах (зарегистрированных blank-импортом в cores.go) даёт ожидаемый
// recommend для канонических proxy URL.
//
// Тест-кейсы охватывают матрицу совместимости из validate-функций каждого
// ядра. При изменении Validate в любом ядре должен сразу всплыть здесь.
func TestDetect_RealCores(t *testing.T) {
	// Sanity: все три ядра зарегистрированы.
	names := core.Names()
	if len(names) != 3 {
		t.Fatalf("ожидалось 3 ядра (sing-box, xray, mihomo), получено %d: %v", len(names), names)
	}

	cases := []struct {
		name      string
		url       string
		wantRec   string
		wantInAll []string // хотя бы эти ядра должны быть в allCompatible
		notInAll  []string // эти ядра НЕ должны быть в allCompatible
	}{
		{
			name:      "plain_vless_all_compat",
			url:       "vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=none&type=tcp#test",
			wantRec:   "sing-box",
			wantInAll: []string{"mihomo", "sing-box", "xray"},
		},
		{
			name:      "pq_vless_xray_only",
			url:       "vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=mlkem768x25519plus&type=tcp&security=reality&pbk=AAA&sid=BB#pq",
			wantRec:   "xray",
			wantInAll: []string{"xray"},
			notInAll:  []string{"sing-box", "mihomo"},
		},
		{
			// XHTTP с mode=packet-up: sing-box не поддерживает (validate.go:15),
			// xray поддерживает нативно, mihomo также через xhttp-opts.
			name:      "xhttp_packet_up_no_singbox",
			url:       "vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=none&type=xhttp&mode=packet-up&security=tls&sni=example.com#xh",
			wantRec:   "mihomo", // sing-box отсутствует → первое в alphabetical = mihomo
			wantInAll: []string{"mihomo", "xray"},
			notInAll:  []string{"sing-box"},
		},
		{
			name:      "vision_udp443_singbox_xray",
			url:       "vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=none&flow=xtls-rprx-vision-udp443&type=tcp&security=reality&pbk=AAA&sid=BB#vu",
			wantRec:   "sing-box",
			wantInAll: []string{"sing-box", "xray"},
			notInAll:  []string{"mihomo"},
		},
		{
			name:      "tuic_singbox_mihomo",
			url:       "tuic://uuid:pass@example.com:443?congestion_control=bbr&udp_relay_mode=native#tuic",
			wantRec:   "sing-box",
			wantInAll: []string{"sing-box", "mihomo"},
			notInAll:  []string{"xray"},
		},
		{
			name:      "hysteria2_singbox_mihomo",
			url:       "hy2://pass@example.com:443?obfs=salamander&obfs-password=foo&insecure=1&up=50&down=200#hy",
			wantRec:   "sing-box",
			wantInAll: []string{"sing-box", "mihomo"},
			notInAll:  []string{"xray"},
		},
		{
			name:      "reality_spiderx_xray_only",
			url:       "vless://11111111-1111-1111-1111-111111111111@example.com:443?encryption=none&type=tcp&security=reality&pbk=AAA&sid=BB&spx=%2F#spx",
			wantRec:   "xray",
			wantInAll: []string{"xray"},
			notInAll:  []string{"sing-box", "mihomo"},
		},
		{
			name:      "ss_plain_all_compat",
			url:       "ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ=@example.com:8388#ss",
			wantRec:   "sing-box",
			wantInAll: []string{"mihomo", "sing-box", "xray"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ob, canon, err := proxyparse.ParseCanonical(tc.url)
			if err != nil {
				t.Fatalf("ParseCanonical(%q): %v", tc.url, err)
			}
			// Перенос canonical-полей в Outbound — то же что делает
			// parseProxyURLToOutbound в cmd_install.go.
			ob.Protocol = canon.Protocol
			ob.Transport = canon.Transport
			ob.TLS = canon.TLS
			ob.Proto = canon.Proto

			rec, all := core.RecommendCore(ob)
			if rec != tc.wantRec {
				t.Errorf("recommended=%q, ожидалось %q (all=%v)", rec, tc.wantRec, all)
			}

			allSet := map[string]bool{}
			for _, n := range all {
				allSet[n] = true
			}
			for _, expect := range tc.wantInAll {
				if !allSet[expect] {
					t.Errorf("ожидалось %q в allCompatible, получено %v", expect, all)
				}
			}
			for _, notExpect := range tc.notInAll {
				if allSet[notExpect] {
					t.Errorf("%q не должен быть в allCompatible, получено %v", notExpect, all)
				}
			}
		})
	}
}

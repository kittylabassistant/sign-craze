package peer

import (
	"context"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestMieruPathsFor_Format(t *testing.T) {
	p := MieruPathsFor("prod")
	if p.BinPath != "/opt/sbin/mieru" {
		t.Errorf("BinPath = %q", p.BinPath)
	}
	if p.ConfigPath != "/opt/etc/sign-craze/mieru-prod.conf.json" {
		t.Errorf("ConfigPath = %q", p.ConfigPath)
	}
	if p.PIDFile != "/opt/var/run/sign-craze/mieru-prod.pid" {
		t.Errorf("PIDFile = %q", p.PIDFile)
	}
	if p.LogPath != "/opt/var/log/sign-craze/mieru-prod.log" {
		t.Errorf("LogPath = %q", p.LogPath)
	}
}

func TestHasMieruOutbounds(t *testing.T) {
	cases := []struct {
		name string
		obs  []types.Outbound
		want bool
	}{
		{"empty", nil, false},
		{
			"vless only",
			[]types.Outbound{{Protocol: types.ProtocolVLESS}},
			false,
		},
		{
			"mieru present",
			[]types.Outbound{{Protocol: types.ProtocolVLESS}, {Protocol: types.ProtocolMieru}},
			true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := HasMieruOutbounds(c.obs)
			if got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestCollectMieruStatuses_FiltersNonMieru(t *testing.T) {
	obs := []types.Outbound{
		{Tag: "vless1", Protocol: types.ProtocolVLESS},
		{Tag: "m1", Protocol: types.ProtocolMieru, Proto: &types.ProtoOpts{MieruLocalPort: 41000}},
	}
	got := CollectMieruStatuses(context.Background(), obs)
	if len(got) != 1 {
		t.Fatalf("len = %d, ожидалось 1 (только mieru)", len(got))
	}
	if got[0].Tag != "m1" {
		t.Errorf("Tag = %q", got[0].Tag)
	}
	if got[0].LocalPort != 41000 {
		t.Errorf("LocalPort = %d", got[0].LocalPort)
	}
	// Running=false ожидаемо: PID-файла нет, бинаря mieru тоже нет.
	if got[0].Running {
		t.Errorf("Running = true (ожидалось false для незапущенного peer)")
	}
}

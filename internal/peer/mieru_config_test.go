package peer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func newMieruOutbound() types.Outbound {
	return types.Outbound{
		Tag:      "prod",
		Type:     "socks",
		Server:   "1.2.3.4",
		Port:     8080,
		Protocol: types.ProtocolMieru,
		Proto: &types.ProtoOpts{
			MieruUsername:     "alice",
			MieruPassword:     "hunter2",
			MieruPort:         8080,
			MieruProtocol:     "TCP",
			MieruMultiplexing: "MULTIPLEXING_HIGH",
			MieruProfile:      "prod",
			MieruLocalPort:    41000,
		},
	}
}

func TestBuildMieruClientConfig_HappyPath(t *testing.T) {
	cfg, err := BuildMieruClientConfig(newMieruOutbound())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if cfg.ActiveProfile != "prod" {
		t.Errorf("ActiveProfile = %q", cfg.ActiveProfile)
	}
	if cfg.Socks5Port != 41000 {
		t.Errorf("Socks5Port = %d", cfg.Socks5Port)
	}
	if cfg.RPCPort != 41001 {
		t.Errorf("RPCPort = %d, ожидалось 41001 (Socks5Port+1)", cfg.RPCPort)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("Profiles len = %d", len(cfg.Profiles))
	}
	p := cfg.Profiles[0]
	if p.ProfileName != "prod" {
		t.Errorf("ProfileName = %q", p.ProfileName)
	}
	if p.User.Name != "alice" || p.User.Password != "hunter2" {
		t.Errorf("User = %+v", p.User)
	}
	if len(p.Servers) != 1 {
		t.Fatalf("Servers len = %d", len(p.Servers))
	}
	s := p.Servers[0]
	if s.IPAddress != "1.2.3.4" {
		t.Errorf("IPAddress = %q (ожидалось literal IP)", s.IPAddress)
	}
	if s.DomainName != "" {
		t.Errorf("DomainName = %q (для IP-literal должно быть пусто)", s.DomainName)
	}
	if len(s.PortBindings) != 1 || s.PortBindings[0].Port != 8080 || s.PortBindings[0].Protocol != "TCP" {
		t.Errorf("PortBindings = %+v", s.PortBindings)
	}
	if p.Multiplexing == nil || p.Multiplexing.Level != "MULTIPLEXING_HIGH" {
		t.Errorf("Multiplexing = %+v", p.Multiplexing)
	}
}

func TestBuildMieruClientConfig_DomainName(t *testing.T) {
	ob := newMieruOutbound()
	ob.Server = "vps.example.com"
	cfg, err := BuildMieruClientConfig(ob)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	s := cfg.Profiles[0].Servers[0]
	if s.DomainName != "vps.example.com" {
		t.Errorf("DomainName = %q", s.DomainName)
	}
	if s.IPAddress != "" {
		t.Errorf("IPAddress = %q (для домена должно быть пусто)", s.IPAddress)
	}
}

func TestBuildMieruClientConfig_Errors(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*types.Outbound)
		wantErr string
	}{
		{
			name:    "wrong protocol",
			mutate:  func(o *types.Outbound) { o.Protocol = types.ProtocolVLESS },
			wantErr: "не mieru",
		},
		{
			name:    "no Proto",
			mutate:  func(o *types.Outbound) { o.Proto = nil },
			wantErr: "Proto = nil",
		},
		{
			name:    "empty username",
			mutate:  func(o *types.Outbound) { o.Proto.MieruUsername = "" },
			wantErr: "username/password",
		},
		{
			name:    "empty password",
			mutate:  func(o *types.Outbound) { o.Proto.MieruPassword = "" },
			wantErr: "username/password",
		},
		{
			name:    "no LocalPort",
			mutate:  func(o *types.Outbound) { o.Proto.MieruLocalPort = 0 },
			wantErr: "MieruLocalPort не выделен",
		},
		{
			name:    "no Server",
			mutate:  func(o *types.Outbound) { o.Server = "" },
			wantErr: "пустой Server",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ob := newMieruOutbound()
			c.mutate(&ob)
			_, err := BuildMieruClientConfig(ob)
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v; ожидалось содержит %q", err, c.wantErr)
			}
		})
	}
}

func TestWriteMieruConfig_AtomicAndPerm(t *testing.T) {
	dir := t.TempDir()
	ob := newMieruOutbound()
	path, err := WriteMieruConfig(dir, ob)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := filepath.Join(dir, "mieru-prod.conf.json")
	if path != want {
		t.Errorf("path = %q, ожидалось %q", path, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("perm = %o, ожидалось 0600 (содержит password)", mode)
	}

	// JSON парсится обратно с теми же значениями.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got MieruClientConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Socks5Port != 41000 {
		t.Errorf("прочитано Socks5Port = %d", got.Socks5Port)
	}
	if got.Profiles[0].User.Password != "hunter2" {
		t.Errorf("password lost on roundtrip")
	}
}

func TestMieruConfigPath_Format(t *testing.T) {
	got := MieruConfigPath("/opt/etc/sign-craze", "my-tag")
	want := "/opt/etc/sign-craze/mieru-my-tag.conf.json"
	if got != want {
		t.Errorf("MieruConfigPath = %q, ожидалось %q", got, want)
	}
}

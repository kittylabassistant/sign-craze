package state

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestLoad_FileMissingReturnsDefault(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Mode != types.ModePolicy {
		t.Errorf("Mode = %q, ожидалось policy", s.Mode)
	}
	if s.PolicyName != DefaultPolicyName {
		t.Errorf("PolicyName = %q, ожидалось %q", s.PolicyName, DefaultPolicyName)
	}
	if len(s.Outbounds) != 1 || s.Outbounds[0].Type != "direct" {
		t.Errorf("ожидался stub direct outbound, получено %+v", s.Outbounds)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := &State{
		Mode: types.ModeFull,
		Outbounds: []types.Outbound{
			{Tag: "proxy", Type: "socks", Server: "1.2.3.4", Port: 1080},
		},
		Ports:       []uint16{80, 443},
		Excludes:    []string{"192.168.1.0/24"},
		DPIEnabled:  true,
		DPIStrategy: "preset:default",
		DPITargets:  []string{"discord.com", "youtube.com"},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Mode != want.Mode {
		t.Errorf("Mode = %q, ожидалось %q", got.Mode, want.Mode)
	}
	if len(got.Ports) != 2 || got.Ports[0] != 80 {
		t.Errorf("Ports = %v, ожидалось [80 443]", got.Ports)
	}
	if !got.DPIEnabled {
		t.Error("DPIEnabled должен быть true")
	}
	if len(got.DPITargets) != 2 || got.DPITargets[0] != "discord.com" || got.DPITargets[1] != "youtube.com" {
		t.Errorf("DPITargets = %v, ожидалось [discord.com youtube.com]", got.DPITargets)
	}
}

// TestLoad_DPITargetsEmptyDefault — при отсутствии поля DPITargets в JSON
// Load() должен вернуть пустой slice (не nil), чтобы web API возвращал [].
func TestLoad_DPITargetsEmptyDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	// Save без DPITargets → omitempty опускает поле.
	st := Default()
	if err := Save(path, st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DPITargets == nil {
		t.Error("DPITargets должен быть пустым slice, не nil")
	}
	if len(got.DPITargets) != 0 {
		t.Errorf("DPITargets = %v, ожидался пустой slice", got.DPITargets)
	}
}

func TestSave_Permissions0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Save(path, Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("права %o, ожидалось 0600 (содержит креды outbound)", mode)
	}
}

func TestSave_NilStateReturnsError(t *testing.T) {
	err := Save(filepath.Join(t.TempDir(), "x.json"), nil)
	if err == nil {
		t.Fatal("ожидалась ошибка при nil State")
	}
}

// TestDefault_LocalNetsExcluded проверяет что Default() включает безопасные
// исключения для loopback, link-local, multicast, reserved и RFC1918, а также
// дефолтный admin port 22 — это защищает от SSH-lockout при первом запуске
// (см. tasks/safety-fixes.md #2).
func TestDefault_LocalNetsExcluded(t *testing.T) {
	s := Default()

	required := []string{
		"127.0.0.0/8",
		"169.254.0.0/16",
		"224.0.0.0/4",
		"240.0.0.0/4",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}
	for _, want := range required {
		found := false
		for _, got := range s.Excludes {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Default().Excludes не содержит %q (полученные: %v)", want, s.Excludes)
		}
	}

	if len(s.AdminPorts) != 2 || s.AdminPorts[0] != 22 || s.AdminPorts[1] != 222 {
		t.Errorf("Default().AdminPorts = %v, ожидалось [22 222]", s.AdminPorts)
	}
	if s.AdminPort != 0 {
		t.Errorf("Default().AdminPort = %d, ожидалось 0 (legacy field)", s.AdminPort)
	}
}

// TestLoad_MigratesAdminPort — старые state.json с admin_port:22 должны
// мигрировать в admin_ports:[22] при загрузке.
func TestLoad_MigratesAdminPort(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/state.json"
	legacy := `{"mode":"policy","admin_port":2222}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(s.AdminPorts) != 1 || s.AdminPorts[0] != 2222 {
		t.Errorf("после миграции AdminPorts = %v, ожидалось [2222]", s.AdminPorts)
	}
	if s.AdminPort != 0 {
		t.Errorf("legacy AdminPort должен быть обнулён, получено %d", s.AdminPort)
	}
}

// TestState_Validate_RejectsInvalid — Save валидирует State и отвергает
// невалидные значения (порт 0, неизвестный Mode).
func TestState_Validate_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		s    *State
	}{
		{"port 0", &State{Mode: "proxy", Ports: []uint16{0, 80}}},
		{"unknown mode", &State{Mode: "evil"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Save(filepath.Join(t.TempDir(), "s.json"), c.s)
			if err == nil {
				t.Errorf("ожидалась ошибка валидации")
			}
		})
	}
}

func TestLoad_NormalisesNilSlices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"mode":"policy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Outbounds == nil || s.Ports == nil || s.Excludes == nil {
		t.Errorf("ожидались инициализированные пустые слайсы, получено %+v", s)
	}
}

// TestLoad_MigratesEmptyCoreToDefault — старые state.json без поля core
// при загрузке должны получить DefaultCore = "sing-box".
func TestLoad_MigratesEmptyCoreToDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"mode":"policy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Core != DefaultCore {
		t.Errorf("Core после миграции = %q, ожидалось %q", s.Core, DefaultCore)
	}
}

// TestSaveLoad_PreservesCanonicalFields — canonical-поля (Protocol/Transport/TLS/Proto)
// внутри Outbound сохраняются в state.json и читаются обратно (Phase D.4.2).
func TestSaveLoad_PreservesCanonicalFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := Default()
	st.Outbounds = []types.Outbound{
		{
			Tag:      "proxy",
			Type:     "vless",
			Server:   "example.com",
			Port:     443,
			Protocol: types.ProtocolVLESS,
			Transport: &types.Transport{
				Kind: types.TransportXHTTP,
				Mode: types.XHTTPPacketUp,
				Path: "/v2",
			},
			TLS: &types.TLSConfig{
				Enabled:    true,
				ServerName: "example.com",
				Reality: &types.RealityConfig{
					Enabled:   true,
					PublicKey: "pk",
					ShortID:   "deadbeef",
				},
			},
			Proto: &types.ProtoOpts{
				UUID:       "uuid-x",
				Flow:       "xtls-rprx-vision-udp443",
				Encryption: "mlkem768x25519plus",
			},
		},
	}
	if err := Save(path, st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Outbounds) == 0 {
		t.Fatal("Outbounds пусты после Load")
	}
	ob := got.Outbounds[0]
	if ob.Protocol != types.ProtocolVLESS {
		t.Errorf("Protocol = %q, ожидалось %q", ob.Protocol, types.ProtocolVLESS)
	}
	if ob.Transport == nil || ob.Transport.Mode != types.XHTTPPacketUp {
		t.Errorf("Transport.Mode потерян: %+v", ob.Transport)
	}
	if ob.TLS == nil || ob.TLS.Reality == nil || ob.TLS.Reality.PublicKey != "pk" {
		t.Errorf("TLS.Reality потерян: %+v", ob.TLS)
	}
	if ob.Proto == nil || ob.Proto.Flow != "xtls-rprx-vision-udp443" {
		t.Errorf("Proto.Flow потерян: %+v", ob.Proto)
	}
}

// TestLoad_MigratesLegacyOutboundCanonicals — старые state.json с полем
// outbound_canonicals при Load() переносят canonical-данные в inline-поля
// каждого Outbound (однократная миграция Phase D.4.2).
func TestLoad_MigratesLegacyOutboundCanonicals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	legacy := `{
		"mode": "policy",
		"outbounds": [
			{"tag": "proxy", "type": "vless", "server": "example.com", "server_port": 443}
		],
		"outbound_canonicals": {
			"proxy": {
				"protocol": "vless",
				"transport": {"kind": "grpc", "service_name": "grpc-svc"},
				"tls": {"enabled": true, "server_name": "example.com",
					"reality": {"enabled": true, "public_key": "migrated-pk", "short_id": "ff00"}},
				"proto": {"uuid": "migrated-uuid", "flow": "xtls-rprx-vision-udp443"}
			}
		}
	}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Outbounds) == 0 {
		t.Fatal("Outbounds пусты после Load legacy")
	}
	ob := got.Outbounds[0]
	if ob.Protocol != types.ProtocolVLESS {
		t.Errorf("Protocol не мигрирован: %q", ob.Protocol)
	}
	if ob.Transport == nil || ob.Transport.ServiceName != "grpc-svc" {
		t.Errorf("Transport не мигрирован: %+v", ob.Transport)
	}
	if ob.TLS == nil || ob.TLS.Reality == nil || ob.TLS.Reality.PublicKey != "migrated-pk" {
		t.Errorf("TLS.Reality не мигрирована: %+v", ob.TLS)
	}
	if ob.Proto == nil || ob.Proto.UUID != "migrated-uuid" {
		t.Errorf("Proto.UUID не мигрирован: %+v", ob.Proto)
	}
}

// TestLoad_LegacyOutboundCanonicals_RoundTrip — round-trip тест:
// legacy state.json (с outbound_canonicals + port) → Load → Save → re-Load →
// Outbound содержит Protocol/TLS/Proto; сохранённый state не содержит outbound_canonicals.
func TestLoad_LegacyOutboundCanonicals_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "legacy.json")
	newPath := filepath.Join(dir, "new.json")

	// state.json с legacy-форматом: port вместо server_port + outbound_canonicals
	legacy := `{
		"mode": "full",
		"outbounds": [
			{"tag": "proxy", "type": "vless", "server": "1.2.3.4", "port": 443}
		],
		"outbound_canonicals": {
			"proxy": {
				"protocol": "vless",
				"tls": {"enabled": true, "server_name": "rt-test.example.com"},
				"proto": {"uuid": "rt-uuid"}
			}
		}
	}`
	if err := os.WriteFile(legacyPath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	// Load мигрирует
	loaded, err := Load(legacyPath)
	if err != nil {
		t.Fatalf("Load legacy: %v", err)
	}

	// Port должен был мигрировать из "port" → Port
	if len(loaded.Outbounds) == 0 || loaded.Outbounds[0].Port != 443 {
		t.Errorf("Port не мигрирован из legacy 'port': %+v", loaded.Outbounds)
	}

	// Protocol мигрирован из outbound_canonicals
	if loaded.Outbounds[0].Protocol != types.ProtocolVLESS {
		t.Errorf("Protocol не мигрирован: %q", loaded.Outbounds[0].Protocol)
	}

	// Сохраняем в новом формате
	if err := Save(newPath, loaded); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Re-load и проверяем отсутствие outbound_canonicals в JSON
	rawJSON, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(rawJSON) != "" {
		if bytes.Contains(rawJSON, []byte("outbound_canonicals")) {
			t.Error("новый state.json не должен содержать outbound_canonicals")
		}
	}

	// Re-load
	reloaded, err := Load(newPath)
	if err != nil {
		t.Fatalf("Load new: %v", err)
	}
	ob := reloaded.Outbounds[0]
	if ob.Protocol != types.ProtocolVLESS {
		t.Errorf("Protocol потерян после re-load: %q", ob.Protocol)
	}
	if ob.TLS == nil || ob.TLS.ServerName != "rt-test.example.com" {
		t.Errorf("TLS.ServerName потерян после re-load: %+v", ob.TLS)
	}
	if ob.Proto == nil || ob.Proto.UUID != "rt-uuid" {
		t.Errorf("Proto.UUID потерян после re-load: %+v", ob.Proto)
	}
}

// TestSaveLoad_PreservesCore — выбранное ядро сохраняется и читается обратно.
func TestSaveLoad_PreservesCore(t *testing.T) {
	cases := []string{"sing-box", "xray", "mihomo"}
	for _, core := range cases {
		t.Run(core, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			st := Default()
			st.Core = core
			if err := Save(path, st); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got.Core != core {
				t.Errorf("Core = %q, ожидалось %q", got.Core, core)
			}
		})
	}
}

// TestState_Validate_RejectsUnknownCore — неизвестное имя ядра отвергается.
func TestState_Validate_RejectsUnknownCore(t *testing.T) {
	s := Default()
	s.Core = "v2ray-evil"
	err := Save(filepath.Join(t.TempDir(), "s.json"), s)
	if err == nil {
		t.Fatal("ожидалась ошибка валидации для неизвестного ядра")
	}
}

// TestLoad_MigratesLegacyMode — режимы proxy/dpi/hybrid из старого state.json
// при загрузке мигрируют в ModePolicy. Save() новой версии вернёт ModePolicy.
func TestLoad_MigratesLegacyMode(t *testing.T) {
	cases := []string{"proxy", "dpi", "hybrid"}
	for _, legacy := range cases {
		t.Run(legacy, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			body := `{"mode":"` + legacy + `","outbounds":[{"tag":"direct","type":"direct"}]}`
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			s, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if s.Mode != types.ModePolicy {
				t.Errorf("Mode после миграции = %q, ожидалось %q", s.Mode, types.ModePolicy)
			}
			if s.PolicyName != DefaultPolicyName {
				t.Errorf("PolicyName = %q, ожидалось %q", s.PolicyName, DefaultPolicyName)
			}
		})
	}
}

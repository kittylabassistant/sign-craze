package types

import (
	"encoding/json"
	"testing"
)

// TestOutbound_Marshal_CanonicalFields — canonical-поля (Protocol/Transport/TLS/Proto)
// сериализуются в state.json как обычные struct-поля.
// Flatten Settings → sing-box делает renderOutboundJSON в internal/singbox/render.go.
func TestOutbound_Marshal_CanonicalFields(t *testing.T) {
	o := Outbound{
		Tag:      "vless-proxy",
		Type:     "vless",
		Server:   "srv",
		Port:     443,
		Protocol: ProtocolVLESS,
		Transport: &Transport{
			Kind: TransportGRPC,
			ServiceName: "grpc-service",
		},
		TLS: &TLSConfig{
			Enabled:    true,
			ServerName: "example.com",
		},
		Proto: &ProtoOpts{
			UUID: "abc-uuid",
			Flow: "xtls-rprx-vision",
		},
	}
	data, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Базовые поля
	if got["tag"] != "vless-proxy" || got["type"] != "vless" || got["server"] != "srv" {
		t.Errorf("базовые поля: %+v", got)
	}
	// server_port — struct tag
	if got["server_port"] == nil {
		t.Errorf("server_port отсутствует: %+v", got)
	}
	// protocol должен присутствовать (canonical)
	if got["protocol"] != "vless" {
		t.Errorf("protocol = %v, ожидалось vless", got["protocol"])
	}
	// transport, tls, proto — canonical поля (не flatten!)
	if got["transport"] == nil {
		t.Errorf("transport отсутствует в state.json: %+v", got)
	}
	if got["tls"] == nil {
		t.Errorf("tls отсутствует в state.json: %+v", got)
	}
	if got["proto"] == nil {
		t.Errorf("proto отсутствует в state.json: %+v", got)
	}
	// Settings НЕ flatten-ятся в state.json (нет поля "uuid" на top-level)
	if got["uuid"] != nil {
		t.Errorf("uuid не должен быть на top-level в state.json: %+v", got)
	}
}

// TestOutbound_RoundTrip_PreservesPort — критичный регресс: "server_port" tag
// должен корректно round-trip'ать через state.json.
func TestOutbound_RoundTrip_PreservesPort(t *testing.T) {
	src := Outbound{
		Tag:      "vless-test",
		Type:     "vless",
		Server:   "example.invalid",
		Port:     443,
		Protocol: ProtocolVLESS,
		Proto: &ProtoOpts{
			UUID: "00000000-0000-0000-0000-000000000000",
		},
		TLS: &TLSConfig{Enabled: true},
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Outbound
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Port != 443 {
		t.Errorf("Port lost in round-trip: got %d, want 443. JSON=%s", got.Port, data)
	}
	if got.Tag != src.Tag || got.Type != src.Type || got.Server != src.Server {
		t.Errorf("базовые поля потеряны: %+v", got)
	}
	if got.Protocol != ProtocolVLESS {
		t.Errorf("Protocol потерян: %+v", got.Protocol)
	}
	if got.Proto == nil || got.Proto.UUID != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("Proto.UUID потерян: %+v", got.Proto)
	}
}

// TestOutbound_Unmarshal_LegacyPortKey — приём старого ключа "port"
// для совместимости со state.json, написанными до фикса.
func TestOutbound_Unmarshal_LegacyPortKey(t *testing.T) {
	data := []byte(`{"tag":"x","type":"vless","server":"s","port":1234}`)
	var o Outbound
	if err := json.Unmarshal(data, &o); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if o.Port != 1234 {
		t.Errorf("legacy port не прочитан: got %d", o.Port)
	}
}

func TestMode_Validate(t *testing.T) {
	tests := []struct {
		mode    Mode
		wantErr bool
	}{
		{ModePolicy, false},
		{ModeFull, false},
		{"", true},
		{"xray", true},
		{"POLICY", true},
		{"proxy", true},  // legacy, миграция выполняется в state.Load(), Validate отвергает
		{"dpi", true},    // legacy
		{"hybrid", true}, // legacy
	}
	for _, tt := range tests {
		err := tt.mode.Validate()
		if (err != nil) != tt.wantErr {
			t.Errorf("Mode(%q).Validate() error = %v, wantErr %v", tt.mode, err, tt.wantErr)
		}
	}
}

func TestLegacyModes_AllMigrateToPolicy(t *testing.T) {
	for legacy, target := range LegacyModes {
		if target != ModePolicy {
			t.Errorf("LegacyModes[%q] = %q, ожидалось ModePolicy", legacy, target)
		}
	}
	for _, want := range []Mode{"proxy", "dpi", "hybrid"} {
		if _, ok := LegacyModes[want]; !ok {
			t.Errorf("LegacyModes не содержит %q", want)
		}
	}
}

func TestArch_Validate(t *testing.T) {
	tests := []struct {
		arch    Arch
		wantErr bool
	}{
		{ArchARM64, false},
		{ArchARM7, false},
		{ArchMIPSLE, false},
		{ArchMIPS, false},
		{ArchAMD64, false},
		{"x86", true},
		{"", true},
		{"ARM64", true},
	}
	for _, tt := range tests {
		err := tt.arch.Validate()
		if (err != nil) != tt.wantErr {
			t.Errorf("Arch(%q).Validate() error = %v, wantErr %v", tt.arch, err, tt.wantErr)
		}
	}
}

func TestArch_GoEnv(t *testing.T) {
	tests := []struct {
		arch     Arch
		wantKeys []string
	}{
		{ArchARM64, []string{"GOOS", "GOARCH"}},
		{ArchARM7, []string{"GOOS", "GOARCH", "GOARM"}},
		{ArchMIPSLE, []string{"GOOS", "GOARCH", "GOMIPS"}},
		{ArchMIPS, []string{"GOOS", "GOARCH", "GOMIPS"}},
	}
	for _, tt := range tests {
		env := tt.arch.GoEnv()
		for _, k := range tt.wantKeys {
			if _, ok := env[k]; !ok {
				t.Errorf("Arch(%q).GoEnv() не содержит ключ %q", tt.arch, k)
			}
		}
	}
}

func TestArch_GoEnv_MIPSLESoftfloat(t *testing.T) {
	env := ArchMIPSLE.GoEnv()
	if env["GOMIPS"] != "softfloat" {
		t.Errorf("MIPSLE должен использовать softfloat, получили %q", env["GOMIPS"])
	}
}

func TestPort_Validate(t *testing.T) {
	if err := Port(0).Validate(); err == nil {
		t.Error("Port(0) должен возвращать ошибку")
	}
	if err := Port(7895).Validate(); err != nil {
		t.Errorf("Port(7895).Validate() = %v", err)
	}
	if err := Port(65535).Validate(); err != nil {
		t.Errorf("Port(65535).Validate() = %v", err)
	}
}

func TestPortRange_Validate(t *testing.T) {
	tests := []struct {
		r       PortRange
		wantErr bool
	}{
		{PortRange{80, 80}, false},
		{PortRange{80, 443}, false},
		{PortRange{443, 80}, true}, // from > to
		{PortRange{0, 80}, true},   // from == 0
		{PortRange{80, 0}, true},   // to == 0
	}
	for _, tt := range tests {
		err := tt.r.Validate()
		if (err != nil) != tt.wantErr {
			t.Errorf("PortRange{%d,%d}.Validate() error = %v, wantErr %v",
				tt.r.From, tt.r.To, err, tt.wantErr)
		}
	}
}

func TestOutbound_Validate(t *testing.T) {
	valid := Outbound{Tag: "proxy", Type: "socks", Server: "1.2.3.4", Port: 1080}
	if err := valid.Validate(); err != nil {
		t.Errorf("валидный Outbound: %v", err)
	}

	noTag := valid
	noTag.Tag = ""
	if err := noTag.Validate(); err == nil {
		t.Error("ожидалась ошибка при пустом Tag")
	}

	noType := valid
	noType.Type = ""
	if err := noType.Validate(); err == nil {
		t.Error("ожидалась ошибка при пустом Type")
	}

	noServer := valid
	noServer.Server = ""
	if err := noServer.Validate(); err == nil {
		t.Error("ожидалась ошибка при пустом Server")
	}

	zeroPort := valid
	zeroPort.Port = 0
	if err := zeroPort.Validate(); err == nil {
		t.Error("ожидалась ошибка при Port=0")
	}
}

// TestOutbound_TagFormat — некорректный tag (кавычки, пробелы, JSON-meta)
// отвергается. Защита от template-injection в config.json.
func TestOutbound_TagFormat(t *testing.T) {
	good := []string{"proxy", "vless-proxy", "tag_with_underscore", "tag.dot", "a-b.c_d-e", "x"}
	bad := []string{"", `tag"with-quote`, "tag with space", "tag\nnewline", "<script>", "тег-кириллица", "a/b"}

	for _, tag := range good {
		o := Outbound{Tag: tag, Type: "socks", Server: "1.2.3.4", Port: 1080}
		if err := o.Validate(); err != nil {
			t.Errorf("Tag=%q должен быть валиден: %v", tag, err)
		}
	}
	for _, tag := range bad {
		o := Outbound{Tag: tag, Type: "socks", Server: "1.2.3.4", Port: 1080}
		if err := o.Validate(); err == nil {
			t.Errorf("Tag=%q должен быть отвергнут", tag)
		}
	}
}

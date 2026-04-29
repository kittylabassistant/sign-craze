package types

import (
	"encoding/json"
	"testing"
)

// TestOutbound_MarshalJSON_FlatSettings — Settings мерджатся как top-level
// поля (sing-box формат), не заворачиваются в "settings" (XRay формат).
func TestOutbound_MarshalJSON_FlatSettings(t *testing.T) {
	o := Outbound{
		Tag:    "vless-proxy",
		Type:   "vless",
		Server: "srv",
		Port:   443,
		Settings: map[string]any{
			"uuid":       "abc",
			"flow":       "xtls-rprx-vision",
			"encryption": "mlkem768x25519plus",
			"tls": map[string]any{
				"enabled": true,
				"reality": map[string]any{
					"enabled":        true,
					"public_key":     "PK",
					"mldsa65_verify": "VK",
				},
			},
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

	// Settings должны быть распакованы НА ВЕРХНЕМ УРОВНЕ, не в "settings"
	if _, has := got["settings"]; has {
		t.Errorf("'settings' wrapper не должен присутствовать в sing-box формате: %s", data)
	}
	if got["uuid"] != "abc" || got["flow"] != "xtls-rprx-vision" {
		t.Errorf("uuid/flow не на верхнем уровне: %s", data)
	}
	if got["encryption"] != "mlkem768x25519plus" {
		t.Errorf("encryption (PQ) не на верхнем уровне: %s", data)
	}

	tls, ok := got["tls"].(map[string]any)
	if !ok {
		t.Fatalf("tls не объект: %s", data)
	}
	reality, _ := tls["reality"].(map[string]any)
	if reality["mldsa65_verify"] != "VK" {
		t.Errorf("mldsa65_verify не сохранён: %+v", reality)
	}
}

func TestMode_Validate(t *testing.T) {
	tests := []struct {
		mode    Mode
		wantErr bool
	}{
		{ModeProxy, false},
		{ModeDPI, false},
		{ModeHybrid, false},
		{"", true},
		{"xray", true},
		{"PROXY", true},
	}
	for _, tt := range tests {
		err := tt.mode.Validate()
		if (err != nil) != tt.wantErr {
			t.Errorf("Mode(%q).Validate() error = %v, wantErr %v", tt.mode, err, tt.wantErr)
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

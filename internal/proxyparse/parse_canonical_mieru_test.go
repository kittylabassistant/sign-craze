package proxyparse

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestParseCanonical_MieruSimple_HappyPath — основной mierus:// формат.
func TestParseCanonical_MieruSimple_HappyPath(t *testing.T) {
	rawURL := "mierus://alice:hunter2@vps.example.com" +
		"?port=8080&protocol=TCP&multiplexing=MULTIPLEXING_HIGH&profile=prod" +
		"#mieru-prod"

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}

	if ob.Tag != "mieru-prod" {
		t.Errorf("Tag = %q, ожидалось mieru-prod", ob.Tag)
	}
	if ob.Type != "socks" {
		t.Errorf("Type = %q, ожидалось socks", ob.Type)
	}
	if ob.Server != "vps.example.com" {
		t.Errorf("Server = %q", ob.Server)
	}
	if ob.Port != 8080 {
		t.Errorf("Port = %d, ожидалось 8080", ob.Port)
	}
	if canon.Protocol != types.ProtocolMieru {
		t.Errorf("Protocol = %q", canon.Protocol)
	}
	if canon.Proto == nil {
		t.Fatal("Proto = nil")
	}
	if canon.Proto.MieruUsername != "alice" {
		t.Errorf("MieruUsername = %q", canon.Proto.MieruUsername)
	}
	if canon.Proto.MieruPassword != "hunter2" {
		t.Errorf("MieruPassword = %q", canon.Proto.MieruPassword)
	}
	if canon.Proto.MieruPort != 8080 {
		t.Errorf("MieruPort = %d", canon.Proto.MieruPort)
	}
	if canon.Proto.MieruProtocol != "TCP" {
		t.Errorf("MieruProtocol = %q", canon.Proto.MieruProtocol)
	}
	if canon.Proto.MieruMultiplexing != "MULTIPLEXING_HIGH" {
		t.Errorf("MieruMultiplexing = %q", canon.Proto.MieruMultiplexing)
	}
	if canon.Proto.MieruProfile != "prod" {
		t.Errorf("MieruProfile = %q", canon.Proto.MieruProfile)
	}
	if canon.Proto.MieruLocalPort != 0 {
		t.Errorf("MieruLocalPort = %d, ожидалось 0 (заполняется при --start)", canon.Proto.MieruLocalPort)
	}
}

// TestParseCanonical_MieruSimple_Defaults — опциональные поля → defaults.
func TestParseCanonical_MieruSimple_Defaults(t *testing.T) {
	rawURL := "mierus://user:pass@1.2.3.4?port=2027"

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}
	if ob.Tag != "mieru-out" {
		t.Errorf("Tag default = %q, ожидалось mieru-out", ob.Tag)
	}
	if canon.Proto.MieruProtocol != "TCP" {
		t.Errorf("MieruProtocol default = %q", canon.Proto.MieruProtocol)
	}
	if canon.Proto.MieruMultiplexing != "MULTIPLEXING_LOW" {
		t.Errorf("MieruMultiplexing default = %q", canon.Proto.MieruMultiplexing)
	}
	if canon.Proto.MieruProfile != "default" {
		t.Errorf("MieruProfile default = %q", canon.Proto.MieruProfile)
	}
}

// TestParseCanonical_MieruSimple_UDP — protocol=UDP.
func TestParseCanonical_MieruSimple_UDP(t *testing.T) {
	rawURL := "mierus://u:p@host?port=443&protocol=UDP"
	_, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}
	if canon.Proto.MieruProtocol != "UDP" {
		t.Errorf("MieruProtocol = %q", canon.Proto.MieruProtocol)
	}
}

// TestParseCanonical_MieruSimple_Errors — все ошибочные кейсы.
func TestParseCanonical_MieruSimple_Errors(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"нет username", "mierus://:pass@host?port=8080", "username"},
		{"нет password", "mierus://user@host?port=8080", "password"},
		{"нет порта", "mierus://u:p@host", "port обязателен"},
		{"кривой порт", "mierus://u:p@host?port=0", "port"},
		{"плохой protocol", "mierus://u:p@host?port=80&protocol=ICMP", "protocol"},
		{"плохой multiplexing", "mierus://u:p@host?port=80&multiplexing=INSANE", "multiplexing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := ParseCanonical(c.input)
			if err == nil {
				t.Fatalf("ожидалась ошибка")
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v; ожидалось содержит %q", err, c.wantErr)
			}
		})
	}
}

// buildMieruProto собирает минимальный protobuf-config из полей.
// Используется в тестах для генерации валидного wire-format payload.
func buildMieruProto(profileName, username, password, server string, port uint32, protoNum int, muxLevel int) []byte {
	// User { name=1 password=2 }
	user := append(encodeField(1, wireBytes, encodeString(username)),
		encodeField(2, wireBytes, encodeString(password))...)

	// PortBinding { port=1 protocol=2 }
	pb := append(encodeField(1, wireVarint, encodeVarint(uint64(port))),
		encodeField(2, wireVarint, encodeVarint(uint64(protoNum)))...)

	// ServerEndpoint { ipAddress=1, portBindings=3 }
	server1 := append(encodeField(1, wireBytes, encodeString(server)),
		encodeField(3, wireBytes, pb)...)

	// MultiplexingConfig { level=1 }
	mux := encodeField(1, wireVarint, encodeVarint(uint64(muxLevel)))

	// Profile { profileName=1 user=2 servers=3 multiplexing=4 }
	profile := encodeField(1, wireBytes, encodeString(profileName))
	profile = append(profile, encodeField(2, wireBytes, user)...)
	profile = append(profile, encodeField(3, wireBytes, server1)...)
	profile = append(profile, encodeField(4, wireBytes, mux)...)

	// ClientConfig { profiles=1, socks5Port=4 (просто чтобы проверить skip unknown) }
	cfg := encodeField(1, wireBytes, profile)
	cfg = append(cfg, encodeField(4, wireVarint, encodeVarint(1080))...)
	return cfg
}

func encodeField(fieldNum, wireType int, val []byte) []byte {
	tag := encodeVarint(uint64(fieldNum<<3 | wireType))
	if wireType == wireBytes {
		return append(append(tag, encodeVarint(uint64(len(val)))...), val...)
	}
	return append(tag, val...)
}

func encodeString(s string) []byte { return []byte(s) }

func encodeVarint(v uint64) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

// TestParseCanonical_MieruProto_HappyPath — base64-protobuf формат.
func TestParseCanonical_MieruProto_HappyPath(t *testing.T) {
	cfg := buildMieruProto("prod", "alice", "hunter2", "1.2.3.4", 8080, 1 /*TCP*/, 4 /*HIGH*/)
	encoded := base64.RawURLEncoding.EncodeToString(cfg)
	rawURL := "mieru://" + encoded + "#proto-tag"

	ob, canon, err := ParseCanonical(rawURL)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}
	if ob.Tag != "proto-tag" {
		t.Errorf("Tag = %q", ob.Tag)
	}
	if ob.Server != "1.2.3.4" {
		t.Errorf("Server = %q", ob.Server)
	}
	if ob.Port != 8080 {
		t.Errorf("Port = %d", ob.Port)
	}
	if canon.Proto.MieruUsername != "alice" {
		t.Errorf("Username = %q", canon.Proto.MieruUsername)
	}
	if canon.Proto.MieruPassword != "hunter2" {
		t.Errorf("Password = %q", canon.Proto.MieruPassword)
	}
	if canon.Proto.MieruProfile != "prod" {
		t.Errorf("Profile = %q", canon.Proto.MieruProfile)
	}
	if canon.Proto.MieruProtocol != "TCP" {
		t.Errorf("Protocol = %q", canon.Proto.MieruProtocol)
	}
	if canon.Proto.MieruMultiplexing != "MULTIPLEXING_HIGH" {
		t.Errorf("Multiplexing = %q", canon.Proto.MieruMultiplexing)
	}
}

// TestParseCanonical_MieruProto_DefaultTag — без fragment → tag=mieru-out.
func TestParseCanonical_MieruProto_DefaultTag(t *testing.T) {
	cfg := buildMieruProto("default", "u", "p", "host", 443, 1, 2)
	encoded := base64.RawURLEncoding.EncodeToString(cfg)
	ob, _, err := ParseCanonical("mieru://" + encoded)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}
	if ob.Tag != "mieru-out" {
		t.Errorf("Tag = %q", ob.Tag)
	}
}

// TestParseCanonical_MieruProto_StdBase64 — fallback на std-encoding.
func TestParseCanonical_MieruProto_StdBase64(t *testing.T) {
	cfg := buildMieruProto("default", "u", "p", "host", 80, 1, 2)
	encoded := base64.StdEncoding.EncodeToString(cfg)
	_, _, err := ParseCanonical("mieru://" + encoded)
	if err != nil {
		t.Fatalf("std base64: %v", err)
	}
}

// TestParseCanonical_MieruProto_Errors — некорректные protobuf.
func TestParseCanonical_MieruProto_Errors(t *testing.T) {
	// Полностью валидный protobuf, но содержит только socks5Port (field=4) —
	// не имеет profiles[0]. Должен ругаться на отсутствие профиля.
	onlySocks5Port := encodeField(4, wireVarint, encodeVarint(1080))
	noProfilesEnc := base64.RawURLEncoding.EncodeToString(onlySocks5Port)

	noServer := encodeField(1, wireBytes,
		encodeField(2, wireBytes, append(encodeField(1, wireBytes, encodeString("u")), encodeField(2, wireBytes, encodeString("p"))...)),
	)
	noServerEnc := base64.RawURLEncoding.EncodeToString(noServer)

	cases := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"плохой base64", "mieru://!!!not-base64!!!", "base64"},
		{"пусто после префикса", "mieru://", "пустой base64"},
		{"нет profiles", "mieru://" + noProfilesEnc, "profiles[0] не найден"},
		{"нет server", "mieru://" + noServerEnc, "server address"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := ParseCanonical(c.input)
			if err == nil {
				t.Fatalf("ожидалась ошибка")
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v; ожидалось содержит %q", err, c.wantErr)
			}
		})
	}
}

// TestParseCanonical_MieruProto_DomainName — server через field-tag=2 (domainName).
func TestParseCanonical_MieruProto_DomainName(t *testing.T) {
	// Сервер только через domainName (field=2), без ipAddress.
	user := append(encodeField(1, wireBytes, encodeString("u")), encodeField(2, wireBytes, encodeString("p"))...)
	pb := append(encodeField(1, wireVarint, encodeVarint(443)), encodeField(2, wireVarint, encodeVarint(2 /*UDP*/))...)
	server := append(encodeField(2, wireBytes, encodeString("vps.example.org")), encodeField(3, wireBytes, pb)...)
	profile := encodeField(2, wireBytes, user)
	profile = append(profile, encodeField(3, wireBytes, server)...)
	cfg := encodeField(1, wireBytes, profile)
	enc := base64.RawURLEncoding.EncodeToString(cfg)

	ob, canon, err := ParseCanonical("mieru://" + enc)
	if err != nil {
		t.Fatalf("ParseCanonical: %v", err)
	}
	if ob.Server != "vps.example.org" {
		t.Errorf("Server = %q", ob.Server)
	}
	if ob.Port != 443 {
		t.Errorf("Port = %d", ob.Port)
	}
	if canon.Proto.MieruProtocol != "UDP" {
		t.Errorf("Protocol = %q", canon.Proto.MieruProtocol)
	}
}

// FuzzMieruWireProto — гарантирует, что декодер не паникует на произвольных входах.
func FuzzMieruWireProto(f *testing.F) {
	// Seeds: пустота, валидный config, мусор
	f.Add([]byte{})
	f.Add(buildMieruProto("d", "u", "p", "host", 80, 1, 2))
	f.Add([]byte{0xff, 0xff, 0xff})
	f.Add([]byte{0x08, 0x05}) // varint field=1 wire=0
	f.Add([]byte{0x0a, 0xff, 0xff, 0xff}) // bytes с фиктивной длиной

	f.Fuzz(func(t *testing.T, data []byte) {
		// Не падать, ошибку возвращать корректно.
		_, _ = parseMieruWireConfig(data)
	})
}

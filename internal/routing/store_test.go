package routing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestLoad_NotExist проверяет, что отсутствующий файл возвращает (nil, nil).
func TestLoad_NotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	c, err := Load(path)
	if err != nil {
		t.Fatalf("ожидался nil error, получено: %v", err)
	}
	if c != nil {
		t.Fatalf("ожидался nil config, получено: %+v", c)
	}
}

// TestSave_Load_Roundtrip проверяет сохранение и последующую загрузку конфига.
func TestSave_Load_Roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")

	orig := &types.RoutingConfig{
		Version: SchemaVersion,
		Outbounds: []types.Outbound{
			{Tag: "direct", Type: "direct"},
			{
				Tag:    "proxy",
				Type:   "socks",
				Server: "127.0.0.1",
				Port:   1080,
			},
		},
		Rules: []types.RouteRule{
			{
				Domain:   []string{"example.com"},
				Outbound: "proxy",
			},
		},
		Final: "direct",
	}

	if err := Save(path, orig); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load вернул nil, ожидался конфиг")
	}

	// сравниваем через JSON (deep equal для вложенных map[string]any)
	origJSON, _ := json.Marshal(orig)
	gotJSON, _ := json.Marshal(got)
	if string(origJSON) != string(gotJSON) {
		t.Errorf("round-trip mismatch:\norig: %s\ngot:  %s", origJSON, gotJSON)
	}
}

// TestSave_AtomicPermissions проверяет, что файл после Save имеет права 0o640.
func TestSave_AtomicPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")

	c := Default()
	if err := Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	// проверяем права 0o640
	perm := info.Mode().Perm()
	if perm != 0o640 {
		t.Errorf("права файла: got %o, want %o", perm, 0o640)
	}
}

// TestLoad_InvalidJSON проверяет ошибку при некорректном JSON.
func TestLoad_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")
	if err := os.WriteFile(path, []byte("not-json{{{"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалась ошибка при некорректном JSON, получен nil")
	}
}

// bootstrapState — заглушка BootstrapState для тестов.
type bootstrapState struct {
	outbounds []types.Outbound
}

func (b *bootstrapState) GetOutbounds() []types.Outbound { return b.outbounds }

// TestBootstrapFromState_CreatesIfMissing проверяет, что при отсутствии routing.json
// файл создаётся и содержит outbounds из state, а также стандартный tun-inbound.
func TestBootstrapFromState_CreatesIfMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")

	st := &bootstrapState{
		outbounds: []types.Outbound{
			{Tag: "vless-grpc-craze", Type: "vless", Server: "example.com", Port: 443},
			{Tag: "direct", Type: "direct"},
		},
	}

	if err := BootstrapFromState(path, st); err != nil {
		t.Fatalf("BootstrapFromState: %v", err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load после bootstrap: %v", err)
	}
	if c == nil {
		t.Fatal("Load вернул nil после bootstrap")
	}

	// Проверяем версию схемы.
	if c.Version != SchemaVersion {
		t.Errorf("Version: got %d, want %d", c.Version, SchemaVersion)
	}

	// Должен быть один inbound tun-in.
	if len(c.Inbounds) != 1 || c.Inbounds[0].Tag != "tun-in" {
		t.Errorf("Inbounds: got %+v, want [{Tag:tun-in}]", c.Inbounds)
	}

	// Outbounds должны совпадать со state.
	if len(c.Outbounds) != 2 {
		t.Errorf("Outbounds len: got %d, want 2", len(c.Outbounds))
	}
	if c.Outbounds[0].Tag != "vless-grpc-craze" {
		t.Errorf("Outbounds[0].Tag: got %q, want %q", c.Outbounds[0].Tag, "vless-grpc-craze")
	}

	// Final — тег первого outbound.
	if c.Final != "vless-grpc-craze" {
		t.Errorf("Final: got %q, want %q", c.Final, "vless-grpc-craze")
	}
}

// TestBootstrapFromState_SkipsIfExists проверяет, что существующий routing.json
// не перезаписывается при повторном вызове BootstrapFromState.
func TestBootstrapFromState_SkipsIfExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")

	// Создаём исходный файл с известным содержимым.
	original := &types.RoutingConfig{
		Version:   SchemaVersion,
		Outbounds: []types.Outbound{{Tag: "direct", Type: "direct"}},
		Final:     "direct",
	}
	if err := Save(path, original); err != nil {
		t.Fatalf("Save original: %v", err)
	}

	// Пытаемся bootstrap с другими outbound'ами.
	st := &bootstrapState{
		outbounds: []types.Outbound{
			{Tag: "vless-grpc-craze", Type: "vless", Server: "example.com", Port: 443},
		},
	}
	if err := BootstrapFromState(path, st); err != nil {
		t.Fatalf("BootstrapFromState: %v", err)
	}

	// Файл не должен измениться.
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Outbounds) != 1 || c.Outbounds[0].Tag != "direct" {
		t.Errorf("файл был перезаписан: outbounds = %+v", c.Outbounds)
	}
}

// TestBootstrapFromState_EmptyOutbounds проверяет корректное поведение,
// когда state не содержит ни одного outbound.
func TestBootstrapFromState_EmptyOutbounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")

	st := &bootstrapState{outbounds: nil}
	if err := BootstrapFromState(path, st); err != nil {
		t.Fatalf("BootstrapFromState: %v", err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Final != "" {
		t.Errorf("Final при пустых outbounds: got %q, want %q", c.Final, "")
	}
}

// TestLoad_LegacyVersionZero проверяет, что файл с version=0 (legacy, без поля version)
// успешно загружается после миграции до SchemaVersion, а не возвращает ошибку.
func TestLoad_LegacyVersionZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")
	// version=0 — legacy-файл, записанный до введения поля version.
	data := []byte(`{"version": 0}`)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("ожидалась успешная загрузка legacy-файла, получено: %v", err)
	}
	if c == nil {
		t.Fatal("Load вернул nil, ожидался конфиг")
	}
	if c.Version != SchemaVersion {
		t.Errorf("Version после миграции: got %d, want %d", c.Version, SchemaVersion)
	}
}

// TestLoad_InvalidField проверяет ошибку при невалидном (но корректном JSON) конфиге,
// где ошибка не связана с version.
func TestLoad_InvalidField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")
	// rule с некорректным action — валидационная ошибка не в version.
	data := []byte(`{"version": 1, "rules": [{"action": "invalid-action"}]}`)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалась ошибка валидации, получен nil")
	}
}

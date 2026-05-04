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

// TestLoad_InvalidConfig проверяет ошибку при невалидном (но корректном JSON) конфиге.
func TestLoad_InvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")
	// version=0 — невалидно согласно Validate()
	data := []byte(`{"version": 0}`)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("ожидалась ошибка валидации, получен nil")
	}
}

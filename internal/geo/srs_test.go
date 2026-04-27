package geo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupTestServer(t *testing.T, manifest Manifest, files map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(manifest) //nolint:errcheck
	})

	for name, data := range files {
		data := data // захват переменной цикла
		mux.HandleFunc("/"+name, func(w http.ResponseWriter, r *http.Request) {
			w.Write(data) //nolint:errcheck
		})
	}

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestFetchManifest_ПарситJSON(t *testing.T) {
	m := Manifest{
		Version:   "20260427-001",
		UpdatedAt: "2026-04-27T00:00:00Z",
		Files: []ManifestEntry{
			{Name: "geosite-ru.srs", SHA256: "aabbcc", Size: 1234},
		},
	}
	srv := setupTestServer(t, m, nil)

	old := manifestURLVar
	manifestURLVar = srv.URL + "/manifest.json"
	defer func() { manifestURLVar = old }()

	got, err := FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest: %v", err)
	}
	if got.Version != "20260427-001" {
		t.Errorf("Version = %q, ожидался 20260427-001", got.Version)
	}
	if len(got.Files) != 1 || got.Files[0].Name != "geosite-ru.srs" {
		t.Errorf("Files = %v, ожидался [geosite-ru.srs]", got.Files)
	}
}

func TestUpdate_СкачиваетТолькоИзменённые(t *testing.T) {
	content1 := []byte("srs-content-ru")
	content2 := []byte("srs-content-google")

	m := Manifest{
		Version:   "20260427-001",
		UpdatedAt: "2026-04-27T00:00:00Z",
		Files: []ManifestEntry{
			{Name: "geosite-ru.srs", SHA256: sha256Hex(content1), Size: int64(len(content1))},
			{Name: "geosite-google.srs", SHA256: sha256Hex(content2), Size: int64(len(content2))},
		},
	}
	srv := setupTestServer(t, m, map[string][]byte{
		"geosite-ru.srs":     content1,
		"geosite-google.srs": content2,
	})

	oldManifest := manifestURLVar
	oldBase := downloadBaseURLVar
	manifestURLVar = srv.URL + "/manifest.json"
	downloadBaseURLVar = srv.URL + "/"
	defer func() {
		manifestURLVar = oldManifest
		downloadBaseURLVar = oldBase
	}()

	geoDir := t.TempDir()

	// Предварительно кладём актуальный geosite-ru.srs (совпадает SHA256 → не скачиваем)
	if err := os.WriteFile(filepath.Join(geoDir, "geosite-ru.srs"), content1, 0o644); err != nil {
		t.Fatal(err)
	}

	// Ожидаем: geosite-ru.srs — пропуск, geosite-google.srs — скачивание
	updated, err := Update(context.Background(), []string{"geosite-ru.srs", "geosite-google.srs"}, geoDir)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, ожидалось 1", updated)
	}

	// Проверяем содержимое скачанного файла
	got, err := os.ReadFile(filepath.Join(geoDir, "geosite-google.srs"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(content2) {
		t.Errorf("содержимое geosite-google.srs не совпадает")
	}
}

func TestUpdate_ОшибкаЕслиКатегорияОтсутствуетВМанифесте(t *testing.T) {
	m := Manifest{
		Version: "v1",
		Files:   []ManifestEntry{},
	}
	srv := setupTestServer(t, m, nil)

	old := manifestURLVar
	manifestURLVar = srv.URL + "/manifest.json"
	defer func() { manifestURLVar = old }()

	_, err := Update(context.Background(), []string{"geosite-nonexistent.srs"}, t.TempDir())
	if err == nil {
		t.Error("ожидалась ошибка для отсутствующей категории")
	}
}

func TestUpdate_ОтклоняетНесовпадающийSHA256(t *testing.T) {
	content := []byte("real-content")

	m := Manifest{
		Version: "v1",
		Files: []ManifestEntry{
			{Name: "geosite-ru.srs", SHA256: "0000000000000000000000000000000000000000000000000000000000000000", Size: 0},
		},
	}
	srv := setupTestServer(t, m, map[string][]byte{
		"geosite-ru.srs": content,
	})

	oldManifest := manifestURLVar
	oldBase := downloadBaseURLVar
	manifestURLVar = srv.URL + "/manifest.json"
	downloadBaseURLVar = srv.URL + "/"
	defer func() {
		manifestURLVar = oldManifest
		downloadBaseURLVar = oldBase
	}()

	_, err := Update(context.Background(), []string{"geosite-ru.srs"}, t.TempDir())
	if err == nil {
		t.Error("ожидалась ошибка при несовпадении SHA256")
	}
}

func TestLocalSHA256_СовпадаетСЭталонным(t *testing.T) {
	data := []byte("test data for hashing")
	expected := sha256Hex(data)

	f, err := os.CreateTemp(t.TempDir(), "hash-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	f.Write(data) //nolint:errcheck
	f.Close()

	got := localSHA256(f.Name())
	if got != expected {
		t.Errorf("localSHA256 = %s, ожидался %s", got, expected)
	}
}

func TestLocalSHA256_ВозвращаетПустуюДляОтсутствующегоФайла(t *testing.T) {
	got := localSHA256("/nonexistent/path/file.srs")
	if got != "" {
		t.Errorf("ожидалась пустая строка для отсутствующего файла, получено %q", got)
	}
}

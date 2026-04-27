package singbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// fakeRelease строит JSON-ответ GitHub API с одним asset.
func fakeRelease(assetName, downloadURL string) []byte {
	rel := types.Release{
		TagName: "v1.10.0",
		Assets: []types.Asset{
			{
				Name:               assetName,
				BrowserDownloadURL: downloadURL,
				Size:               42,
			},
		},
	}
	data, _ := json.Marshal(rel)
	return data
}

func TestDownload_Success(t *testing.T) {
	content := []byte("fake-tarball-content")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "releases/latest"):
			assetURL := "http://" + r.Host + "/download/sing-box-v1.10.0-linux-arm64.tar.gz"
			w.Header().Set("Content-Type", "application/json")
			w.Write(fakeRelease("sing-box-v1.10.0-linux-arm64.tar.gz", assetURL))
		default:
			w.Header().Set("ETag", `"abc123"`)
			w.Write(content)
		}
	}))
	defer srv.Close()

	// подменяем URL GitHub API на тестовый сервер
	origURL := githubReleasesURL
	githubReleasesURL = srv.URL + "/repos/SagerNet/sing-box/releases/latest"
	defer func() { githubReleasesURL = origURL }()

	dir := t.TempDir()
	res, err := Download(context.Background(), types.ArchARM64, dir)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !res.Downloaded {
		t.Error("ожидался Downloaded=true")
	}
	if res.Version != "v1.10.0" {
		t.Errorf("Version = %q, ожидалось %q", res.Version, "v1.10.0")
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("файл не создан: %v", err)
	}

	got, _ := os.ReadFile(res.Path)
	if string(got) != string(content) {
		t.Errorf("содержимое файла = %q, ожидалось %q", got, content)
	}
}

func TestDownload_ETagSkipsRedownload(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "releases/latest"):
			assetURL := "http://" + r.Host + "/download/sing-box-v1.10.0-linux-arm64.tar.gz"
			w.Write(fakeRelease("sing-box-v1.10.0-linux-arm64.tar.gz", assetURL))
		default:
			calls++
			if r.Header.Get("If-None-Match") == `"etag-v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"etag-v1"`)
			w.Write([]byte("data"))
		}
	}))
	defer srv.Close()

	origURL := githubReleasesURL
	githubReleasesURL = srv.URL + "/repos/SagerNet/sing-box/releases/latest"
	defer func() { githubReleasesURL = origURL }()

	dir := t.TempDir()

	// первая загрузка
	if _, err := Download(context.Background(), types.ArchARM64, dir); err != nil {
		t.Fatalf("первый Download: %v", err)
	}

	// вторая загрузка — должна вернуть 304
	res, err := Download(context.Background(), types.ArchARM64, dir)
	if err != nil {
		t.Fatalf("второй Download: %v", err)
	}
	if res.Downloaded {
		t.Error("ожидался Downloaded=false при 304 Not Modified")
	}
}

func TestDownload_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "releases/latest") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	origURL := githubReleasesURL
	githubReleasesURL = srv.URL + "/repos/SagerNet/sing-box/releases/latest"
	defer func() { githubReleasesURL = origURL }()

	_, err := Download(context.Background(), types.ArchARM64, t.TempDir())
	if err == nil {
		t.Fatal("ожидалась ошибка при HTTP 500")
	}
}

func TestDownload_UnsupportedArch(t *testing.T) {
	_, err := Download(context.Background(), "riscv64", t.TempDir())
	if err == nil {
		t.Fatal("ожидалась ошибка для неподдерживаемой архитектуры")
	}
}

func TestFindAsset(t *testing.T) {
	assets := []types.Asset{
		{Name: "sing-box-v1.10.0-linux-arm64.tar.gz"},
		{Name: "sing-box-v1.10.0-linux-amd64.tar.gz"},
		{Name: "sing-box-v1.10.0-linux-mipsle-softfloat.tar.gz"},
	}

	tests := []struct {
		pattern  string
		wantName string
		wantNil  bool
	}{
		{"linux-arm64.tar.gz", "sing-box-v1.10.0-linux-arm64.tar.gz", false},
		{"linux-mipsle-softfloat.tar.gz", "sing-box-v1.10.0-linux-mipsle-softfloat.tar.gz", false},
		{"linux-arm64.tar.gz", "sing-box-v1.10.0-linux-arm64.tar.gz", false},
		{"linux-riscv64.tar.gz", "", true},
	}

	for _, tt := range tests {
		got := findAsset(assets, tt.pattern)
		if tt.wantNil && got != nil {
			t.Errorf("findAsset(%q): ожидался nil, получили %+v", tt.pattern, got)
		}
		if !tt.wantNil {
			if got == nil {
				t.Errorf("findAsset(%q): ожидался asset, получили nil", tt.pattern)
			} else if got.Name != tt.wantName {
				t.Errorf("findAsset(%q): Name = %q, ожидалось %q", tt.pattern, got.Name, tt.wantName)
			}
		}
	}
}

func TestReadETag_MissingFile(t *testing.T) {
	if etag := readETag(filepath.Join(t.TempDir(), "nonexistent.etag")); etag != "" {
		t.Errorf("ожидалась пустая строка для отсутствующего etag-файла, получили %q", etag)
	}
}

package singbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// fakeRelease формирует /releases/latest JSON с asset + сопутствующим .sha256.
// VerifySHA для sing-box обязателен (защита от MITM на GitHub CDN), поэтому
// тестовый сервер должен отдавать оба файла.
func fakeRelease(assetName, downloadURL string, size int64, shaURL string) []byte {
	rel := types.Release{
		TagName: "v1.10.0",
		Assets: []types.Asset{
			{Name: assetName, BrowserDownloadURL: downloadURL, Size: size},
			{Name: assetName + ".sha256", BrowserDownloadURL: shaURL, Size: 100},
		},
	}
	data, _ := json.Marshal(rel)
	return data
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestDownload_Success(t *testing.T) {
	content := []byte("fake-tarball-content")
	sumLine := sha256Hex(content) + "  sing-box-v1.10.0-linux-arm64.tar.gz\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			assetURL := "http://" + r.Host + "/download/sing-box-v1.10.0-linux-arm64.tar.gz"
			shaURL := "http://" + r.Host + "/download/sing-box-v1.10.0-linux-arm64.tar.gz.sha256"
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(fakeRelease("sing-box-v1.10.0-linux-arm64.tar.gz", assetURL, int64(len(content)), shaURL))
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = w.Write([]byte(sumLine))
		default:
			w.Header().Set("ETag", `"abc123"`)
			_, _ = w.Write(content)
		}
	}))
	defer srv.Close()

	orig := ghrelease.APIBaseURL
	ghrelease.APIBaseURL = srv.URL
	defer func() { ghrelease.APIBaseURL = orig }()

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
	content := []byte("data")
	sumLine := sha256Hex(content) + "  sing-box-v1.10.0-linux-arm64.tar.gz\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			assetURL := "http://" + r.Host + "/download/sing-box-v1.10.0-linux-arm64.tar.gz"
			shaURL := "http://" + r.Host + "/download/sing-box-v1.10.0-linux-arm64.tar.gz.sha256"
			_, _ = w.Write(fakeRelease("sing-box-v1.10.0-linux-arm64.tar.gz", assetURL, int64(len(content)), shaURL))
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = w.Write([]byte(sumLine))
		default:
			if r.Header.Get("If-None-Match") == `"etag-v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"etag-v1"`)
			_, _ = w.Write(content)
		}
	}))
	defer srv.Close()

	orig := ghrelease.APIBaseURL
	ghrelease.APIBaseURL = srv.URL
	defer func() { ghrelease.APIBaseURL = orig }()

	dir := t.TempDir()

	if _, err := Download(context.Background(), types.ArchARM64, dir); err != nil {
		t.Fatalf("первый Download: %v", err)
	}

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
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()

	orig := ghrelease.APIBaseURL
	ghrelease.APIBaseURL = srv.URL
	defer func() { ghrelease.APIBaseURL = orig }()

	_, err := Download(context.Background(), types.ArchARM64, t.TempDir())
	if err == nil {
		t.Fatal("ожидалась ошибка при HTTP 500")
	}
}

// TestDownload_TruncatedTarballRejected — manifest заявляет Size=100, но
// сервер отдаёт 50 байт (порванный канал/MITM-обрыв). Fetch должен отвергнуть.
func TestDownload_TruncatedTarballRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			assetURL := "http://" + r.Host + "/download/sing-box-v1.10.0-linux-arm64.tar.gz"
			shaURL := "http://" + r.Host + "/download/sing-box-v1.10.0-linux-arm64.tar.gz.sha256"
			_, _ = w.Write(fakeRelease("sing-box-v1.10.0-linux-arm64.tar.gz", assetURL, 100, shaURL))
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000  x\n"))
		default:
			// сервер отдаёт ТОЛЬКО 10 байт вместо 100
			w.Header().Set("ETag", `"truncated"`)
			_, _ = w.Write([]byte("truncated!"))
		}
	}))
	defer srv.Close()

	orig := ghrelease.APIBaseURL
	ghrelease.APIBaseURL = srv.URL
	defer func() { ghrelease.APIBaseURL = orig }()

	_, err := Download(context.Background(), types.ArchARM64, t.TempDir())
	if err == nil {
		t.Fatal("ожидалась ошибка для truncated tarball")
	}
	if !strings.Contains(err.Error(), "размер") {
		t.Errorf("сообщение = %q, ожидалось содержащее 'размер'", err.Error())
	}
}

func TestDownload_UnsupportedArch(t *testing.T) {
	_, err := Download(context.Background(), "riscv64", t.TempDir())
	if err == nil {
		t.Fatal("ожидалась ошибка для неподдерживаемой архитектуры")
	}
}

// Тест внутренних утилит ghrelease.MatchByContains.
func TestMatchByContains(t *testing.T) {
	assets := []types.Asset{
		{Name: "sing-box-v1.10.0-linux-arm64.tar.gz"},
		{Name: "sing-box-v1.10.0-linux-amd64.tar.gz"},
		{Name: "sing-box-v1.10.0-linux-mipsle-softfloat.tar.gz"},
	}

	tests := []struct {
		pattern  string
		wantName string
	}{
		{"linux-arm64.tar.gz", "sing-box-v1.10.0-linux-arm64.tar.gz"},
		{"linux-mipsle-softfloat.tar.gz", "sing-box-v1.10.0-linux-mipsle-softfloat.tar.gz"},
	}

	for _, tt := range tests {
		fn := ghrelease.MatchByContains(tt.pattern)
		var got *types.Asset
		for i := range assets {
			if fn(assets[i]) {
				got = &assets[i]
				break
			}
		}
		if got == nil {
			t.Errorf("MatchByContains(%q): не найдено", tt.pattern)
			continue
		}
		if got.Name != tt.wantName {
			t.Errorf("MatchByContains(%q): Name = %q, ожидалось %q", tt.pattern, got.Name, tt.wantName)
		}
	}
}

package xray

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func fakeRelease(assetName, downloadURL string, size int64) []byte {
	rel := types.Release{
		TagName: "v1.8.4",
		Assets: []types.Asset{
			{
				Name:               assetName,
				BrowserDownloadURL: downloadURL,
				Size:               size,
			},
		},
	}
	data, _ := json.Marshal(rel)
	return data
}

func TestDownload_Success(t *testing.T) {
	content := []byte("fake-zip-content")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			assetURL := "http://" + r.Host + "/download/Xray-linux-arm64-v8a.zip"
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(fakeRelease("Xray-linux-arm64-v8a.zip", assetURL, int64(len(content))))
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
	if res.Version != "v1.8.4" {
		t.Errorf("Version = %q, ожидалось %q", res.Version, "v1.8.4")
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("файл не создан: %v", err)
	}
}

// TestDownload_UpstreamArches проверяет, что для arm64/arm7/amd64 запрос идёт
// в upstream XTLS/Xray-core и берёт правильный asset.
func TestDownload_UpstreamArches(t *testing.T) {
	cases := []struct {
		arch  types.Arch
		asset string
	}{
		{types.ArchARM64, "Xray-linux-arm64-v8a.zip"},
		{types.ArchARM7, "Xray-linux-arm32-v7a.zip"},
		{types.ArchAMD64, "Xray-linux-64.zip"},
	}
	for _, c := range cases {
		t.Run(string(c.arch), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/releases/latest") {
					rel := types.Release{
						TagName: "v1.8.4",
						Assets: []types.Asset{
							{Name: "Xray-linux-arm64-v8a.zip", BrowserDownloadURL: "http://" + r.Host + "/a", Size: 4},
							{Name: "Xray-linux-arm32-v7a.zip", BrowserDownloadURL: "http://" + r.Host + "/b", Size: 4},
							{Name: "Xray-linux-64.zip", BrowserDownloadURL: "http://" + r.Host + "/e", Size: 4},
						},
					}
					data, _ := json.Marshal(rel)
					_, _ = w.Write(data)
					return
				}
				_, _ = w.Write([]byte("zzzz"))
			}))
			defer srv.Close()

			orig := ghrelease.APIBaseURL
			ghrelease.APIBaseURL = srv.URL
			defer func() { ghrelease.APIBaseURL = orig }()

			dir := t.TempDir()
			res, err := Download(context.Background(), c.arch, dir)
			if err != nil {
				t.Fatalf("Download(%s): %v", c.arch, err)
			}
			if !strings.Contains(res.Path, c.asset) {
				t.Errorf("Path=%q не содержит %q", res.Path, c.asset)
			}
		})
	}
}

// TestDownload_MIPSCustomBuild проверяет, что для mips/mipsle Download идёт
// в releases/tags/xray-mips-<ver> на kittylabassistant/sign-craze, а не в upstream.
func TestDownload_MIPSCustomBuild(t *testing.T) {
	cases := []struct {
		arch  types.Arch
		asset string
	}{
		{types.ArchMIPSLE, "Xray-linux-mips32le.zip"},
		{types.ArchMIPS, "Xray-linux-mips32.zip"},
	}
	for _, c := range cases {
		t.Run(string(c.arch), func(t *testing.T) {
			content := []byte("zzzz")
			// SHA256("zzzz") = 2d6ccd34ad7af363159ed4bbe18c0e43c681f606877d9ffc96b62200720d7291
			wantSum := "2d6ccd34ad7af363159ed4bbe18c0e43c681f606877d9ffc96b62200720d7291"
			expectedTag := "xray-mips-" + XRayMIPSVersion

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/releases/latest"):
					http.Error(w, "wrong endpoint: latest must not be queried for MIPS", http.StatusBadRequest)
					return
				case strings.HasSuffix(r.URL.Path, "/releases/tags/"+expectedTag):
					rel := types.Release{
						TagName: expectedTag,
						Assets: []types.Asset{
							{Name: "Xray-linux-mips32le.zip", BrowserDownloadURL: "http://" + r.Host + "/le.zip", Size: 4},
							{Name: "Xray-linux-mips32le.zip.sha256", BrowserDownloadURL: "http://" + r.Host + "/le.sha256", Size: 64},
							{Name: "Xray-linux-mips32.zip", BrowserDownloadURL: "http://" + r.Host + "/be.zip", Size: 4},
							{Name: "Xray-linux-mips32.zip.sha256", BrowserDownloadURL: "http://" + r.Host + "/be.sha256", Size: 64},
						},
					}
					_ = json.NewEncoder(w).Encode(rel)
				case strings.HasSuffix(r.URL.Path, ".sha256"):
					_, _ = w.Write([]byte(wantSum + "  asset\n"))
				default:
					_, _ = w.Write(content)
				}
			}))
			defer srv.Close()

			orig := ghrelease.APIBaseURL
			ghrelease.APIBaseURL = srv.URL
			defer func() { ghrelease.APIBaseURL = orig }()

			dir := t.TempDir()
			res, err := Download(context.Background(), c.arch, dir)
			if err != nil {
				t.Fatalf("Download(%s): %v", c.arch, err)
			}
			if !strings.Contains(res.Path, c.asset) {
				t.Errorf("Path=%q не содержит %q", res.Path, c.asset)
			}
			if res.Version != expectedTag {
				t.Errorf("Version = %q, ожидался %q", res.Version, expectedTag)
			}
		})
	}
}

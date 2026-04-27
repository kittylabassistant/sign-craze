package dpi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

const releasesPath = "/repos/bol-van/nfqws2-keenetic/releases/latest"

func TestDownload_СкачиваетТарбол(t *testing.T) {
	fakeContent := []byte("fake-nfqws2-tarball")

	mux := http.NewServeMux()
	mux.HandleFunc(releasesPath, func(w http.ResponseWriter, r *http.Request) {
		release := types.Release{
			TagName: "v1.0.0",
			Assets: []types.Asset{
				{
					Name:               "nfqws2-v1.0.0-aarch64.tar.gz",
					BrowserDownloadURL: "http://" + r.Host + "/download/nfqws2.tar.gz",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(release)
	})
	mux.HandleFunc("/download/nfqws2.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"etag-v1"`)
		_, _ = w.Write(fakeContent)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	old := ghrelease.APIBaseURL
	ghrelease.APIBaseURL = srv.URL
	defer func() { ghrelease.APIBaseURL = old }()

	dstDir := t.TempDir()
	res, err := Download(context.Background(), types.ArchARM64, dstDir)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !res.Downloaded {
		t.Error("ожидался Downloaded=true")
	}
	if res.Version != "v1.0.0" {
		t.Errorf("Version = %q, ожидался v1.0.0", res.Version)
	}

	data, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(fakeContent) {
		t.Errorf("содержимое файла не совпадает")
	}
}

func TestDownload_ПропускаетПриСовпаденииETag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc(releasesPath, func(w http.ResponseWriter, r *http.Request) {
		release := types.Release{
			TagName: "v1.0.0",
			Assets: []types.Asset{
				{
					Name:               "nfqws2-v1.0.0-aarch64.tar.gz",
					BrowserDownloadURL: "http://" + r.Host + "/download/nfqws2.tar.gz",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(release)
	})
	mux.HandleFunc("/download/nfqws2.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"etag-v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"etag-v1"`)
		_, _ = w.Write([]byte("content"))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	old := ghrelease.APIBaseURL
	ghrelease.APIBaseURL = srv.URL
	defer func() { ghrelease.APIBaseURL = old }()

	dstDir := t.TempDir()
	dstFile := filepath.Join(dstDir, "nfqws2-v1.0.0-aarch64.tar.gz")

	if err := os.WriteFile(dstFile+".etag", []byte(`"etag-v1"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dstFile, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Download(context.Background(), types.ArchARM64, dstDir)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if res.Downloaded {
		t.Error("ожидался Downloaded=false при совпадении ETag")
	}
}

func TestDownload_НеизвестнаяАрхитектура(t *testing.T) {
	_, err := Download(context.Background(), "riscv64", t.TempDir())
	if err == nil {
		t.Error("ожидалась ошибка для неизвестной архитектуры")
	}
}

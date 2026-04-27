package dpi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestDownload_СкачиваетТарбол(t *testing.T) {
	fakeContent := []byte("fake-nfqws2-tarball")

	// Сервер отдаёт метаданные релиза и файл
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		release := types.Release{
			TagName: "v1.0.0",
			Assets: []types.Asset{
				{
					Name:               "nfqws2-v1.0.0-aarch64.tar.gz",
					BrowserDownloadURL: "http://" + r.Host + "/download/nfqws2.tar.gz",
				},
			},
		}
		json.NewEncoder(w).Encode(release) //nolint:errcheck
	})
	mux.HandleFunc("/download/nfqws2.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"etag-v1"`)
		w.Write(fakeContent) //nolint:errcheck
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	old := nfqws2ReleasesURL
	nfqws2ReleasesURL = srv.URL + "/releases/latest"
	defer func() { nfqws2ReleasesURL = old }()

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
	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		release := types.Release{
			TagName: "v1.0.0",
			Assets: []types.Asset{
				{
					Name:               "nfqws2-v1.0.0-aarch64.tar.gz",
					BrowserDownloadURL: "http://" + r.Host + "/download/nfqws2.tar.gz",
				},
			},
		}
		json.NewEncoder(w).Encode(release) //nolint:errcheck
	})
	mux.HandleFunc("/download/nfqws2.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Header.Get("If-None-Match") == `"etag-v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"etag-v1"`)
		w.Write([]byte("content")) //nolint:errcheck
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	old := nfqws2ReleasesURL
	nfqws2ReleasesURL = srv.URL + "/releases/latest"
	defer func() { nfqws2ReleasesURL = old }()

	dstDir := t.TempDir()
	dstFile := filepath.Join(dstDir, "nfqws2-v1.0.0-aarch64.tar.gz")

	// Сохраняем ETag заранее (имитируем предыдущую загрузку)
	if err := os.WriteFile(dstFile+".etag", []byte(`"etag-v1"`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Создаём файл, чтобы он «существовал»
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
	_ = callCount
}

func TestDownload_НеизвестнаяАрхитектура(t *testing.T) {
	_, err := Download(context.Background(), "riscv64", t.TempDir())
	if err == nil {
		t.Error("ожидалась ошибка для неизвестной архитектуры")
	}
}

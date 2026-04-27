package ghrelease

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestFetch_Success(t *testing.T) {
	content := []byte("payload")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			rel := types.Release{
				TagName: "v1.2.3",
				Assets: []types.Asset{{
					Name:               "tool-linux-arm64.tar.gz",
					BrowserDownloadURL: "http://" + r.Host + "/dl/tool.tar.gz",
				}},
			}
			_ = json.NewEncoder(w).Encode(rel)
		default:
			w.Header().Set("ETag", `"abc"`)
			_, _ = w.Write(content)
		}
	}))
	defer srv.Close()

	old := APIBaseURL
	APIBaseURL = srv.URL
	defer func() { APIBaseURL = old }()

	res, err := New().Fetch(context.Background(), FetchOptions{
		Owner:      "owner",
		Repo:       "repo",
		AssetMatch: MatchByContains("arm64"),
		DstDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !res.Downloaded {
		t.Error("ожидался Downloaded=true")
	}
	if res.Version != "v1.2.3" {
		t.Errorf("Version = %q, ожидалось v1.2.3", res.Version)
	}
	got, _ := os.ReadFile(res.Path)
	if string(got) != string(content) {
		t.Errorf("содержимое = %q, ожидалось %q", got, content)
	}
}

func TestFetch_AssetNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rel := types.Release{
			TagName: "v1.0",
			Assets:  []types.Asset{{Name: "other.tar.gz"}},
		}
		_ = json.NewEncoder(w).Encode(rel)
	}))
	defer srv.Close()

	old := APIBaseURL
	APIBaseURL = srv.URL
	defer func() { APIBaseURL = old }()

	_, err := New().Fetch(context.Background(), FetchOptions{
		Owner:      "x",
		Repo:       "y",
		AssetMatch: MatchByContains("missing"),
		DstDir:     t.TempDir(),
	})
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestFetch_VerifySHA_Match(t *testing.T) {
	content := []byte("hello")
	// sha256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	wantSum := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			rel := types.Release{
				TagName: "v1",
				Assets: []types.Asset{
					{Name: "x.bin", BrowserDownloadURL: "http://" + r.Host + "/x.bin"},
					{Name: "x.bin.sha256", BrowserDownloadURL: "http://" + r.Host + "/x.bin.sha256"},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = w.Write([]byte(wantSum + "  x.bin\n"))
		default:
			_, _ = w.Write(content)
		}
	}))
	defer srv.Close()

	old := APIBaseURL
	APIBaseURL = srv.URL
	defer func() { APIBaseURL = old }()

	_, err := New().Fetch(context.Background(), FetchOptions{
		Owner:      "x",
		Repo:       "y",
		AssetMatch: func(a types.Asset) bool { return a.Name == "x.bin" },
		DstDir:     t.TempDir(),
		VerifySHA:  true,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
}

func TestFetch_VerifySHA_Mismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			rel := types.Release{
				TagName: "v1",
				Assets: []types.Asset{
					{Name: "x.bin", BrowserDownloadURL: "http://" + r.Host + "/x.bin"},
					{Name: "x.bin.sha256", BrowserDownloadURL: "http://" + r.Host + "/x.bin.sha256"},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = w.Write([]byte("deadbeef  x.bin\n"))
		default:
			_, _ = w.Write([]byte("payload"))
		}
	}))
	defer srv.Close()

	old := APIBaseURL
	APIBaseURL = srv.URL
	defer func() { APIBaseURL = old }()

	_, err := New().Fetch(context.Background(), FetchOptions{
		Owner:      "x",
		Repo:       "y",
		AssetMatch: func(a types.Asset) bool { return a.Name == "x.bin" },
		DstDir:     t.TempDir(),
		VerifySHA:  true,
	})
	if err == nil {
		t.Fatal("ожидалась ошибка несовпадения SHA256")
	}
}

func TestMatchByContains_Helper(t *testing.T) {
	fn := MatchByContains("arm64")
	if !fn(types.Asset{Name: "tool-arm64.tar.gz"}) {
		t.Error("должен матчить")
	}
	if fn(types.Asset{Name: "tool-amd64.tar.gz"}) {
		t.Error("не должен матчить")
	}
}

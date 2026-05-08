package ghrelease

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

func TestFetch_ByTag(t *testing.T) {
	content := []byte("payload")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			http.Error(w, "latest must not be queried when Tag set", http.StatusBadRequest)
			return
		case strings.HasSuffix(r.URL.Path, "/releases/tags/v9.9.9"):
			rel := types.Release{
				TagName: "v9.9.9",
				Assets: []types.Asset{{
					Name:               "tool-linux-arm64.tar.gz",
					BrowserDownloadURL: "http://" + r.Host + "/dl/tool.tar.gz",
				}},
			}
			_ = json.NewEncoder(w).Encode(rel)
		default:
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
		Tag:        "v9.9.9",
		AssetMatch: MatchByContains("arm64"),
		DstDir:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Version != "v9.9.9" {
		t.Errorf("Version = %q, ожидалось v9.9.9", res.Version)
	}
}

// TestDownloadAsset_BodyIdleTimeout_FailsOverToNextMirror — mirror A
// возвращает headers + Content-Length, но body не отдаёт (висит дольше
// bodyIdleTimeout). Watchdog должен закрыть body, downloadAsset перейти
// на mirror B и успешно скачать оттуда. Без этого механизма stall'нувшее
// зеркало (наблюдение 2026-05-08: ghfast.top отдал headers и 0 байт
// тела за 5 минут) валит всю загрузку, хотя следующее работоспособно.
func TestDownloadAsset_BodyIdleTimeout_FailsOverToNextMirror(t *testing.T) {
	// Уменьшаем idle timeout чтобы тест шёл миллисекундами, не 30s.
	oldTimeout := bodyIdleTimeout
	bodyIdleTimeout = 200 * time.Millisecond
	defer func() { bodyIdleTimeout = oldTimeout }()

	content := []byte("real-payload-from-mirror-B")
	stallEnter := make(chan struct{}, 1)

	stallSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1024")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		stallEnter <- struct{}{}
		// Ждём пока клиент не закроет соединение (idle watchdog → Body.Close
		// → request ctx cancel) — либо до окончания теста.
		<-r.Context().Done()
	}))
	defer stallSrv.Close()

	goodSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(content)
	}))
	defer goodSrv.Close()

	// Release-meta + sha-meta отдаёт отдельный сервер, чтобы Mirrors
	// применялись только к asset-загрузке.
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := types.Release{
			TagName: "v1.0.0",
			Assets: []types.Asset{{
				Name:               "asset.bin",
				BrowserDownloadURL: "https://example.invalid/asset.bin",
				Size:               int64(len(content)),
			}},
		}
		_ = json.NewEncoder(w).Encode(rel)
		_ = r
	}))
	defer apiSrv.Close()

	old := APIBaseURL
	APIBaseURL = apiSrv.URL
	defer func() { APIBaseURL = old }()

	// Mirror rewriters: asset URL (example.invalid) переписывается на тестовые
	// серверы, всё остальное (release-meta) проходит direct, чтобы fetchReleaseMeta
	// не попало в stall-сервер.
	d := New()
	d.Mirrors = []URLRewriter{
		func(u string) string {
			if strings.Contains(u, "example.invalid") {
				return stallSrv.URL + "/asset.bin"
			}
			return u
		},
		func(u string) string {
			if strings.Contains(u, "example.invalid") {
				return goodSrv.URL + "/asset.bin"
			}
			return "" // meta уже прошла через первый mirror
		},
	}

	dst := t.TempDir()
	res, err := d.Fetch(context.Background(), FetchOptions{
		Owner:      "owner",
		Repo:       "repo",
		AssetMatch: MatchByContains("asset"),
		DstDir:     dst,
	})
	if err != nil {
		t.Fatalf("Fetch вернул ошибку, ожидался успех с fallback: %v", err)
	}

	// Удостоверимся, что stall-сервер действительно был вызван (не пропущен).
	select {
	case <-stallEnter:
	default:
		t.Error("stall mirror не был вызван — fallback сработал слишком рано")
	}

	got, readErr := os.ReadFile(res.Path)
	if readErr != nil {
		t.Fatalf("чтение результата: %v", readErr)
	}
	if string(got) != string(content) {
		t.Errorf("содержимое = %q, ожидалось %q (с good mirror)", got, content)
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

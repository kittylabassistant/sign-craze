package ghrelease

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// TestFetch_AssetMatchers_FirstMatchWins — релиз содержит base+musl, причём base
// идёт ПЕРВЫМ в списке assets. Приоритетный matcher [musl, base] обязан выбрать
// musl (первый matcher), а не первый в API-ответе. Это инвариант приоритета.
func TestFetch_AssetMatchers_FirstMatchWins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			rel := types.Release{
				TagName: "v1",
				Assets: []types.Asset{
					{Name: "tool-linux-arm64.tar.gz", BrowserDownloadURL: "http://" + r.Host + "/dl/base"},
					{Name: "tool-linux-arm64-musl.tar.gz", BrowserDownloadURL: "http://" + r.Host + "/dl/musl"},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
		default:
			_, _ = w.Write([]byte("payload"))
		}
	}))
	defer srv.Close()

	old := APIBaseURL
	APIBaseURL = srv.URL
	defer func() { APIBaseURL = old }()

	res, err := New().Fetch(context.Background(), FetchOptions{
		Owner: "o", Repo: "r",
		AssetMatchers: []func(types.Asset) bool{
			MatchByContains("linux-arm64-musl.tar.gz"),
			MatchByContains("linux-arm64.tar.gz"),
		},
		DstDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(res.Path, "arm64-musl.tar.gz") {
		t.Errorf("выбран %q, ожидался musl-вариант (приоритет matcher'а)", res.Path)
	}
}

// TestFetch_AssetMatchers_FallbackToSecond — релиз содержит ТОЛЬКО базовый ассет
// (musl отсутствует, как у sing-box для mips). Первый matcher не находит → fallback
// на второй. Download выживает.
func TestFetch_AssetMatchers_FallbackToSecond(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			rel := types.Release{
				TagName: "v1",
				Assets:  []types.Asset{{Name: "tool-linux-arm64.tar.gz", BrowserDownloadURL: "http://" + r.Host + "/dl/base"}},
			}
			_ = json.NewEncoder(w).Encode(rel)
		default:
			_, _ = w.Write([]byte("payload"))
		}
	}))
	defer srv.Close()

	old := APIBaseURL
	APIBaseURL = srv.URL
	defer func() { APIBaseURL = old }()

	res, err := New().Fetch(context.Background(), FetchOptions{
		Owner: "o", Repo: "r",
		AssetMatchers: []func(types.Asset) bool{
			MatchByContains("linux-arm64-musl.tar.gz"),
			MatchByContains("linux-arm64.tar.gz"),
		},
		DstDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if strings.Contains(res.Path, "musl") || !strings.Contains(res.Path, "arm64.tar.gz") {
		t.Errorf("выбран %q, ожидался базовый ассет (fallback)", res.Path)
	}
}

// TestFetch_AssetMatchers_NoneMatch — ни один matcher не находит asset → ошибка.
func TestFetch_AssetMatchers_NoneMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rel := types.Release{
			TagName: "v1",
			Assets:  []types.Asset{{Name: "tool-linux-amd64.tar.gz"}},
		}
		_ = json.NewEncoder(w).Encode(rel)
	}))
	defer srv.Close()

	old := APIBaseURL
	APIBaseURL = srv.URL
	defer func() { APIBaseURL = old }()

	_, err := New().Fetch(context.Background(), FetchOptions{
		Owner: "o", Repo: "r",
		AssetMatchers: []func(types.Asset) bool{
			MatchByContains("linux-arm64-musl.tar.gz"),
			MatchByContains("linux-arm64.tar.gz"),
		},
		DstDir: t.TempDir(),
	})
	if err == nil {
		t.Fatal("ожидалась ошибка: ни один matcher не нашёл asset")
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
	d.BodyIdleTimeout = 200 * time.Millisecond
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

// TestTryMirrorDownload_SHA256CorrectViaTeeReader — R13: после замены
// ручного atomic-write блока (CreateTemp+MultiWriter) на
// atomicfs.WriteFileAtomicFromReader + io.TeeReader хеш должен по-прежнему
// совпадать с sha256 контента, в том числе когда контент больше внутреннего
// copy-buffer'а atomicfs (8KB) — то есть Read вызывается несколько раз и
// TeeReader копит хеш по кускам, а не за один присест.
func TestTryMirrorDownload_SHA256CorrectViaTeeReader(t *testing.T) {
	content := bytes.Repeat([]byte("sign-craze-r13-teereader-check-"), 4096) // ~128KB
	wantSum := sha256.Sum256(content)
	wantHex := hex.EncodeToString(wantSum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "asset.bin")
	d := New()
	downloaded, _, sum, retriable, err := d.tryMirrorDownload(context.Background(), srv.URL+"/asset.bin", dst, "", "test-host")
	if err != nil {
		t.Fatalf("tryMirrorDownload: %v (retriable=%v)", err, retriable)
	}
	if !downloaded {
		t.Fatal("ожидался downloaded=true")
	}
	if sum != wantHex {
		t.Errorf("sha256 = %s, ожидалось %s", sum, wantHex)
	}

	got, readErr := os.ReadFile(dst)
	if readErr != nil {
		t.Fatalf("чтение результата: %v", readErr)
	}
	if !bytes.Equal(got, content) {
		t.Error("содержимое скачанного файла не совпадает с оригиналом")
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

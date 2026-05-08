package ghrelease

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGHProxyRewriter(t *testing.T) {
	cases := map[string]string{
		"https://github.com/foo/bar/releases/download/v1/asset.gz":  "https://gh-proxy.com/https://github.com/foo/bar/releases/download/v1/asset.gz",
		"https://api.github.com/repos/foo/bar/releases/latest":      "https://gh-proxy.com/https://api.github.com/repos/foo/bar/releases/latest",
		"https://raw.githubusercontent.com/foo/bar/main/README.md":  "https://gh-proxy.com/https://raw.githubusercontent.com/foo/bar/main/README.md",
		"https://objects.githubusercontent.com/foo/bar/release.bin": "https://gh-proxy.com/https://objects.githubusercontent.com/foo/bar/release.bin",
		"https://example.com/not-github":                            "",
		"http://github.com/insecure":                                "",
	}
	for in, want := range cases {
		got := ghProxyRewriter(in)
		if got != want {
			t.Errorf("ghProxyRewriter(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJsdelivrRewriter(t *testing.T) {
	cases := map[string]string{
		"https://raw.githubusercontent.com/foo/bar/main/path/to/file.txt": "https://cdn.jsdelivr.net/gh/foo/bar@main/path/to/file.txt",
		"https://raw.githubusercontent.com/owner/repo/v1.0/sub/file.bin":  "https://cdn.jsdelivr.net/gh/owner/repo@v1.0/sub/file.bin",
		"https://github.com/foo/bar/releases/download/v1/x.gz":            "", // jsdelivr не хостит releases
		"https://api.github.com/repos/foo/bar":                            "",
	}
	for in, want := range cases {
		got := jsdelivrRewriter(in)
		if got != want {
			t.Errorf("jsdelivrRewriter(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDoWithFallback_DirectFails_FallbackToMirror — первый mirror (direct)
// возвращает 503, второй (mirror-stub) возвращает 200. Должны получить
// успешный resp от второго mirror.
func TestDoWithFallback_DirectFails_FallbackToMirror(t *testing.T) {
	var directCalls, mirrorCalls atomic.Int32

	directSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		directCalls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer directSrv.Close()

	mirrorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mirrorCalls.Add(1)
		_, _ = w.Write([]byte("ok"))
	}))
	defer mirrorSrv.Close()

	d := New()
	d.Mirrors = []URLRewriter{
		func(_ string) string { return directSrv.URL + "/asset" },
		func(_ string) string { return mirrorSrv.URL + "/asset" },
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://github.com/foo/bar", nil)
	resp, err := d.doWithFallback(req)
	if err != nil {
		t.Fatalf("doWithFallback: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if directCalls.Load() != 1 {
		t.Errorf("direct calls = %d, want 1", directCalls.Load())
	}
	if mirrorCalls.Load() != 1 {
		t.Errorf("mirror calls = %d, want 1", mirrorCalls.Load())
	}
}

// TestDoWithFallback_AllMirrorsFail — все mirrors вернули 5xx → итоговая ошибка.
func TestDoWithFallback_AllMirrorsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	d := New()
	d.Mirrors = []URLRewriter{
		func(_ string) string { return srv.URL + "/a" },
		func(_ string) string { return srv.URL + "/b" },
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://github.com/foo/bar", nil)
	resp, err := d.doWithFallback(req)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("ожидалась ошибка когда все зеркала вернули 5xx")
	}
}

// TestDoWithFallback_PreservesHeaders — User-Agent и If-None-Match
// должны сохраняться при переходе на mirror.
func TestDoWithFallback_PreservesHeaders(t *testing.T) {
	var seenUA, seenETag string

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		seenETag = r.Header.Get("If-None-Match")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer okSrv.Close()

	d := New()
	d.Mirrors = []URLRewriter{
		func(_ string) string { return failSrv.URL },
		func(_ string) string { return okSrv.URL },
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://github.com/x", nil)
	req.Header.Set("User-Agent", "curl/8.12.1")
	req.Header.Set("If-None-Match", `"abc"`)

	resp, err := d.doWithFallback(req)
	if err != nil {
		t.Fatalf("doWithFallback: %v", err)
	}
	resp.Body.Close()

	if seenUA != "curl/8.12.1" {
		t.Errorf("User-Agent на mirror = %q, want curl/8.12.1", seenUA)
	}
	if seenETag != `"abc"` {
		t.Errorf("If-None-Match на mirror = %q, want \"abc\"", seenETag)
	}
}

// TestDoWithFallback_SkipsEmptyRewriter — пустой результат rewriter
// (mirror не поддерживает данный URL) пропускается без ошибки.
func TestDoWithFallback_SkipsEmptyRewriter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	d := New()
	d.Mirrors = []URLRewriter{
		func(_ string) string { return "" }, // skip
		func(_ string) string { return "" }, // skip
		func(_ string) string { return srv.URL },
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://github.com/x", nil)
	resp, err := d.doWithFallback(req)
	if err != nil {
		t.Fatalf("doWithFallback: %v", err)
	}
	resp.Body.Close()

	if calls.Load() != 1 {
		t.Errorf("server calls = %d, want 1", calls.Load())
	}
}

func TestHostOf(t *testing.T) {
	cases := map[string]string{
		"https://gh-proxy.com/https://github.com/foo": "gh-proxy.com",
		"https://api.github.com/repos/x/y":            "api.github.com",
		"https://cdn.jsdelivr.net/gh/foo/bar@main/x":  "cdn.jsdelivr.net",
		"https://example.com":                         "example.com",
	}
	for in, want := range cases {
		if got := hostOf(in); got != want {
			t.Errorf("hostOf(%q) = %q, want %q", in, got, want)
		}
	}
	_ = strings.Builder{} // ensure strings import is used in production file
}

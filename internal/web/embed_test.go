package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestEmbedCacheMiddleware_Assets — /assets/* должен получать immutable Cache-Control.
func TestEmbedCacheMiddleware_Assets(t *testing.T) {
	handler := embedCacheMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/assets/foo.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("Cache-Control")
	want := "public, max-age=31536000, immutable"
	if got != want {
		t.Errorf("/assets/foo.js Cache-Control = %q, ожидалось %q", got, want)
	}
}

// TestEmbedCacheMiddleware_Nuxt — /_nuxt/* должен получать immutable Cache-Control.
func TestEmbedCacheMiddleware_Nuxt(t *testing.T) {
	handler := embedCacheMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/_nuxt/app.bundle.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("Cache-Control")
	want := "public, max-age=31536000, immutable"
	if got != want {
		t.Errorf("/_nuxt/ Cache-Control = %q, ожидалось %q", got, want)
	}
}

// TestEmbedCacheMiddleware_Index — / и /index.html должны получать no-cache.
func TestEmbedCacheMiddleware_Index(t *testing.T) {
	handler := embedCacheMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, urlPath := range []string{"/", "/index.html"} {
		req := httptest.NewRequest(http.MethodGet, urlPath, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		got := rec.Header().Get("Cache-Control")
		want := "no-cache"
		if got != want {
			t.Errorf("%s Cache-Control = %q, ожидалось %q", urlPath, got, want)
		}
	}
}

// TestEmbedCacheMiddleware_Other — прочие пути без Cache-Control.
func TestEmbedCacheMiddleware_Other(t *testing.T) {
	handler := embedCacheMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Errorf("/api/status Cache-Control = %q, ожидалось пусто", got)
	}
}

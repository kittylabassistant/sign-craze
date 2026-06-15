package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// embedCacheMiddleware ставит Cache-Control заголовки для статических ресурсов:
//   - /assets/* и /_nuxt/* → "public, max-age=31536000, immutable" (хешированные бандлы)
//   - /index.html (и SPA-fallback /) → "no-cache" (точка входа, всегда проверять)
//
// Прочие пути — без Cache-Control (браузерный default).
func embedCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/assets/") || strings.HasPrefix(p, "/_nuxt/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case p == "/" || p == "/index.html":
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

// assets содержит встроенный Zashboard (git submodule) и Routing UI.
// Для инициализации submodule: git submodule update --init internal/web/assets/zashboard
//
// Директорный паттерн "all:assets" терпим к отсутствию submodule: если
// assets/zashboard не инициализирован (как в golden-check CI без
// submodules:recursive), embed просто не включает его файлы — пакет всё равно
// компилируется. Явное перечисление файлов (для trim шрифтов) ломало бы эти
// джобы, поэтому trim вынесен на уровень submodule-снапшота (отдельная задача).
//
//go:embed all:assets
var assets embed.FS

// zashboardFS возвращает файловую систему с Zashboard.
func zashboardFS() fs.FS {
	sub, err := fs.Sub(assets, "assets/zashboard")
	if err != nil {
		// assets/zashboard не инициализирован — возвращаем пустую FS
		return assets
	}
	return sub
}

// spaHandler раздаёт статические файлы из embed.FS.
// Если файл не найден — отдаёт index.html (SPA routing).
type spaHandler struct {
	fs   fs.FS
	file http.Handler
}

func newSPAHandler() http.Handler {
	fsys := zashboardFS()
	spa := &spaHandler{
		fs:   fsys,
		file: http.FileServer(http.FS(fsys)),
	}
	return embedCacheMiddleware(spa)
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Проверяем, существует ли запрошенный файл в embed.FS
	urlPath := r.URL.Path
	if urlPath == "" || urlPath == "/" {
		urlPath = "index.html"
	}
	if urlPath[0] == '/' {
		urlPath = urlPath[1:]
	}

	if _, err := fs.Stat(h.fs, urlPath); err == nil {
		h.file.ServeHTTP(w, r)
		return
	}

	// SPA fallback: отдаём index.html ТОЛЬКО для путей, похожих на маршруты приложения
	// (без файлового расширения или с .html). Запросы к /etc/passwd, /admin.creds,
	// /favicon.ico на несуществующее → 404, чтобы не маскировать misconfiguration.
	if !looksLikeSPARoute(urlPath) {
		http.NotFound(w, r)
		return
	}

	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	h.file.ServeHTTP(w, r2)
}

// looksLikeSPARoute возвращает true, если путь похож на client-side маршрут SPA
// (без файлового расширения или с .html). Любые расширения (.js, .css, .png, ...)
// должны существовать в FS — иначе это 404, а не fallback.
func looksLikeSPARoute(p string) bool {
	if p == "" || p == "index.html" {
		return true
	}
	if strings.ContainsAny(p, "\\") {
		return false
	}
	ext := path.Ext(p)
	if ext == "" || ext == ".html" {
		return true
	}
	return false
}

package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// assets содержит встроенный Zashboard (git submodule).
// Для инициализации submodule: git submodule update --init internal/web/assets/zashboard
//
//go:embed assets
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
	return &spaHandler{
		fs:   fsys,
		file: http.FileServer(http.FS(fsys)),
	}
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

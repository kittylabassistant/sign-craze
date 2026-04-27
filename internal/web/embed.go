package web

import (
	"embed"
	"io/fs"
	"net/http"
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
	path := r.URL.Path
	if path == "" || path == "/" {
		path = "index.html"
	}
	if path[0] == '/' {
		path = path[1:]
	}

	if _, err := fs.Stat(h.fs, path); err == nil {
		h.file.ServeHTTP(w, r)
		return
	}

	// Файл не найден → SPA fallback: отдаём index.html
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	h.file.ServeHTTP(w, r2)
}

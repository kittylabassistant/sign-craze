package web

import (
	"embed"
	"io/fs"
)

// routingUIRaw — встроенные файлы SPA-редактора routing (Preact + htm, offline).
//
//go:embed assets/routingui
var routingUIRaw embed.FS

// routingUIFS возвращает sub-FS с корнем в assets/routingui (без префикса пути).
func routingUIFS() fs.FS {
	sub, err := fs.Sub(routingUIRaw, "assets/routingui")
	if err != nil {
		panic(err)
	}
	return sub
}

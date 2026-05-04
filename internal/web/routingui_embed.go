package web

import (
	"embed"
	"io/fs"
)

// routingUIRaw — встроенные файлы SPA-редактора routing (Preact + htm, offline).
//
// Префикс all: для симметрии с zashboard-embed: на случай добавления каталогов
// вида _vendor/ или .well-known/ в будущем — embed не молча выкидывает их.
//
//go:embed all:assets/routingui
var routingUIRaw embed.FS

// routingUIFS возвращает sub-FS с корнем в assets/routingui (без префикса пути).
func routingUIFS() fs.FS {
	sub, err := fs.Sub(routingUIRaw, "assets/routingui")
	if err != nil {
		panic(err)
	}
	return sub
}

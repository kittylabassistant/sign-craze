// Package backup создаёт и восстанавливает tar.gz-архивы директорий конфигов.
//
// Используется командами `--backup`, `--restore`, `--config-backup` и
// `--config-restore`. Реализация на чистом Go (archive/tar + compress/gzip),
// без вызовов внешних утилит.
package backup

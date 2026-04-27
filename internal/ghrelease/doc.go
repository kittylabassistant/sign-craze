// Package ghrelease — общий загрузчик релизов GitHub с кэшированием по ETag.
//
// Используется внутренними пакетами singbox, dpi и selfupdate для получения
// бинарей с GitHub Releases. Поддерживает If-None-Match (HTTP 304), SHA256-
// верификацию (опционально через .sha256 asset) и атомарную запись в DstDir.
package ghrelease

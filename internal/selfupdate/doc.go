// Package selfupdate выполняет обновление бинаря sign-craze из GitHub Releases.
//
// Логика: ghrelease.Downloader забирает asset для текущей архитектуры,
// SHA256 проверяется через парный .sha256 asset, бинарь атомарно заменяется
// через atomicfs.BackupAndReplace. Запущенный сервис не перезапускается.
package selfupdate

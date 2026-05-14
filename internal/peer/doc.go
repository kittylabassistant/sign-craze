// Package peer управляет supervised-процессами рядом с ядром sign-craze.
//
// Supervised peer — внешний клиентский процесс, который sign-craze запускает
// перед стартом ядра (sing-box/xray/mihomo) и останавливает после. Ядро
// обращается к peer через локальный SOCKS5 на 127.0.0.1:<LocalPort>.
//
// Первый и единственный peer на v0.X.Y — mieru (ADR-0020).
//
// Архитектура:
//
//	internal/peer/portalloc.go  — выделение MieruLocalPort
//	internal/peer/mieru_config.go — генерация client.conf.json
//	internal/peer/mieru_peer.go  — обёртка над service.Lifecycle
//
// Lifecycle полностью делегирован internal/service.Lifecycle (PID-файл,
// SIGTERM grace, stabilization-check). Watchdog не реализуется отдельным
// демоном: при `--start` peer стартует один раз; при крахе пользователь
// видит ошибку через `sign-craze --status` и делает `--restart`.
package peer

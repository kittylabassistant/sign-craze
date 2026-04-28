// Package diag — самодиагностика sign-craze.
//
// Запускает набор проверок (бинарь sing-box, конфиг, iptables/ipset,
// сервис, geo-файлы, блокировка) и возвращает результат с PASS/WARN/FAIL.
// Используется командой `sign-craze --diag`.
package diag

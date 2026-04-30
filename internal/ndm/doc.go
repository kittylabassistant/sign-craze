// Package ndm — клиент Keenetic RCI (Remote Control Interface).
//
// RCI слушает на http://127.0.0.1:79/rci/ без аутентификации.
// Используется для:
//   - создания/чтения/удаления IP Policy (mode=policy);
//   - сохранения running-config в startup-config (configuration save).
//
// Источник схемы: эмпирическое исследование на живом Keenetic Ultra
// KeeneticOS 5.0.4 (см. ~/Документы/rci-dump/).
package ndm

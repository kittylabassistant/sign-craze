// Package core определяет абстракцию прокси-ядра (sing-box, xray, mihomo).
//
// Sign-craze управляет несколькими прокси-ядрами через единый интерфейс Core.
// Конкретные ядра живут в подпакетах (internal/core/singbox/), реализуют Core
// и регистрируют себя в registry через init().
//
// Диспетчер вызывающего кода (cmd, cli) не должен импортировать конкретные
// ядра напрямую — берёт Core по имени из state через core.Get(state.Core).
package core

package singbox

import (
	"github.com/kittylabassistant/sign-craze/internal/firewall"
)

// probeTProxyKernel определяет доступность xt_TPROXY на ядре.
// Возвращает true, если модуль загружен или .ko-файл доступен для insmod.
//
// При true — Render() выставляет inbound type "tproxy" с listen "::" (TCP+UDP).
// При false — fallback "redirect" с listen "0.0.0.0" (TCP-only) для kernel'ов
// без xt_TPROXY (Keenetic 4.9 NDM stock).
//
// Вынесен из cli/deps.go:configParamsFromState чтобы singbox.coreImpl.RenderConfig
// мог самостоятельно сделать probe без CLI-слоя. Импорт firewall безопасен:
// firewall не зависит от singbox (нет cycle).
func probeTProxyKernel() bool {
	if firewall.IsModuleLoaded("xt_TPROXY") {
		return true
	}
	// .ko-файл существует → модуль loadable (insmod сделает applier при apply).
	return firewall.FindKernelModule("xt_TPROXY") != ""
}

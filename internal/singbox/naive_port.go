package singbox

import (
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// DefaultNaiveListenPort — default локальный SOCKS5 порт для bridge
// sing-box → naive daemon. MVP: фикс 18443. Множественные naive outbounds
// пока не поддерживаются (см. state.Validate guard).
const DefaultNaiveListenPort = 18443

// AllocateNaiveListenPort возвращает локальный порт для naive bridge.
// MVP: если у первого naive outbound уже задан NaiveListenPort — используем
// его (для предсказуемости после рестарта), иначе DefaultNaiveListenPort.
// Будущая Phase 2: dynamic port allocator при нескольких naive outbounds.
func AllocateNaiveListenPort(outbounds []types.Outbound) int {
	for _, o := range outbounds {
		if o.Protocol != types.ProtocolNaive || o.Proto == nil {
			continue
		}
		if o.Proto.NaiveListenPort != 0 {
			return o.Proto.NaiveListenPort
		}
	}
	return DefaultNaiveListenPort
}

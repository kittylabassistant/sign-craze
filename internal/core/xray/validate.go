package xray

import (
	"fmt"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// Validate проверяет, что xray поддерживает данное canonical-описание.
// Возвращает ошибку с подсказкой про другое ядро, если протокол не поддерживается.
func Validate(c types.Canonical) error {
	switch c.Protocol {
	case types.ProtocolTUIC:
		return fmt.Errorf("xray не поддерживает TUIC v5, используйте --core sing-box или --core mihomo")
	case types.ProtocolWireGuard:
		return fmt.Errorf("xray не поддерживает WireGuard как outbound, используйте --core sing-box")
	case types.ProtocolHysteria2:
		return fmt.Errorf("xray не поддерживает Hysteria2 нативно, используйте --core sing-box или --core mihomo")
	default:
		return nil
	}
}

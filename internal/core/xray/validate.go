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
	case types.ProtocolMieru:
		// mieru = supervised peer (ADR-0020). Поддерживается только sing-box
		// (через socks bridge на 127.0.0.1:LocalSocksPort). Для xray/mihomo
		// нет аналогичной интеграции.
		return fmt.Errorf("xray не поддерживает mieru как supervised peer, используйте --core sing-box")
	case types.ProtocolNaive:
		// naive = supervised peer (ADR-0021-naiveproxy-process-chain).
		// sign-craze запускает klzgrad/naiveproxy daemon и подключает sing-box
		// через локальный SOCKS5. Для xray/mihomo аналогичной интеграции нет.
		return fmt.Errorf("xray не поддерживает naive как supervised peer, используйте --core sing-box")
	default:
		return nil
	}
}

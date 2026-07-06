package core

import (
	"fmt"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// RejectSupervisedPeer отклоняет протоколы, реализованные как supervised
// peer: mieru (ADR-0020) и naive (ADR-0021-naiveproxy-process-chain).
// Для обоих sign-craze запускает отдельный процесс (mieru-клиент /
// klzgrad/naiveproxy) и направляет на его локальный SOCKS5-порт только
// sing-box — ни xray, ни mihomo аналогичной интеграции не имеют.
//
// Validate каждого из этих двух ядер обязан вызывать RejectSupervisedPeer
// первым, до собственных ядро-специфичных ограничений. coreName — имя
// вызывающего ядра для текста ошибки ("xray", "mihomo").
func RejectSupervisedPeer(coreName string, c types.Canonical) error {
	switch c.Protocol {
	case types.ProtocolMieru, types.ProtocolNaive:
		return fmt.Errorf("%s не поддерживает %s как supervised peer, используйте --core sing-box", coreName, c.Protocol)
	default:
		return nil
	}
}

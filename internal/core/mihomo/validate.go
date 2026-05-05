package mihomo

import (
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// Validate проверяет совместимость canonical-описания outbound с mihomo.
// Возвращает ошибку если конфигурация требует возможностей, которые mihomo
// не поддерживает (PQ-VLESS, Vision UDP443 и т.п.).
func Validate(c types.Canonical) error {
	if c.Proto == nil {
		return nil
	}
	if c.Protocol == types.ProtocolVLESS {
		return validateVLESS(c)
	}
	return nil
}

// validateVLESS проверяет VLESS-специфичные ограничения mihomo.
func validateVLESS(c types.Canonical) error {
	proto := c.Proto

	// Vision UDP443 split — mihomo не поддерживает суффикс -udp443.
	// Пользователь должен переключиться на --core xray.
	if strings.HasSuffix(proto.Flow, "-udp443") {
		return fmt.Errorf(
			"mihomo не поддерживает Vision UDP443 (flow=%q): "+
				"используйте --core xray для данного соединения",
			proto.Flow,
		)
	}

	// PQ-VLESS (mlkem768x25519plus) реализован только в xray.
	// Mihomo (Clash.Meta) не имеет стабильной поддержки PQ-VLESS на 2026-04.
	// URL может содержать суффиксы: mlkem768x25519plus.native.0rtt.<base64> —
	// используем HasPrefix вместо точного сравнения.
	if strings.HasPrefix(proto.Encryption, "mlkem768x25519plus") {
		return fmt.Errorf(
			"mihomo не поддерживает PQ-VLESS (encryption=%q): "+
				"используйте --core xray для данного соединения",
			proto.Encryption,
		)
	}

	return nil
}

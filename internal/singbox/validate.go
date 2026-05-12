package singbox

import (
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// Validate проверяет совместимость outbound с sing-box.
// Возвращает ошибку с подсказкой про другое ядро, если outbound использует
// xray-only возможности (XHTTP modes, PQ-VLESS encryption).
//
// Reality spider_x (spx=...) поддерживается только в xray; sing-box render
// его молча игнорирует (см. render.go), поэтому валидацию не блокируем.
func Validate(o types.Outbound) error {
	// XHTTP mode (stream-up/stream-one/packet-up) — только xray.
	if o.Transport != nil && o.Transport.Kind == types.TransportXHTTP &&
		o.Transport.Mode != types.XHTTPNone {
		return fmt.Errorf(
			"sing-box не поддерживает XHTTP mode=%q: используйте --core xray",
			o.Transport.Mode,
		)
	}
	// PQ-VLESS encryption (mlkem768x25519plus с любыми суффиксами .native.0rtt.X...).
	if o.Proto != nil && strings.HasPrefix(o.Proto.Encryption, "mlkem768x25519plus") {
		return fmt.Errorf(
			"sing-box не поддерживает PQ-VLESS encryption=%q: используйте --core xray",
			o.Proto.Encryption,
		)
	}
	return nil
}

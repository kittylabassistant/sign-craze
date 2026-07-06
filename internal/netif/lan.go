// Package netif — детекция сетевых интерфейсов для bind/firewall логики.
//
// LAN-bridge детектируется через net.InterfaceByName с фолбэком по списку
// стандартных имён (Keenetic = br0, OpenWRT = br-lan). Используется чтобы
// привязать Web UI listener к LAN-IP (не 0.0.0.0) — UI работает в
// LAN-trust-модели без auth/TLS, поэтому WAN-доступность недопустима.
package netif

import (
	"errors"
	"fmt"
	"net"
)

// LANBridgeCandidates — стандартные имена LAN-bridge на разных embedded
// Linux-дистрибутивах. Порядок отражает целевые платформы sign-craze.
var LANBridgeCandidates = []string{"br0", "br-lan", "lan"}

// ErrLANNotFound — ни один из LANBridgeCandidates не существует или не имеет IPv4.
var ErrLANNotFound = errors.New("netif: LAN-bridge не найден")

// DetectLANAddr возвращает первый IPv4-адрес одного из LAN-bridge интерфейсов.
// Приоритет: br0 (Keenetic) → br-lan (OpenWRT) → lan (generic).
// IPv6 пропускается: первая итерация LAN-only enforcement — IPv4-only.
func DetectLANAddr() (string, error) {
	for _, name := range LANBridgeCandidates {
		addr, err := lanAddrFromIface(name)
		if err == nil {
			return addr, nil
		}
	}
	return "", fmt.Errorf("%w (искали %v)", ErrLANNotFound, LANBridgeCandidates)
}

func lanAddrFromIface(name string) (string, error) {
	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return "", fmt.Errorf("netif: интерфейс %q: %w", name, err)
	}
	addrs, err := ifi.Addrs()
	if err != nil {
		return "", fmt.Errorf("netif: %s.Addrs: %w", name, err)
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip4 := ipNet.IP.To4()
		if ip4 == nil {
			continue
		}
		return ip4.String(), nil
	}
	return "", fmt.Errorf("netif: %s не имеет IPv4-адреса", name)
}

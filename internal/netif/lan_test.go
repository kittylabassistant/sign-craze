package netif

import (
	"errors"
	"net"
	"testing"
)

// TestDetectLANAddr_NoCandidates — на хосте без br0/br-lan/lan детект
// возвращает ErrLANNotFound. Это контракт: production-код должен fail-secure.
func TestDetectLANAddr_NoCandidates(t *testing.T) {
	for _, name := range LANBridgeCandidates {
		if _, err := net.InterfaceByName(name); err == nil {
			t.Skipf("в окружении присутствует %q — тест неприменим", name)
		}
	}
	addr, err := DetectLANAddr()
	if err == nil {
		t.Fatalf("ожидался ErrLANNotFound, получен addr=%q", addr)
	}
	if !errors.Is(err, ErrLANNotFound) {
		t.Fatalf("ожидался ErrLANNotFound, получено: %v", err)
	}
}

// TestLANAddrOf_Loopback — резолв конкретного интерфейса (lo) даёт 127.0.0.1.
// Проверяет что lanAddrFromIface работает корректно на любом существующем iface.
func TestLANAddrOf_Loopback(t *testing.T) {
	loopback := loopbackName(t)
	addr, err := LANAddrOf(loopback)
	if err != nil {
		t.Fatalf("LANAddrOf(%q): %v", loopback, err)
	}
	if addr != "127.0.0.1" {
		t.Fatalf("ожидался 127.0.0.1, получен %q", addr)
	}
}

// TestLANAddrOf_Missing — несуществующий интерфейс даёт ошибку.
func TestLANAddrOf_Missing(t *testing.T) {
	_, err := LANAddrOf("sign-craze-nonexistent-iface-x9k7")
	if err == nil {
		t.Fatal("ожидалась ошибка для несуществующего интерфейса")
	}
}

// loopbackName возвращает имя loopback-интерфейса в текущей системе
// (lo на Linux, lo0 на BSD/macOS).
func loopbackName(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"lo", "lo0"} {
		if _, err := net.InterfaceByName(name); err == nil {
			return name
		}
	}
	t.Skip("loopback-интерфейс не найден")
	return ""
}

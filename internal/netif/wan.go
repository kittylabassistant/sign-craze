package netif

import (
	"context"
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// DetectWANIface определяет имя WAN-интерфейса в Linux через
// `ip route show default` и парсинг "dev <iface>". На Keenetic возвращает
// kernel-имя (eth3, ppp0), не RCI-имя (GigabitEthernet1) — для использования
// в iptables -i.
//
// Использовать когда WANIface не задан явно в конфигурации; результат
// должен валидироваться вызывающим (fail-secure если пусто).
func DetectWANIface(ctx context.Context, runner exectx.Runner) (string, error) {
	res, err := runner.Run(ctx, "ip", "route", "show", "default")
	if err != nil {
		return "", fmt.Errorf("netif: ip route show default: %w", err)
	}
	return ParseDefaultRouteIface(string(res.Stdout))
}

// ParseDefaultRouteIface извлекает первое значение после "dev" в выводе
// `ip route show default`. Чистый парсер без exec — тестируется напрямую.
func ParseDefaultRouteIface(routeOutput string) (string, error) {
	for _, line := range strings.Split(routeOutput, "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if f == "dev" && i+1 < len(fields) {
				return fields[i+1], nil
			}
		}
	}
	return "", fmt.Errorf("netif: не удалось определить WAN-интерфейс из вывода ip route")
}

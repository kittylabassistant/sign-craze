package geo

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
)

// ParseCIDRList разбирает текстовый CIDR-лист (по одному префиксу на строку).
// Строки начинающиеся с '#' и пустые строки игнорируются.
// Возвращает ошибку на первой невалидной строке.
func ParseCIDRList(data []byte) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix

	start := 0
	for i := 0; i <= len(data); i++ {
		if i == len(data) || data[i] == '\n' {
			line := trimSpace(data[start:i])
			start = i + 1

			if len(line) == 0 || line[0] == '#' {
				continue
			}

			p, err := netip.ParsePrefix(string(line))
			if err != nil {
				return nil, fmt.Errorf("geo ipset: невалидный CIDR %q: %w", line, err)
			}
			prefixes = append(prefixes, p.Masked())
		}
	}

	return prefixes, nil
}

// ApplyToIPSet атомарно заменяет содержимое ipset-набора списком префиксов.
// Делегирует в firewall.IPSet.AtomicReplace (create-swap-destroy).
// setName — имя набора (напр. "signcraze_ipv4").
// family — "inet" для IPv4, "inet6" для IPv6.
func ApplyToIPSet(ctx context.Context, runner exectx.Runner, setName, family string, prefixes []netip.Prefix) error {
	ipset := firewall.NewIPSet(runner)
	if err := ipset.AtomicReplace(ctx, setName, "hash:net", family, prefixes); err != nil {
		return fmt.Errorf("geo ipset: обновление %s: %w", setName, err)
	}
	return nil
}

// trimSpace обрезает ведущие и завершающие пробелы/табуляции без аллокаций на строках без пробелов.
func trimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r') {
		end--
	}
	return b[start:end]
}

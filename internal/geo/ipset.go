package geo

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
)

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

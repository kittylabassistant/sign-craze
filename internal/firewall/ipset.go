package firewall

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// IPSet управляет ipset-наборами через exectx.Runner.
type IPSet struct {
	runner exectx.Runner
}

// NewIPSet создаёт IPSet с заданным runner.
func NewIPSet(runner exectx.Runner) *IPSet {
	return &IPSet{runner: runner}
}

// EnsureSet создаёт ipset-набор если он отсутствует. Идемпотентно.
func (s *IPSet) EnsureSet(ctx context.Context, name, setType, family string) error {
	if _, err := s.runner.Run(ctx, "ipset", "list", name); err == nil {
		return nil
	}
	if _, err := s.runner.Run(ctx, "ipset", "create", name, setType, "family", family); err != nil {
		return fmt.Errorf("firewall: создание ipset %s: %w", name, err)
	}
	log.L().Debug("firewall: ipset создан", "name", name, "type", setType, "family", family)
	return nil
}

// AtomicReplace заменяет содержимое набора атомарно через create-swap-destroy.
// Алгоритм: создать tmp → добавить cidrs → swap(name, tmp) → destroy tmp.
// При ошибке добавления — tmp уничтожается без swap.
func (s *IPSet) AtomicReplace(ctx context.Context, name, setType, family string, cidrs []netip.Prefix) error {
	tmp := name + "_tmp"

	if _, err := s.runner.Run(ctx, "ipset", "create", tmp, setType, "family", family); err != nil {
		return fmt.Errorf("firewall: создание tmp ipset %s: %w", tmp, err)
	}

	for _, cidr := range cidrs {
		if _, err := s.runner.Run(ctx, "ipset", "add", tmp, cidr.String()); err != nil {
			// откат — уничтожаем tmp
			if _, delErr := s.runner.Run(ctx, "ipset", "destroy", tmp); delErr != nil {
				log.L().Warn("firewall: не удалось уничтожить tmp ipset при откате", "tmp", tmp, "err", delErr)
			}
			return fmt.Errorf("firewall: добавление %s в tmp ipset: %w", cidr, err)
		}
	}

	if _, err := s.runner.Run(ctx, "ipset", "swap", name, tmp); err != nil {
		if _, delErr := s.runner.Run(ctx, "ipset", "destroy", tmp); delErr != nil {
			log.L().Warn("firewall: не удалось уничтожить tmp ipset после ошибки swap", "tmp", tmp, "err", delErr)
		}
		return fmt.Errorf("firewall: swap ipset %s → %s: %w", tmp, name, err)
	}

	if _, err := s.runner.Run(ctx, "ipset", "destroy", tmp); err != nil {
		log.L().Warn("firewall: не удалось уничтожить tmp ipset после swap", "tmp", tmp, "err", err)
	}
	return nil
}

// DestroySet удаляет ipset-набор. Идемпотентно — игнорирует "does not exist".
func (s *IPSet) DestroySet(ctx context.Context, name string) error {
	_, err := s.runner.Run(ctx, "ipset", "destroy", name)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "The set with the given name does not exist") {
		return nil
	}
	return fmt.Errorf("firewall: уничтожение ipset %s: %w", name, err)
}

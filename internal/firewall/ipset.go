package firewall

import (
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// ipsetBatchThreshold — порог переключения с поштучного `ipset add` на
// batch `ipset restore` через stdin. Один subprocess вместо N экономит
// 60-150 секунд при geo-обновлении (~10K CIDR) на slow MIPS softfloat.
const ipsetBatchThreshold = 32

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

// AtomicReplace заменяет содержимое набора атомарно. При количестве CIDR ≥
// ipsetBatchThreshold и поддержке runner'ом stdin (StdinRunner) — использует
// `ipset restore` (один subprocess для всего batch). Иначе fallback на
// поштучный `ipset add` (legacy путь, нужен для mock-runner'ов в тестах).
func (s *IPSet) AtomicReplace(ctx context.Context, name, setType, family string, cidrs []netip.Prefix) error {
	tmp := name + "_tmp"

	// Sweep stale tmp от прошлых неудачных Apply (safety-fixes #13). Идемпотентно.
	if _, err := s.runner.Run(ctx, "ipset", "destroy", tmp); err != nil {
		log.L().Debug("firewall: sweep tmp ipset (no-op если отсутствует)", "tmp", tmp)
	}

	if stdinRunner, ok := s.runner.(exectx.StdinRunner); ok && len(cidrs) >= ipsetBatchThreshold {
		return s.atomicReplaceBatch(ctx, stdinRunner, name, tmp, setType, family, cidrs)
	}
	return s.atomicReplaceLegacy(ctx, name, tmp, setType, family, cidrs)
}

// atomicReplaceBatch выполняет create+add+swap+destroy одним вызовом
// `ipset restore` через stdin. Формат stdin — Linux ipset save/restore:
//
//	create <tmp> <type> family <family>
//	add <tmp> <cidr>
//	...
//	swap <name> <tmp>
//	destroy <tmp>
//
// При ошибке kernel обнаруживает невалидную операцию и rollback'ит весь
// batch (ipset transaction-aware). Для надёжности также делаем явный
// destroy <tmp> отдельным вызовом — на случай если swap не достиг конца.
func (s *IPSet) atomicReplaceBatch(ctx context.Context, runner exectx.StdinRunner, name, tmp, setType, family string, cidrs []netip.Prefix) error {
	var buf bytes.Buffer
	// preallocate: ~30 байт на ADD-строку (cidr + префикс) для умеренной
	// экономии аллокаций при 10K CIDR (≈300KB строки).
	buf.Grow(64 + 30*len(cidrs))
	fmt.Fprintf(&buf, "create %s %s family %s\n", tmp, setType, family)
	for _, cidr := range cidrs {
		fmt.Fprintf(&buf, "add %s %s\n", tmp, cidr.String())
	}
	fmt.Fprintf(&buf, "swap %s %s\n", name, tmp)
	fmt.Fprintf(&buf, "destroy %s\n", tmp)

	if _, err := runner.RunWithStdin(ctx, &buf, "ipset", "restore"); err != nil {
		// rollback: явно уничтожаем tmp если он остался от частично применённого batch
		if _, delErr := s.runner.Run(ctx, "ipset", "destroy", tmp); delErr != nil {
			log.L().Debug("firewall: rollback tmp после ipset restore (no-op если отсутствует)", "tmp", tmp, "err", delErr)
		}
		return fmt.Errorf("firewall: ipset restore (%d CIDR в %s): %w", len(cidrs), name, err)
	}
	log.L().Debug("firewall: ipset batch restore", "set", name, "cidrs", len(cidrs))
	return nil
}

// atomicReplaceLegacy — поштучный путь для тестовых mock-runner'ов.
// Identичен прежней реализации до F1.
func (s *IPSet) atomicReplaceLegacy(ctx context.Context, name, tmp, setType, family string, cidrs []netip.Prefix) error {
	if _, err := s.runner.Run(ctx, "ipset", "create", tmp, setType, "family", family); err != nil {
		return fmt.Errorf("firewall: создание tmp ipset %s: %w", tmp, err)
	}

	for _, cidr := range cidrs {
		if _, err := s.runner.Run(ctx, "ipset", "add", tmp, cidr.String()); err != nil {
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

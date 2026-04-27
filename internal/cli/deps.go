package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/dpi"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
)

// newRunner возвращает стандартный OS-runner для exec-команд.
func newRunner() exectx.Runner { return exectx.OS }

// newSingboxLifecycle возвращает Lifecycle с дефолтными путями.
func newSingboxLifecycle() service.Lifecycle { return singbox.DefaultLifecycle() }

// newDPILifecycle возвращает Lifecycle nfqws2 с дефолтными путями.
func newDPILifecycle() service.Lifecycle { return dpi.DefaultLifecycle() }

// newFirewallApplier строит Applier из state: пробрасывает ports/excludes в Config.
func newFirewallApplier(s *state.State) (firewall.Applier, error) {
	cfg := firewall.DefaultConfig()
	cfg.Ports = append([]uint16(nil), s.Ports...)
	excl, err := state.ParsedExcludes(s)
	if err != nil {
		return nil, fmt.Errorf("firewall: некорректные excludes: %w", err)
	}
	cfg.Excludes = excl
	return firewall.NewApplier(newRunner(), cfg), nil
}

// loadState читает state.json из стандартного пути.
func loadState() (*state.State, error) {
	return state.Load(state.DefaultPath)
}

// saveState атомарно записывает state.json по стандартному пути.
func saveState(s *state.State) error {
	return state.Save(state.DefaultPath, s)
}

// configPath — путь к /opt/etc/sign-craze/config.json.
func configPath() string {
	return filepath.Join(singbox.DefaultConfigDir, "config.json")
}

// regenerateConfig генерирует config.json sing-box из state и атомарно записывает.
// Если бинарь sing-box установлен, выполняет валидацию (sing-box check -c) перед записью.
func regenerateConfig(ctx context.Context, s *state.State) error {
	params := singbox.DefaultConfigParams()
	params.Mode = s.Mode
	params.Outbounds = s.Outbounds
	if len(s.Outbounds) > 0 {
		params.DefaultOutboundTag = s.Outbounds[0].Tag
	}
	return singbox.WriteConfig(ctx, newRunner(), params, singbox.DefaultBinPath, configPath())
}

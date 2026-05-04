package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/dpi"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/log"
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

// newFirewallApplier строит Applier из state: пробрасывает ports/excludes/admin
// + PolicyMark + DPIEnabled в Config.
// AdminIPs мерджатся в Excludes — оба попадают в ipset signcraze_excludes.
func newFirewallApplier(s *state.State) (firewall.Applier, error) {
	cfg := firewall.DefaultConfig()
	cfg.Ports = append([]uint16(nil), s.Ports...)
	cfg.AdminPorts = append([]uint16(nil), s.AdminPorts...)
	cfg.PolicyMark = s.PolicyMark
	cfg.DPIEnabled = s.DPIEnabled

	excl, err := state.ParsedExcludes(s)
	if err != nil {
		return nil, fmt.Errorf("firewall: некорректные excludes: %w", err)
	}
	adminPrefixes, err := state.ParsedAdminIPs(s)
	if err != nil {
		return nil, fmt.Errorf("firewall: некорректные admin IPs: %w", err)
	}
	excl = append(excl, adminPrefixes...)
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

// configParamsFromState собирает singbox.ConfigParams на основе state.
// Используется регенерацией и сравнением "ожидаемого" конфига с дисковым.
func configParamsFromState(s *state.State) singbox.ConfigParams {
	params := singbox.DefaultConfigParams()
	params.Mode = s.Mode
	params.Outbounds = s.Outbounds
	if len(s.Outbounds) > 0 {
		params.DefaultOutboundTag = s.Outbounds[0].Tag
	}
	return params
}

// regenerateConfig генерирует config.json sing-box из state и атомарно записывает.
// Если бинарь sing-box установлен, выполняет валидацию (sing-box check -c) перед записью.
func regenerateConfig(ctx context.Context, s *state.State) error {
	params := configParamsFromState(s)
	return singbox.WriteConfig(ctx, newRunner(), params, singbox.DefaultBinPath, configPath())
}

// ensureConfigFresh регенерирует config.json только если рендер из текущих
// дефолтов и state расходится с дисковым содержимым. Это нужно после ручной
// замены /opt/sbin/sign-craze в обход --update-core: дефолты в коде (например,
// TUN MTU) могли измениться, а на диске лежит конфиг от старого бинаря.
//
// На fast-path (конфиг идентичен) пропускаем `sing-box check -c`, который
// на slow MIPS softfloat занимает десятки секунд и удлинял бы каждый --start
// и init.d boot.
func ensureConfigFresh(ctx context.Context, s *state.State) error {
	params := configParamsFromState(s)
	want, err := singbox.Render(params)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	got, err := os.ReadFile(configPath())
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Файла нет — пишем напрямую.
	case err != nil:
		return fmt.Errorf("чтение текущего config.json: %w", err)
	default:
		if bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
			return nil
		}
		log.L().Info("config.json устарел, регенерация")
	}
	return singbox.WriteConfig(ctx, newRunner(), params, singbox.DefaultBinPath, configPath())
}

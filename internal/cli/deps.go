package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/dpi"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/routing"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
)

// newDPILifecycle возвращает Lifecycle nfqws2 с дефолтными путями.
// Используется для Stop/Status/Diag — операции по PIDFile, cmdline не критичен.
// Для запуска (Start) использовать newDPILifecycleFromState — cmdline должен
// учитывать state.DPIStrategy и hostlist.
func newDPILifecycle() service.Lifecycle { return dpi.DefaultLifecycle() }

// newDPILifecycleFromState возвращает Lifecycle nfqws2 с cmdline, собранным из
// текущего state. Если state.DPIStrategy непустой — он переопределяет
// NFQWS_ARGS (TCP/TLS-блок).
//
// HostlistPath подключается через --hostlist= если выполнено хотя бы одно:
//  1. state.DPITargets непустой (явные домены от пользователя);
//  2. /opt/etc/sign-craze/dpi-hostlist.txt существует на диске (auto-update
//     создал файл из upstream-источников; см. dpi.UpdateHostlist).
//
// Без проверки (2) auto-update скачивает файл, но nfqws2 запускается без
// --hostlist и десинкает весь трафик — теряется selective-эффект.
func newDPILifecycleFromState(st *state.State, iface string) service.Lifecycle {
	params := dpi.DefaultConfigParams()
	params.ISPInterface = iface
	if st != nil && st.DPIStrategy != "" {
		params.Args = st.DPIStrategy
	}
	if hostlistShouldApply(st) {
		params.HostlistPath = dpi.DefaultHostlistPath
	}
	return dpi.NewLifecycle(dpi.DefaultBinPath, params, dpi.DefaultPIDFile)
}

// hostlistShouldApply возвращает true если nfqws2 должен запускаться с
// --hostlist=. Условие: явные DPITargets, либо файл уже создан auto-update'ом.
func hostlistShouldApply(st *state.State) bool {
	if st != nil && len(st.DPITargets) > 0 {
		return true
	}
	if _, err := os.Stat(dpi.DefaultHostlistPath); err == nil {
		return true
	}
	return false
}

// newFirewallApplier строит Applier из state: пробрасывает ports/excludes/admin
// + PolicyMark + DPIEnabled в Config.
// AdminIPs мерджатся в Excludes — оба попадают в ipset signcraze_excludes.
//
// SkipTUNCheck определяется по активному ядру: sing-box работает через TUN
// (pre-flight CheckTUNAvailable обязателен), xray/mihomo — через TProxy и
// не создают TUN-устройство (проверка ложно фейлится в среде без TUN).
//
// VPNExcludeIPs мерджит state.DPIExcludeIPs (явные IP от пользователя — для
// downstream-VPN-клиентов) с резолвом server у первого outbound (sing-box к
// собственному Reality-серверу). Десинхронизация на этих IP отключается, чтобы
// не ломать TLS-маскировку VPN-handshake.
func newFirewallApplier(s *state.State) (firewall.Applier, error) {
	cfg := firewall.DefaultConfig()
	cfg.Ports = append([]uint16(nil), s.Ports...)
	cfg.AdminPorts = append([]uint16(nil), s.AdminPorts...)
	cfg.PolicyMark = s.PolicyMark
	cfg.DPIEnabled = s.DPIEnabled
	cfg.SkipTUNCheck = s.Core != "" && s.Core != state.DefaultCore
	cfg.UseTProxy = s.Inbound == "tproxy"
	cfg.SkipTUNCheck = cfg.SkipTUNCheck || cfg.UseTProxy

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

	cfg.VPNExcludeIPs = collectVPNExcludeIPs(s)
	return firewall.NewApplier(exectx.OS, cfg), nil
}

// collectVPNExcludeIPs собирает список IP-эндпоинтов VPN, для которых отключается
// nfqws2-десинхронизация:
//   - state.DPIExcludeIPs (явные IP от пользователя — главный механизм управления);
//   - резолв первого outbound.Server (best-effort: ошибка резолва игнорируется,
//     явные DPIExcludeIPs остаются единственным барьером).
//
// Дубликаты удаляются. Резолв homelabcloud.ru и подобных хостов идёт через
// system resolver — у нас нет гарантии что Keenetic-DNS вернёт реальный IP
// (Reality маскируется под другие домены), поэтому DPIExcludeIPs — авторитетный
// источник; outbound.Server-резолв — опортунистическое дополнение.
func collectVPNExcludeIPs(s *state.State) []string {
	seen := make(map[string]struct{}, len(s.DPIExcludeIPs)+1)
	out := make([]string, 0, len(s.DPIExcludeIPs)+1)
	for _, ip := range s.DPIExcludeIPs {
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	if len(s.Outbounds) > 0 {
		host := s.Outbounds[0].Server
		if host != "" {
			if addrs, err := net.LookupHost(host); err == nil {
				for _, ip := range addrs {
					if _, ok := seen[ip]; ok {
						continue
					}
					seen[ip] = struct{}{}
					out = append(out, ip)
				}
			}
		}
	}
	return out
}

// loadState читает state.json из стандартного пути.
func loadState() (*state.State, error) {
	return state.Load(state.DefaultPath)
}

// activeCore возвращает Core, соответствующее state.Core активного state.json.
// При отсутствии state файла возвращает Default sing-box.
//
// Для unit-тестов и read-only диагностики, где state.json может отсутствовать,
// используйте mustActiveCore — оно гарантирует ненулевой Core.
func activeCore() (core.Core, error) {
	s, err := loadState()
	if err != nil {
		return nil, fmt.Errorf("activeCore: %w", err)
	}
	c, err := core.Active(s.Core)
	if err != nil {
		return nil, fmt.Errorf("activeCore: %w", err)
	}
	return c, nil
}

// mustActiveCore — activeCore с fallback на DefaultCore при ошибках.
// Гарантирует ненулевой Core; используется в путях read-only (status, version),
// где краш из-за повреждённого state.json неприемлем.
func mustActiveCore() core.Core {
	if c, err := activeCore(); err == nil {
		return c
	}
	return core.MustGet(state.DefaultCore)
}

// activeCoreProvider реализует web.CoresProvider на базе текущего state.json.
// Используется web-сервером для GET /api/cores, чтобы пометить активное ядро.
type activeCoreProvider struct{}

func (activeCoreProvider) ActiveCoreName() string {
	st, err := loadState()
	if err != nil || st == nil {
		return state.DefaultCore
	}
	if st.Core == "" {
		return state.DefaultCore
	}
	return st.Core
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
//
// Если /opt/etc/sign-craze/routing.json существует — он подгружается в
// params.RoutingConfig и имеет приоритет над legacy outbounds/routing
// (см. internal/singbox/render_rules.go: buildEffectiveModel).
func configParamsFromState(s *state.State) singbox.ConfigParams {
	params := singbox.DefaultConfigParams()
	params.Mode = s.Mode
	params.Outbounds = s.Outbounds
	if len(s.Outbounds) > 0 {
		params.DefaultOutboundTag = s.Outbounds[0].Tag
	}
	params.InboundMode = s.Inbound
	if rc, err := routing.Load(routing.DefaultPath); err != nil {
		log.L().Warn("routing.json: ошибка загрузки, используется legacy",
			"path", routing.DefaultPath, "err", err)
	} else if rc != nil {
		// Миграция inbound-режима: если state.Inbound=tproxy, но routing.json
		// содержит legacy TUN inbound (от v0.6.x bootstrap), фильтруем TUN из
		// rc.Inbounds. Без миграции Render() считает RoutingConfig.Inbounds
		// "явно заданными" и пропускает TPROXY-генерацию → sing-box стартует
		// с TUN inbound, но iptables направляют через TPROXY → marked-трафик
		// LAN-клиентов попадает в local socket 7895, который не существует
		// (sing-box слушает signbox-tun) → reset, нет интернета.
		// См. v0.8.3 release notes.
		if s.Inbound == "tproxy" && len(rc.Inbounds) > 0 {
			filtered := rc.Inbounds[:0]
			removed := 0
			for _, in := range rc.Inbounds {
				if in.Type == "tun" {
					removed++
					continue
				}
				filtered = append(filtered, in)
			}
			if removed > 0 {
				rc.Inbounds = filtered
				log.L().Warn("routing.json: legacy TUN inbound удалён из RoutingConfig (state.inbound=tproxy)",
					"removed_count", removed,
					"hint", "запустите --restart, и Render автогенерирует TPROXY-inbound из state",
				)
				if err := routing.Save(routing.DefaultPath, rc); err != nil {
					log.L().Warn("routing.json: не удалось сохранить миграцию",
						"path", routing.DefaultPath, "err", err)
				}
			}
		}
		params.RoutingConfig = rc
		// Если в RoutingConfig есть outbounds — DefaultOutboundTag берётся из первого.
		if len(rc.Outbounds) > 0 && params.DefaultOutboundTag == "" {
			params.DefaultOutboundTag = rc.Outbounds[0].Tag
		}
	}

	// Probe TPROXY: определяем доступность xt_TPROXY для рендера inbound type.
	// Это probe (read-only), не actual insmod — insmod делает applier при apply.
	// Для рендера config достаточно знать: модуль загружен или хотя бы доступен (.ko-файл).
	if s.Inbound == "tproxy" {
		tproxyOK := firewall.IsModuleLoaded("xt_TPROXY")
		if !tproxyOK {
			// .ko-файл существует → модуль loadable (insmod сделает applier).
			tproxyOK = firewall.FindKernelModule("xt_TPROXY") != ""
		}
		params.TProxyKernelOK = tproxyOK
	}

	return params
}

// singboxParamsForInstall собирает singbox.ConfigParams для install/update-core,
// когда routing.json ещё может не существовать. Извлечён из дублирующегося
// блока в cmd_install.go и cmd_update.go.
func singboxParamsForInstall(s *state.State) singbox.ConfigParams {
	params := singbox.DefaultConfigParams()
	params.Mode = s.Mode
	params.Outbounds = s.Outbounds
	if len(s.Outbounds) > 0 {
		params.DefaultOutboundTag = s.Outbounds[0].Tag
	}
	params.InboundMode = s.Inbound
	return params
}

// regenerateConfig генерирует config.json sing-box из state и атомарно записывает.
// Если бинарь sing-box установлен, выполняет валидацию (sing-box check -c) перед записью.
func regenerateConfig(ctx context.Context, s *state.State) error {
	if s == nil {
		return fmt.Errorf("regenerateConfig: state is nil")
	}
	params := configParamsFromState(s)
	return singbox.WriteConfig(ctx, exectx.OS, params, singbox.DefaultBinPath, configPath())
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
	return singbox.WriteConfig(ctx, exectx.OS, params, singbox.DefaultBinPath, configPath())
}

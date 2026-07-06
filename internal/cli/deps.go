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

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/dpi"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/naiveproxy"
	"github.com/kittylabassistant/sign-craze/internal/netif"
	"github.com/kittylabassistant/sign-craze/internal/routing"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
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

// newNaiveLifecycle возвращает Lifecycle naive с дефолтными путями.
// Используется для Stop — параметры ProxyURL не нужны (только PIDFile).
// Для старта использовать newNaiveLifecycleFromOutbound — ProxyURL обязателен.
func newNaiveLifecycle() service.Lifecycle { return naiveproxy.DefaultLifecycle() }

// newNaiveLifecycleFromOutbound собирает Lifecycle с параметрами из конкретного
// outbound (Protocol=ProtocolNaive). Используется в --start: на каждый naive
// outbound запускается отдельный naive-демон с upstream URL из state.
func newNaiveLifecycleFromOutbound(o types.Outbound) (service.Lifecycle, error) {
	params, err := naiveproxy.ParamsFromCanonical(o)
	if err != nil {
		return nil, err
	}
	return naiveproxy.NewLifecycle(naiveproxy.DefaultBinPath, params, naiveproxy.DefaultPIDFile), nil
}

// newFirewallApplier строит Applier из state: пробрасывает ports/excludes/admin
// + PolicyMark + DPIEnabled в Config.
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
	cfg.Excludes = excl

	cfg.VPNExcludeIPs = collectVPNExcludeIPs(s)

	if ip, err := netif.DetectLANAddr(); err == nil && ip != "" {
		cfg.LANAddrs = []string{ip}
	}

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
			if addrs, err := net.DefaultResolver.LookupHost(context.Background(), host); err == nil {
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

// configPath — путь к /opt/etc/sign-craze/config.json (sing-box).
// Используется legacy кодом (ConfigRW в cmd_ui для админ REST API).
// Для активного ядра используйте core.Active(state.Core).ConfigPath().
func configPath() string {
	return filepath.Join(singbox.DefaultConfigDir, "config.json")
}

// loadAndFilterRouting загружает /opt/etc/sign-craze/routing.json и применяет
// миграцию legacy TUN inbound при state.Inbound=tproxy.
//
// Миграция: если state.Inbound=tproxy, но routing.json содержит legacy TUN
// inbound (от v0.6.x bootstrap), TUN отфильтровывается из rc.Inbounds. Без
// миграции Render() считает RoutingConfig.Inbounds "явно заданными" и пропускает
// TPROXY-генерацию → sing-box стартует с TUN inbound, но iptables направляют
// через TPROXY → marked-трафик LAN-клиентов попадает в local socket 7895,
// который не существует → reset, нет интернета. См. v0.8.3 release notes.
//
// Возвращает nil без ошибки, если routing.json отсутствует — caller использует
// fallback на legacy outbound-only путь.
func loadAndFilterRouting(s *state.State) *types.RoutingConfig {
	rc, err := routing.Load(routing.DefaultPath)
	if err != nil {
		log.L().Warn("routing.json: ошибка загрузки, используется legacy",
			"path", routing.DefaultPath, "err", err)
		return nil
	}
	if rc == nil {
		return nil
	}
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
	return rc
}

// coreRenderParamsFromState собирает types.CoreRenderParams из state с подгрузкой
// routing.json и миграцией. Используется регенерацией (regenerateConfig) и
// startup-проверкой (ensureConfigFreshForCore) — единый источник params.
func coreRenderParamsFromState(s *state.State) types.CoreRenderParams {
	rc := loadAndFilterRouting(s)
	defaultTag := ""
	if len(s.Outbounds) > 0 {
		defaultTag = s.Outbounds[0].Tag
	}
	if rc != nil && defaultTag == "" && len(rc.Outbounds) > 0 {
		defaultTag = rc.Outbounds[0].Tag
	}
	return types.CoreRenderParams{
		Mode:               s.Mode,
		Outbounds:          s.Outbounds,
		RoutingConfig:      rc,
		InboundMode:        s.Inbound,
		DefaultOutboundTag: defaultTag,
	}
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

// regenerateConfig — core-aware регенерация конфига активного ядра.
// Резолвит ядро по state.Core, рендерит через c.RenderConfig (включая RoutingConfig),
// атомарно записывает в c.ConfigPath() и вызывает c.CheckConfig для валидации.
//
// Fast-path: если рендер байт-в-байт совпадает с текущим содержимым файла —
// запись и check пропускаются (важно на slow MIPS, где sing-box check / xray test
// занимают десятки секунд).
func regenerateConfig(ctx context.Context, s *state.State) error {
	if s == nil {
		return fmt.Errorf("regenerateConfig: state is nil")
	}
	c, err := core.Active(s.Core)
	if err != nil {
		return fmt.Errorf("regenerateConfig: %w", err)
	}
	return ensureConfigFreshForCore(ctx, c, s)
}

// ensureConfigFreshForCore регенерирует конфиг активного ядра и записывает на
// диск, если рендер отличается от текущего содержимого. На совпадении — no-op.
//
// Один общий путь для всех ядер: sing-box, xray, mihomo. Каждое ядро через свой
// RenderConfig переводит RoutingConfig в свою натив-форму.
func ensureConfigFreshForCore(ctx context.Context, c core.Core, s *state.State) error {
	params := coreRenderParamsFromState(s)
	want, err := c.RenderConfig(params)
	if err != nil {
		return fmt.Errorf("render (%s): %w", c.Name(), err)
	}
	configPath := c.ConfigPath()
	got, readErr := os.ReadFile(configPath)
	switch {
	case errors.Is(readErr, fs.ErrNotExist):
		// Файла нет — пишем напрямую.
	case readErr != nil:
		return fmt.Errorf("чтение текущего конфига (%s): %w", c.Name(), readErr)
	default:
		if bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
			return nil
		}
		log.L().Info("конфиг устарел, регенерация", "core", c.Name(), "path", configPath)
	}
	if writeErr := atomicfs.WriteFileAtomic(configPath, want, 0o640); writeErr != nil {
		return fmt.Errorf("запись конфига (%s): %w", c.Name(), writeErr)
	}
	if checkErr := c.CheckConfig(ctx, exectx.OS, configPath); checkErr != nil {
		return fmt.Errorf("валидация конфига (%s): %w", c.Name(), checkErr)
	}
	log.L().Info("конфиг записан", "core", c.Name(), "path", configPath)
	return nil
}

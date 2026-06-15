package xray

import (
	"context"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// coreImpl реализует core.Core для xray.
//
// Адаптер xray-core. Реализует core.Core: paths, version, lifecycle,
// config render (JSON), download, install. Активируется через --core xray
// или auto-detect по proxy URL (Vision UDP443 / PQ-VLESS).
type coreImpl struct{}

func (coreImpl) Name() string                    { return "xray" }
func (coreImpl) BinaryPath() string              { return DefaultBinPath }
func (coreImpl) ConfigDir() string               { return DefaultConfigDir }
func (coreImpl) CacheDir() string                { return DefaultCacheDir }
func (coreImpl) ConfigPath() string              { return DefaultConfigPath }
func (coreImpl) PIDPath() string                 { return DefaultPIDFile }
func (coreImpl) ConfigFormat() core.ConfigFormat { return core.ConfigJSON }
func (coreImpl) GeoFormat() core.GeoFormat       { return core.GeoDAT }

func (coreImpl) NewLifecycle() service.Lifecycle {
	return DefaultLifecycle()
}

func (coreImpl) Download(ctx context.Context, arch types.Arch, dstDir string) (core.DownloadResult, error) {
	return Download(ctx, arch, dstDir)
}

func (coreImpl) Install(ctx context.Context, runner exectx.Runner, archivePath string) error {
	return Install(ctx, runner, archivePath, DefaultBinPath, "")
}

func (coreImpl) CheckConfig(ctx context.Context, runner exectx.Runner, configPath string) error {
	return CheckConfig(ctx, runner, DefaultBinPath, configPath)
}

func (coreImpl) BinaryVersion(ctx context.Context, runner exectx.Runner) (string, error) {
	return BinaryVersion(ctx, runner, DefaultBinPath)
}

func (coreImpl) ParseVersion(line string) (string, error) {
	return parseVersion(line)
}

// RenderConfig генерирует config.json xray из types.CoreRenderParams.
// Canonical-данные берутся напрямую из Outbound.Protocol/Transport/TLS/Proto
// (Phase D.4.2: OutboundCanonicals параллельная карта упразднена).
//
// Если rp.RoutingConfig задан — он используется как источник outbounds/rules
// (приоритет над rp.Outbounds для outbound-списка, rp.Outbounds остаётся
// fallback для legacy CLI flows без routing.json).
func (coreImpl) RenderConfig(rp types.CoreRenderParams) ([]byte, error) {
	p := DefaultConfigParams()
	p.Outbounds = core.EffectiveOutbounds(rp)
	// Собираем Canonicals из inline-полей каждого Outbound.
	p.Canonicals = make(map[string]types.Canonical, len(p.Outbounds))
	for _, ob := range p.Outbounds {
		if ob.Protocol != "" {
			p.Canonicals[ob.Tag] = types.Canonical{
				Protocol:  ob.Protocol,
				Transport: ob.Transport,
				TLS:       ob.TLS,
				Proto:     ob.Proto,
			}
		}
	}
	p.DefaultTag = core.EffectiveDefaultTag(rp, p.Outbounds)
	p.RoutingConfig = rp.RoutingConfig
	return Render(p)
}

func (coreImpl) ValidateOutbound(ob types.Outbound) error {
	c := types.Canonical{
		Protocol:  ob.Protocol,
		Transport: ob.Transport,
		TLS:       ob.TLS,
		Proto:     ob.Proto,
	}
	return Validate(c)
}

func init() {
	core.Register(coreImpl{})
}

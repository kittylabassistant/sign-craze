package mihomo

import (
	"context"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// coreImpl реализует core.Core для mihomo (Clash.Meta).
type coreImpl struct{}

func (coreImpl) Name() string                    { return "mihomo" }
func (coreImpl) BinaryPath() string              { return DefaultBinPath }
func (coreImpl) ConfigDir() string               { return DefaultConfigDir }
func (coreImpl) CacheDir() string                { return DefaultCacheDir }
func (coreImpl) ConfigPath() string              { return DefaultConfigPath }
func (coreImpl) PIDPath() string                 { return DefaultPIDFile }
func (coreImpl) ConfigFormat() core.ConfigFormat { return core.ConfigYAML }
func (coreImpl) GeoFormat() core.GeoFormat       { return core.GeoMRS }

func (coreImpl) NewLifecycle() service.Lifecycle {
	return DefaultLifecycle()
}

func (coreImpl) CheckConfig(ctx context.Context, runner exectx.Runner, configPath string) error {
	return CheckConfig(ctx, runner, DefaultBinPath, configPath)
}

func (coreImpl) Download(ctx context.Context, arch types.Arch, dstDir string) (core.DownloadResult, error) {
	res, err := Download(ctx, arch, dstDir)
	if err != nil {
		return core.DownloadResult{}, err
	}
	return core.DownloadResult{
		Downloaded: res.Downloaded,
		Version:    res.Version,
		Path:       res.Path,
	}, nil
}

func (coreImpl) Install(ctx context.Context, runner exectx.Runner, archivePath string) error {
	return Install(ctx, runner, archivePath, DefaultBinPath, "")
}

func (coreImpl) BinaryVersion(ctx context.Context, runner exectx.Runner) (string, error) {
	return BinaryVersion(ctx, runner, DefaultBinPath)
}

func (coreImpl) ParseVersion(line string) (string, error) {
	return parseVersion(line)
}

// RenderConfig генерирует config.yaml mihomo из types.CoreRenderParams.
// Canonical-данные берутся напрямую из Outbound.Protocol/Transport/TLS/Proto
// (Phase D.4.2: OutboundCanonicals параллельная карта упразднена).
//
// Если rp.RoutingConfig задан — его Outbounds/Rules/RuleSets используются
// для генерации соответствующих YAML-секций (proxies/rules/rule-providers).
// Иначе Render выпускает legacy "MATCH,DefaultTag" rule с одним proxy-group.
func (coreImpl) RenderConfig(rp types.CoreRenderParams) ([]byte, error) {
	p := DefaultConfigParams()
	p.Outbounds = effectiveOutbounds(rp)
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
	p.DefaultTag = effectiveDefaultTag(rp, p.Outbounds)
	p.RoutingConfig = rp.RoutingConfig
	return Render(p)
}

// effectiveOutbounds возвращает actual outbound-список для рендера.
// Приоритет: RoutingConfig.Outbounds (если непустой) → rp.Outbounds.
func effectiveOutbounds(rp types.CoreRenderParams) []types.Outbound {
	if rp.RoutingConfig != nil && len(rp.RoutingConfig.Outbounds) > 0 {
		return rp.RoutingConfig.Outbounds
	}
	return rp.Outbounds
}

// effectiveDefaultTag — final outbound tag. Приоритет:
// rp.DefaultOutboundTag → RoutingConfig.Final → первый outbound.
func effectiveDefaultTag(rp types.CoreRenderParams, outs []types.Outbound) string {
	if rp.DefaultOutboundTag != "" {
		return rp.DefaultOutboundTag
	}
	if rp.RoutingConfig != nil && rp.RoutingConfig.Final != "" {
		return rp.RoutingConfig.Final
	}
	if len(outs) > 0 {
		return outs[0].Tag
	}
	return ""
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

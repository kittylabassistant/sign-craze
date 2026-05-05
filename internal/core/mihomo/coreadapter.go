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
func (coreImpl) RenderConfig(rp types.CoreRenderParams) ([]byte, error) {
	p := DefaultConfigParams()
	p.Outbounds = rp.Outbounds
	// Собираем Canonicals из inline-полей каждого Outbound.
	p.Canonicals = make(map[string]types.Canonical, len(rp.Outbounds))
	for _, ob := range rp.Outbounds {
		if ob.Protocol != "" {
			p.Canonicals[ob.Tag] = types.Canonical{
				Protocol:  ob.Protocol,
				Transport: ob.Transport,
				TLS:       ob.TLS,
				Proto:     ob.Proto,
			}
		}
	}
	if len(rp.Outbounds) > 0 {
		p.DefaultTag = rp.Outbounds[0].Tag
	}
	return Render(p)
}

func init() {
	core.Register(coreImpl{})
}

package singbox

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// coreImpl реализует core.Core поверх существующих функций пакета singbox.
//
// Phase A адаптер: код пакета singbox остаётся на месте; coreImpl делегирует.
// Phase A.5 (по плану) переместит код в internal/core/singbox/ и упразднит
// этот wrapper. До тех пор state.Core="sing-box" разрешается через registry
// в этот тип, что позволяет CLI диспетчеризовать без прямой зависимости
// на пакет singbox.
type coreImpl struct{}

func (coreImpl) Name() string                    { return "sing-box" }
func (coreImpl) BinaryPath() string              { return DefaultBinPath }
func (coreImpl) ConfigDir() string               { return DefaultConfigDir }
func (coreImpl) CacheDir() string                { return DefaultCacheDir }
func (coreImpl) ConfigPath() string              { return DefaultConfigDir + "/config.json" }
func (coreImpl) PIDPath() string                 { return DefaultPIDFile }
func (coreImpl) ConfigFormat() core.ConfigFormat { return core.ConfigJSON }
func (coreImpl) GeoFormat() core.GeoFormat       { return core.GeoSRS }

func (coreImpl) NewLifecycle() service.Lifecycle {
	return DefaultLifecycle()
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

// Install устанавливает sing-box без валидации конфига. Caller обязан
// вызвать CheckConfig отдельно после WriteConfig, либо использовать
// PrepareAndValidate (валидация ДО подмены бинаря, для install/update flow).
func (coreImpl) Install(_ context.Context, _ exectx.Runner, archivePath string) error {
	log.L().Info("установка sing-box", "tarball", archivePath, "dst", DefaultBinPath)
	stream, err := openSingboxBinaryStream(archivePath)
	if err != nil {
		return fmt.Errorf("singbox install: распаковка tarball: %w", err)
	}
	defer stream.Close()
	if _, err := atomicfs.BackupAndReplaceFromReader(DefaultBinPath, stream.Reader, 0o755); err != nil {
		return fmt.Errorf("singbox install: запись бинаря: %w", err)
	}
	log.L().Info("sing-box установлен", "path", DefaultBinPath)
	return nil
}

func (coreImpl) CheckConfig(ctx context.Context, runner exectx.Runner, configPath string) error {
	return checkConfig(ctx, runner, DefaultBinPath, configPath)
}

func (coreImpl) BinaryVersion(ctx context.Context, runner exectx.Runner) (string, error) {
	return BinaryVersion(ctx, runner, DefaultBinPath)
}

func (coreImpl) ParseVersion(line string) (string, error) {
	return parseVersion(line)
}

// RenderConfig генерирует config.json sing-box из types.CoreRenderParams.
//
// Перед рендером проверяет каждый outbound на совместимость с sing-box через
// Validate — xray-only возможности (XHTTP modes, PQ-VLESS) отклоняются с
// ошибкой до запуска sing-box check, что даёт понятную диагностику.
// Reality spider_x (spx=...) поддерживается только в xray, sing-box render
// его молча игнорирует (см. render.go).
//
// Прокидывает все поля CoreRenderParams в singbox.ConfigParams: RoutingConfig
// (пользовательский routing.json), InboundMode (tun/tproxy), DefaultOutboundTag.
// При InboundMode="tproxy" автоматически выполняет probeTProxyKernel() для
// корректного выбора tproxy vs redirect inbound.
func (coreImpl) RenderConfig(p types.CoreRenderParams) ([]byte, error) {
	for _, ob := range p.Outbounds {
		if err := Validate(ob); err != nil {
			return nil, err
		}
	}
	params := DefaultConfigParams()
	params.Mode = p.Mode
	params.Outbounds = p.Outbounds
	params.RoutingConfig = p.RoutingConfig
	params.InboundMode = p.InboundMode
	params.DefaultOutboundTag = p.DefaultOutboundTag
	if params.DefaultOutboundTag == "" && len(p.Outbounds) > 0 {
		params.DefaultOutboundTag = p.Outbounds[0].Tag
	}
	if params.InboundMode == "tproxy" {
		params.TProxyKernelOK = probeTProxyKernel()
	}
	return Render(params)
}

func (coreImpl) ValidateOutbound(ob types.Outbound) error {
	return Validate(ob)
}

func init() {
	core.Register(coreImpl{})
}

package web

import (
	"context"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// RoutingUIDeps — зависимости routing-UI handlers. Заполняется в cli/deps.go
// при boot'е (Phase 3.1) и передаётся через ServerConfig.RoutingUI.
type RoutingUIDeps struct {
	// RoutingPath — путь к routing.json (default routing.DefaultPath).
	RoutingPath string

	// DefaultOutboundTag — fallback для DNS detour и final outbound, если RoutingConfig.Final пуст.
	// Берётся из state.Outbounds[0].Tag.
	DefaultOutboundTag func() string

	// Renderer — рендерит RoutingConfig в native-формат активного ядра
	// (sing-box JSON / xray JSON / mihomo YAML). Используется apiPreview.
	// Закрытие re-resolve'ит активное ядро на каждом вызове — переключение
	// state.Core без рестарта UI работает.
	Renderer func(ctx context.Context, cfg *types.RoutingConfig) ([]byte, error)

	// ConfigFormat возвращает формат рендера активного ядра (JSON или YAML).
	// Используется apiPreview для выставления корректного Content-Type
	// (application/json vs application/yaml). Может быть nil — fallback JSON.
	ConfigFormat func() core.ConfigFormat

	// Validator проверяет RoutingConfig через активное ядро: рендерит, пишет
	// в tmp-файл, вызывает c.CheckConfig. Возвращает warnings (несовместимые
	// matchers, TUN inbound на xray/mihomo и т.д.) и fatal error если рендер
	// или check не прошли. Заменяет Renderer-based валидацию (которая всегда
	// использовала sing-box даже для xray/mihomo).
	Validator func(ctx context.Context, cfg *types.RoutingConfig) (warnings []string, err error)

	// ActiveGeoFormat возвращает требуемый формат rule-set для активного ядра
	// (SRS/DAT/MRS). Используется apiPresetsApply для резолва per-core URL
	// в translation table.
	ActiveGeoFormat func() core.GeoFormat

	// OnApply вызывается после успешного routing.Save в /api/apply.
	// Может регенерировать config.json (через cli.regenerateConfig).
	// Может быть nil — тогда ответ просто {needs_restart: true}.
	OnApply func(ctx context.Context) error

	// OnRestart вызывается после OnApply для перезапуска core-сервиса.
	// Если nil — возвращается {needs_restart: true}, пользователь перезапускает вручную.
	OnRestart func(ctx context.Context) error
}

package web

import (
	"context"

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

	// Renderer — вызывает singbox.Render для preview/validate.
	// Возвращает rendered config.json или ошибку.
	Renderer func(ctx context.Context, cfg *types.RoutingConfig) ([]byte, error)

	// OnApply вызывается после успешного routing.Save в /api/apply.
	// Может регенерировать config.json (через cli.regenerateConfig).
	// Может быть nil — тогда ответ просто {needs_restart: true}.
	OnApply func(ctx context.Context) error

	// OnRestart вызывается после OnApply для перезапуска core-сервиса.
	// Если nil — возвращается {needs_restart: true}, пользователь перезапускает вручную.
	OnRestart func(ctx context.Context) error
}

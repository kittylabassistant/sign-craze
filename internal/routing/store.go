// Package routing управляет персистентной routing-конфигурацией для UI-редактора.
package routing

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// BootstrapState — минимальный интерфейс state, необходимый для начальной
// инициализации routing.json. Позволяет избежать циклической зависимости
// между пакетами routing и state.
type BootstrapState interface {
	GetOutbounds() []types.Outbound
}

// DefaultPath — стандартный путь к файлу routing-конфигурации.
const DefaultPath = "/opt/etc/sign-craze/routing.json"

// SchemaVersion — текущая версия схемы routing.json.
const SchemaVersion = 1

// Load читает routing.json по указанному пути.
// Если файл отсутствует — возвращает (nil, nil) как сигнал использовать
// legacy RoutingRules. Невалидный JSON или ошибка Validate() → error.
func Load(path string) (*types.RoutingConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("routing: чтение %s: %w", path, err)
	}

	var c types.RoutingConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("routing: парсинг %s: %w", path, err)
	}
	// Миграция legacy-файлов: если version отсутствует или равен 0 (записано
	// предыдущей версией sign-craze без поля version), проставляем текущую схему.
	if c.Version == 0 {
		c.Version = SchemaVersion
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("routing: валидация %s: %w", path, err)
	}
	return &c, nil
}

// Save атомарно записывает RoutingConfig в файл с правами 0o640.
// Перед записью выполняется валидация конфига.
func Save(path string, c *types.RoutingConfig) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("routing: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("routing: marshal: %w", err)
	}
	if err := atomicfs.WriteFileAtomic(path, data, 0o640); err != nil {
		return fmt.Errorf("routing: запись %s: %w", path, err)
	}
	return nil
}

// FilterUnusedRuleSets удаляет из RoutingConfig.RuleSets те записи,
// чей Tag не упоминается ни в одном из RoutingConfig.Rules[].RuleSet.
// Фильтрует ТОЛЬКО по route rules (не dns.rules).
// Мутирует поле RuleSets переданного конфига на месте (nil-safe: nil-вход — no-op).
func FilterUnusedRuleSets(c *types.RoutingConfig) {
	if c == nil || len(c.RuleSets) == 0 {
		return
	}
	// собираем все теги, на которые есть ссылки из rules[]
	referenced := make(map[string]bool)
	for _, r := range c.Rules {
		for _, tag := range r.RuleSet {
			referenced[tag] = true
		}
	}
	// фильтруем RuleSets: оставляем только referenced
	filtered := c.RuleSets[:0]
	for _, rs := range c.RuleSets {
		if referenced[rs.Tag] {
			filtered = append(filtered, rs)
		}
	}
	c.RuleSets = filtered
}

// Default возвращает пустой RoutingConfig с Version=SchemaVersion.
func Default() *types.RoutingConfig {
	return &types.RoutingConfig{
		Version: SchemaVersion,
	}
}

// defaultTunInbound — стандартный tun-inbound, добавляемый при bootstrap.
// Параметры соответствуют шаблону sing-box tun в sign-craze.
var defaultTunInbound = types.Inbound{
	Tag:  "tun-in",
	Type: "tun",
	Settings: map[string]any{
		"interface_name": "signbox-tun",
		"address":        []string{"172.19.0.1/30"},
		"auto_route":     true,
		"stack":          "system",
	},
}

// BootstrapFromState создаёт routing.json по пути rcPath на основе state,
// если файл ещё не существует. Если файл уже есть — no-op.
// Генерирует минимальный RoutingConfig:
//   - Inbounds: один tun-inbound (signbox-tun)
//   - Outbounds: копия outbounds из state
//   - Rules: пустой список
//   - RuleSets: пустой список
//   - Final: тег первого outbound (если есть)
func BootstrapFromState(rcPath string, st BootstrapState) error {
	// Проверяем существование файла; если есть — ничего не делаем.
	if _, err := os.Stat(rcPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("routing: bootstrap stat %s: %w", rcPath, err)
	}

	outbounds := st.GetOutbounds()

	// Определяем final tag — тег первого outbound в списке.
	final := ""
	if len(outbounds) > 0 {
		final = outbounds[0].Tag
	}

	c := &types.RoutingConfig{
		Version:   SchemaVersion,
		Inbounds:  []types.Inbound{defaultTunInbound},
		Outbounds: append([]types.Outbound(nil), outbounds...),
		Rules:     []types.RouteRule{},
		RuleSets:  []types.RuleSetRef{},
		Final:     final,
	}

	return Save(rcPath, c)
}

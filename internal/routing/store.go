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

// Default возвращает пустой RoutingConfig с Version=SchemaVersion.
func Default() *types.RoutingConfig {
	return &types.RoutingConfig{
		Version: SchemaVersion,
	}
}

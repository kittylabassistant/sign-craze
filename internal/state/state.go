package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// DefaultPath — стандартное расположение state.json.
const DefaultPath = "/opt/etc/sign-craze/state.json"

// State — персистентное состояние sign-craze.
type State struct {
	Mode        types.Mode       `json:"mode"`
	Outbounds   []types.Outbound `json:"outbounds"`
	Ports       []uint16         `json:"ports"`
	Excludes    []string         `json:"excludes"`
	DPIEnabled  bool             `json:"dpi_enabled"`
	DPIStrategy string           `json:"dpi_strategy,omitempty"`
}

// Default возвращает state с настройками по умолчанию: режим proxy, stub direct outbound.
func Default() *State {
	return &State{
		Mode: types.ModeProxy,
		Outbounds: []types.Outbound{
			{Tag: "direct", Type: "direct"},
		},
		Ports:    []uint16{},
		Excludes: []string{},
	}
}

// Load читает state.json. Если файл отсутствует — возвращает Default().
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return nil, fmt.Errorf("state: чтение %s: %w", path, err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("state: парсинг %s: %w", path, err)
	}
	if s.Outbounds == nil {
		s.Outbounds = []types.Outbound{}
	}
	if s.Ports == nil {
		s.Ports = []uint16{}
	}
	if s.Excludes == nil {
		s.Excludes = []string{}
	}
	return &s, nil
}

// Save атомарно записывает state в файл с правами 0o600 (содержит outbound credentials).
func Save(path string, s *State) error {
	if s == nil {
		return fmt.Errorf("state: nil State")
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}
	if err := atomicfs.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("state: запись %s: %w", path, err)
	}
	return nil
}

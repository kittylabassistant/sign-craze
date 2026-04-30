package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// DefaultPolicyName — имя IP-policy в Keenetic RCI для режима ModePolicy.
const DefaultPolicyName = "sign-craze"

// DefaultPath — стандартное расположение state.json.
const DefaultPath = "/opt/etc/sign-craze/state.json"

// DefaultExcludes — безопасные CIDR-исключения, добавляемые в Default().
// Покрывают: loopback, link-local, multicast, зарезервированные, RFC1918.
// Защищают от потери LAN-доступа и SSH-lockout при первом запуске,
// если в signcraze_ipv4 окажется широкий префикс (см. safety-fixes.md #2).
var DefaultExcludes = []string{
	"127.0.0.0/8",    // loopback
	"169.254.0.0/16", // link-local
	"224.0.0.0/4",    // multicast
	"240.0.0.0/4",    // reserved
	"10.0.0.0/8",     // RFC1918
	"172.16.0.0/12",  // RFC1918
	"192.168.0.0/16", // RFC1918
}

// DefaultAdminPort — порт SSH/админки, исключаемый из проксирования по умолчанию.
const DefaultAdminPort uint16 = 22

// State — персистентное состояние sign-craze.
type State struct {
	Mode           types.Mode       `json:"mode"`
	Outbounds      []types.Outbound `json:"outbounds"`
	Ports          []uint16         `json:"ports"`
	Excludes       []string         `json:"excludes"`
	AdminPort      uint16           `json:"admin_port,omitempty"`
	AdminIPs       []string         `json:"admin_ips,omitempty"`
	DPIEnabled     bool             `json:"dpi_enabled"`
	DPIStrategy    string           `json:"dpi_strategy,omitempty"`
	BootTimeoutSec int              `json:"boot_timeout_sec,omitempty"` // таймаут waitDefaultRoute, 0 = default 60

	// Поля режима ModePolicy: интеграция с Keenetic IP Policy через RCI.
	// PolicyMark и PolicyTable — кеш runtime-значений, актуальные читаются
	// при каждом --start через ndm.GetPolicy().
	PolicyName   string `json:"policy_name,omitempty"`   // имя policy в RCI, default "sign-craze"
	PolicyMark   uint32 `json:"policy_mark,omitempty"`   // присвоенный Keenetic'ом fwmark (cache)
	PolicyTable  int    `json:"policy_table,omitempty"`  // routing table policy в Keenetic (cache, IPv4)
	WANInterface string `json:"wan_interface,omitempty"` // Keenetic-имя WAN, e.g. GigabitEthernet1
}

// Default возвращает state с настройками по умолчанию: режим policy, stub direct outbound,
// безопасные локальные исключения и admin port 22.
func Default() *State {
	excludes := make([]string, len(DefaultExcludes))
	copy(excludes, DefaultExcludes)
	return &State{
		Mode: types.ModePolicy,
		Outbounds: []types.Outbound{
			{Tag: "direct", Type: "direct"},
		},
		Ports:      []uint16{},
		Excludes:   excludes,
		AdminPort:  DefaultAdminPort,
		AdminIPs:   []string{},
		PolicyName: DefaultPolicyName,
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
	migrateLegacyMode(&s)
	if s.Outbounds == nil {
		s.Outbounds = []types.Outbound{}
	}
	if s.Ports == nil {
		s.Ports = []uint16{}
	}
	if s.Excludes == nil {
		s.Excludes = []string{}
	}
	if s.AdminIPs == nil {
		s.AdminIPs = []string{}
	}
	if s.PolicyName == "" {
		s.PolicyName = DefaultPolicyName
	}
	return &s, nil
}

// migrateLegacyMode конвертирует legacy-режимы (proxy/dpi/hybrid) в ModePolicy
// и пишет WARN в лог. Подсказка про --mode full даётся, чтобы пользователь
// мог вернуть старое поведение, если сознательно его выбирал.
func migrateLegacyMode(s *State) {
	if newMode, ok := types.LegacyModes[s.Mode]; ok {
		log.L().Warn("state: режим устарел, переключено в policy",
			"old_mode", string(s.Mode),
			"new_mode", string(newMode),
			"hint", "для возврата старого поведения: sign-craze --mode full --restart",
		)
		s.Mode = newMode
	}
}

// Validate проверяет корректность критичных полей State.
// Нулевые значения для optional-полей (BootTimeoutSec=0, AdminPort=0) допустимы,
// но если AdminPort задан явно — должен быть в диапазоне 1-65535.
func (s *State) Validate() error {
	if s == nil {
		return fmt.Errorf("state: nil State")
	}
	if s.Mode != "" {
		if err := s.Mode.Validate(); err != nil {
			return err
		}
	}
	// AdminPort=0 трактуем как «выключено» (не вставляем bypass-правило);
	// явное значение должно быть валидным портом. Проверяем верхнюю границу
	// (uint16 не может быть >65535, но Validate() остаётся компактной защитой
	// на случай миграции типа поля).
	for _, p := range s.Ports {
		if p == 0 {
			return fmt.Errorf("state: ports содержит 0 (некорректный порт)")
		}
	}
	return nil
}

// Save атомарно записывает state в файл с правами 0o600 (содержит outbound credentials).
// Перед записью валидирует State — некорректное состояние не должно попадать на диск.
func Save(path string, s *State) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("state: %w", err)
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

package peer

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// MieruConfigDir — путь, в котором sign-craze хранит сгенерированные
// client.conf.json для каждого mieru-outbound'а.
const MieruConfigDir = "/opt/etc/sign-craze"

// MieruClientConfig — минимальная JSON-схема client config mieru v3.x.
//
// Полная схема (см. mieru/pkg/appctl/proto/clientcfg.proto) содержит десятки
// опциональных полей; здесь только то, что sign-craze заполняет из Canonical.
// Поля совпадают по именам с camelCase-маппингом protobuf — mieru CLI
// принимает JSON с этими именами через `mieru run -c <file>`.
type MieruClientConfig struct {
	Profiles      []MieruProfile `json:"profiles"`
	ActiveProfile string         `json:"activeProfile"`
	RPCPort       int            `json:"rpcPort"`
	Socks5Port    int            `json:"socks5Port"`
	// LoggingLevel опционально; INFO/WARN/ERROR. По умолчанию INFO.
	LoggingLevel string `json:"loggingLevel,omitempty"`
}

// MieruProfile — конфигурация одного профиля.
type MieruProfile struct {
	ProfileName  string             `json:"profileName"`
	User         MieruUser          `json:"user"`
	Servers      []MieruServer      `json:"servers"`
	Multiplexing *MieruMultiplexing `json:"multiplexing,omitempty"`
}

// MieruUser — credentials профиля.
type MieruUser struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// MieruServer — один сервер с биндингами портов.
type MieruServer struct {
	// IPAddress используется когда Server — литерал IP; иначе DomainName.
	IPAddress    string             `json:"ipAddress,omitempty"`
	DomainName   string             `json:"domainName,omitempty"`
	PortBindings []MieruPortBinding `json:"portBindings"`
}

// MieruPortBinding — порт + транспортный протокол сервера.
type MieruPortBinding struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // TCP | UDP
}

// MieruMultiplexing — уровень мультиплексирования.
type MieruMultiplexing struct {
	Level string `json:"level"` // MULTIPLEXING_OFF|LOW|MIDDLE|HIGH
}

// MieruConfigPath возвращает абсолютный путь к client.conf.json для outbound.
// Используется как для записи, так и для CLI mieru-клиента (--config).
func MieruConfigPath(dir string, tag string) string {
	return filepath.Join(dir, "mieru-"+tag+".conf.json")
}

// rpcPortForOutbound вычисляет RPC-порт mieru-клиента так, чтобы он не
// конфликтовал с SOCKS5-портом (LocalPort + 1). RPC mieru используется
// командой `mieru status` и должен слушать на 127.0.0.1.
func rpcPortForOutbound(localPort int) int {
	return localPort + 1
}

// BuildMieruClientConfig собирает структуру конфига mieru-клиента из
// canonical-полей outbound. Не записывает на диск — отдельный шаг WriteMieruConfig.
//
// Требования к outbound:
//   - Protocol == ProtocolMieru
//   - Proto != nil, MieruUsername/MieruPassword/MieruPort заданы
//   - MieruLocalPort выделен через peer.AllocateMieruPort (> 0)
//   - Server (или PortBindings cnt 1) задан
func BuildMieruClientConfig(ob types.Outbound) (MieruClientConfig, error) {
	if ob.Protocol != types.ProtocolMieru {
		return MieruClientConfig{}, fmt.Errorf("peer mieru: outbound %q не mieru (protocol=%q)", ob.Tag, ob.Protocol)
	}
	if ob.Proto == nil {
		return MieruClientConfig{}, fmt.Errorf("peer mieru: outbound %q: Proto = nil", ob.Tag)
	}
	if ob.Proto.MieruUsername == "" || ob.Proto.MieruPassword == "" {
		return MieruClientConfig{}, fmt.Errorf("peer mieru: outbound %q: username/password пустые", ob.Tag)
	}
	if ob.Proto.MieruLocalPort <= 0 {
		return MieruClientConfig{}, fmt.Errorf("peer mieru: outbound %q: MieruLocalPort не выделен (запустите AllocateMieruPort)", ob.Tag)
	}
	if ob.Server == "" {
		return MieruClientConfig{}, fmt.Errorf("peer mieru: outbound %q: пустой Server", ob.Tag)
	}

	port := ob.Proto.MieruPort
	if port == 0 {
		port = int(ob.Port)
	}
	if port == 0 {
		return MieruClientConfig{}, fmt.Errorf("peer mieru: outbound %q: отсутствует удалённый порт", ob.Tag)
	}

	proto := ob.Proto.MieruProtocol
	if proto == "" {
		proto = "TCP"
	}

	server := MieruServer{
		PortBindings: []MieruPortBinding{{Port: port, Protocol: proto}},
	}
	if isIPLiteral(ob.Server) {
		server.IPAddress = ob.Server
	} else {
		server.DomainName = ob.Server
	}

	profileName := ob.Proto.MieruProfile
	if profileName == "" {
		profileName = "default"
	}

	profile := MieruProfile{
		ProfileName: profileName,
		User: MieruUser{
			Name:     ob.Proto.MieruUsername,
			Password: ob.Proto.MieruPassword,
		},
		Servers: []MieruServer{server},
	}
	if ob.Proto.MieruMultiplexing != "" {
		profile.Multiplexing = &MieruMultiplexing{Level: ob.Proto.MieruMultiplexing}
	}

	return MieruClientConfig{
		Profiles:      []MieruProfile{profile},
		ActiveProfile: profileName,
		RPCPort:       rpcPortForOutbound(ob.Proto.MieruLocalPort),
		Socks5Port:    ob.Proto.MieruLocalPort,
		LoggingLevel:  "WARN",
	}, nil
}

// WriteMieruConfig сериализует MieruClientConfig для outbound и атомарно
// пишет в `<dir>/mieru-<tag>.conf.json` с правами 0o600 (содержит password).
// Возвращает абсолютный путь к записанному файлу.
func WriteMieruConfig(dir string, ob types.Outbound) (string, error) {
	cfg, err := BuildMieruClientConfig(ob)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("peer mieru: marshal: %w", err)
	}
	path := MieruConfigPath(dir, ob.Tag)
	if err := atomicfs.WriteFileAtomic(path, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("peer mieru: запись %s: %w", path, err)
	}
	return path, nil
}

// isIPLiteral возвращает true если s выглядит как IPv4-литерал (мы пишем
// его в ipAddress, а не domainName). IPv6 пока не поддерживается mieru-сервером
// в первой части интеграции — на v0.X.Y сервера обычно за IPv4.
func isIPLiteral(s string) bool {
	dotCount := 0
	for _, c := range s {
		switch {
		case c == '.':
			dotCount++
		case c >= '0' && c <= '9':
		case c == ':':
			// IPv6 → не IP-literal для mieru (используем domainName)
			return false
		default:
			return false
		}
	}
	return dotCount == 3
}

package proxyparse

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// canonicalMieruSimple парсит human-readable URL формата:
//
//	mierus://<user>:<pass>@<host>?port=<N>&protocol=TCP|UDP&multiplexing=<level>&profile=<name>#<tag>
//
// Обязательные поля: user, pass, host, port. Опциональные: protocol (TCP),
// multiplexing (MULTIPLEXING_LOW), profile (default), tag (mieru-out).
//
// Формат соответствует выводу команды `mieru export config simple` mieru-клиента.
// Server/Port в Outbound — это удалённый адрес mieru-сервера, MieruLocalPort
// остаётся 0 (выделяется при первом --start через peer.AllocateMieruPort).
func canonicalMieruSimple(s string) (types.Outbound, types.Canonical, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: %w", err)
	}
	if u.Host == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: отсутствует host")
	}
	if u.User == nil || u.User.Username() == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: отсутствует username")
	}
	pass, hasPass := u.User.Password()
	if !hasPass || pass == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: отсутствует password")
	}

	host := u.Hostname()
	q := u.Query()

	portStr := q.Get("port")
	if portStr == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: query-параметр port обязателен")
	}
	port, err := parsePort(portStr)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: port: %w", err)
	}

	proto, err := mieruProtocolOrDefault(q.Get("protocol"))
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: %w", err)
	}

	mux, err := mieruMultiplexingOrDefault(q.Get("multiplexing"))
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: %w", err)
	}

	profile := q.Get("profile")
	if profile == "" {
		profile = "default"
	}

	canon := types.Canonical{
		Protocol: types.ProtocolMieru,
		Proto: &types.ProtoOpts{
			MieruUsername:     u.User.Username(),
			MieruPassword:     pass,
			MieruPort:         int(port),
			MieruProtocol:     proto,
			MieruMultiplexing: mux,
			MieruProfile:      profile,
			// MieruLocalPort=0 — выделяется при --start.
		},
	}

	ob := types.Outbound{
		Tag:    tagFromFragment(u, "mieru-out"),
		Type:   "socks", // sing-box видит mieru как socks bridge на 127.0.0.1:LocalPort
		Server: host,
		Port:   port,
	}

	return ob, canon, nil
}

// canonicalMieruProto парсит base64-encoded protobuf формата:
//
//	mieru://<base64-urlsafe-protobuf>
//
// Это полный protobuf-config mieru-клиента, экспортированный командой
// `mieru export config`. Парсится через ручной wire-format декодер
// (без зависимости от google.golang.org/protobuf — избегаем bloat'а main binary).
//
// Извлекаются только минимально необходимые поля:
//
//	profiles[0].profileName        → MieruProfile
//	profiles[0].user.name          → MieruUsername
//	profiles[0].user.password      → MieruPassword
//	profiles[0].servers[0].ipAddress|domainName → Server
//	profiles[0].servers[0].portBindings[0].port     → Port (и MieruPort)
//	profiles[0].servers[0].portBindings[0].protocol → MieruProtocol
//	profiles[0].multiplexing.level → MieruMultiplexing
//
// Поля верхнего уровня (socks5Port, rpcPort, activeProfile) игнорируются —
// они не нужны sign-craze, так как мы сами выделяем MieruLocalPort.
//
// Wire-format упрощения:
//   - Поддерживается только одиночный профиль (profiles[0]) и один сервер.
//   - Tag-numbers ≤ 15 (один байт) — этого достаточно для основных полей mieru-схемы.
//   - Не валидируется CRC, не разбираются packed-repeated.
//
// Если структура не соответствует ожидаемой schema mieru — возвращается
// понятная ошибка вместо panic.
func canonicalMieruProto(s string) (types.Outbound, types.Canonical, error) {
	const prefix = "mieru://"
	if !strings.HasPrefix(s, prefix) {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: неверный префикс")
	}
	body := strings.TrimSpace(s[len(prefix):])
	// fragment — после '#'
	tag := ""
	if hash := strings.LastIndex(body, "#"); hash >= 0 {
		if dec, e := url.QueryUnescape(body[hash+1:]); e == nil {
			tag = dec
		} else {
			tag = body[hash+1:]
		}
		body = body[:hash]
	}
	if body == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: пустой base64 после префикса")
	}

	raw, err := decodeMieruBase64(body)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: base64: %w", err)
	}

	cfg, err := parseMieruWireConfig(raw)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: wire: %w", err)
	}

	if cfg.Username == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: protobuf не содержит username")
	}
	if cfg.Password == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: protobuf не содержит password")
	}
	if cfg.Server == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: protobuf не содержит server address")
	}
	if cfg.Port == 0 {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: protobuf не содержит port")
	}

	proto, err := mieruProtocolOrDefault(cfg.Protocol)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: %w", err)
	}
	mux, err := mieruMultiplexingOrDefault(cfg.Multiplexing)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse mieru: %w", err)
	}

	profile := cfg.Profile
	if profile == "" {
		profile = "default"
	}

	if tag == "" {
		tag = "mieru-out"
	}

	canon := types.Canonical{
		Protocol: types.ProtocolMieru,
		Proto: &types.ProtoOpts{
			MieruUsername:     cfg.Username,
			MieruPassword:     cfg.Password,
			MieruPort:         int(cfg.Port),
			MieruProtocol:     proto,
			MieruMultiplexing: mux,
			MieruProfile:      profile,
		},
	}

	ob := types.Outbound{
		Tag:    tag,
		Type:   "socks",
		Server: cfg.Server,
		Port:   types.Port(cfg.Port),
	}

	return ob, canon, nil
}

// mieruProtocolOrDefault нормализует строку протокола.
// Допустимые: пустая (→ TCP по умолчанию), "TCP", "tcp", "UDP", "udp".
// Числовые wire-значения mieru (1=TCP, 2=UDP) тоже принимаются.
func mieruProtocolOrDefault(v string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "", "TCP", "1":
		return "TCP", nil
	case "UDP", "2":
		return "UDP", nil
	default:
		return "", fmt.Errorf("неподдерживаемый protocol=%q (ожидалось TCP|UDP)", v)
	}
}

// mieruMultiplexingOrDefault нормализует уровень мультиплексирования.
// Допустимые: пустая (→ MULTIPLEXING_LOW), MULTIPLEXING_OFF|LOW|MIDDLE|HIGH,
// а также enum-числа mieru (0..4).
func mieruMultiplexingOrDefault(v string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "":
		return "MULTIPLEXING_LOW", nil
	case "MULTIPLEXING_DEFAULT", "0":
		return "MULTIPLEXING_LOW", nil
	case "MULTIPLEXING_OFF", "1":
		return "MULTIPLEXING_OFF", nil
	case "MULTIPLEXING_LOW", "2":
		return "MULTIPLEXING_LOW", nil
	case "MULTIPLEXING_MIDDLE", "3":
		return "MULTIPLEXING_MIDDLE", nil
	case "MULTIPLEXING_HIGH", "4":
		return "MULTIPLEXING_HIGH", nil
	default:
		return "", fmt.Errorf("неподдерживаемый multiplexing=%q (ожидалось MULTIPLEXING_OFF|LOW|MIDDLE|HIGH)", v)
	}
}

// decodeMieruBase64 пробует Std/URLEncoding с/без паддинга — mieru export config
// использует стандартный URL-safe base64 без паддинга, но допускаем и со звёздочкой.
func decodeMieruBase64(s string) ([]byte, error) {
	// Сначала URL-safe без паддинга (наиболее частый случай у mieru CLI).
	if v, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return v, nil
	}
	if v, err := base64.URLEncoding.DecodeString(s); err == nil {
		return v, nil
	}
	if v, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return v, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

// mieruWireCfg — извлечённые поля из protobuf-сообщения ClientConfig mieru.
type mieruWireCfg struct {
	Profile      string
	Username     string
	Password     string
	Server       string
	Port         uint32
	Protocol     string
	Multiplexing string
}

// parseMieruWireConfig разбирает protobuf wire-format ClientConfig.
//
// Используется минимальный декодер: проходит по top-level полям, извлекает
// первый профиль (`profiles[0]`, field-tag=1), внутри которого извлекает
// `profileName` (1), `user` (2), `servers[0]` (3), `multiplexing` (4).
// Все остальные поля пропускаются с корректным skip'ом по wire-type.
//
// Mieru ClientConfig schema (восстановлено из mieru v3.32.0):
//
//	message ClientConfig {
//	  repeated ClientProfile profiles = 1;
//	  string activeProfile = 2;
//	  int32 rpcPort = 3;
//	  int32 socks5Port = 4;
//	  ...
//	}
//	message ClientProfile {
//	  string profileName = 1;
//	  User user = 2;
//	  repeated ServerEndpoint servers = 3;
//	  MultiplexingConfig multiplexing = 4;
//	  ...
//	}
//	message User { string name = 1; string password = 2; }
//	message ServerEndpoint {
//	  oneof address { string ipAddress = 1; string domainName = 2; }
//	  repeated PortBinding portBindings = 3;
//	}
//	message PortBinding { int32 port = 1; TransportProtocol protocol = 2; }
//	message MultiplexingConfig { MultiplexingLevel level = 1; }
//
// Парсер устойчив к перестановке field-order и неизвестным полям.
func parseMieruWireConfig(data []byte) (mieruWireCfg, error) {
	var cfg mieruWireCfg
	var firstProfileFound bool

	for pos := 0; pos < len(data); {
		fieldNum, wireType, n, err := readTag(data[pos:])
		if err != nil {
			return cfg, fmt.Errorf("top-level tag at %d: %w", pos, err)
		}
		pos += n

		if fieldNum == 1 && wireType == wireBytes && !firstProfileFound {
			// profiles[0]
			profileBytes, m, err := readBytes(data[pos:])
			if err != nil {
				return cfg, fmt.Errorf("profiles[0]: %w", err)
			}
			if err := parseMieruProfile(profileBytes, &cfg); err != nil {
				return cfg, err
			}
			firstProfileFound = true
			pos += m
			continue
		}

		// Skip unknown / not-needed field
		skipN, err := skipWireValue(data[pos:], wireType)
		if err != nil {
			return cfg, fmt.Errorf("skip top-level field=%d wire=%d: %w", fieldNum, wireType, err)
		}
		pos += skipN
	}

	if !firstProfileFound {
		return cfg, fmt.Errorf("ClientConfig: profiles[0] не найден")
	}
	return cfg, nil
}

func parseMieruProfile(data []byte, cfg *mieruWireCfg) error {
	for pos := 0; pos < len(data); {
		fieldNum, wireType, n, err := readTag(data[pos:])
		if err != nil {
			return fmt.Errorf("profile tag at %d: %w", pos, err)
		}
		pos += n

		switch {
		case fieldNum == 1 && wireType == wireBytes: // profileName
			v, m, err := readString(data[pos:])
			if err != nil {
				return fmt.Errorf("profile.profileName: %w", err)
			}
			cfg.Profile = v
			pos += m
		case fieldNum == 2 && wireType == wireBytes: // user
			sub, m, err := readBytes(data[pos:])
			if err != nil {
				return fmt.Errorf("profile.user: %w", err)
			}
			if err := parseMieruUser(sub, cfg); err != nil {
				return err
			}
			pos += m
		case fieldNum == 3 && wireType == wireBytes && cfg.Server == "":
			// servers[0] — первый встретившийся
			sub, m, err := readBytes(data[pos:])
			if err != nil {
				return fmt.Errorf("profile.servers[0]: %w", err)
			}
			if err := parseMieruServer(sub, cfg); err != nil {
				return err
			}
			pos += m
		case fieldNum == 4 && wireType == wireBytes: // multiplexing
			sub, m, err := readBytes(data[pos:])
			if err != nil {
				return fmt.Errorf("profile.multiplexing: %w", err)
			}
			if err := parseMieruMux(sub, cfg); err != nil {
				return err
			}
			pos += m
		default:
			skipN, err := skipWireValue(data[pos:], wireType)
			if err != nil {
				return fmt.Errorf("profile skip field=%d wire=%d: %w", fieldNum, wireType, err)
			}
			pos += skipN
		}
	}
	return nil
}

func parseMieruUser(data []byte, cfg *mieruWireCfg) error {
	for pos := 0; pos < len(data); {
		fieldNum, wireType, n, err := readTag(data[pos:])
		if err != nil {
			return fmt.Errorf("user tag: %w", err)
		}
		pos += n
		switch {
		case fieldNum == 1 && wireType == wireBytes:
			v, m, err := readString(data[pos:])
			if err != nil {
				return fmt.Errorf("user.name: %w", err)
			}
			cfg.Username = v
			pos += m
		case fieldNum == 2 && wireType == wireBytes:
			v, m, err := readString(data[pos:])
			if err != nil {
				return fmt.Errorf("user.password: %w", err)
			}
			cfg.Password = v
			pos += m
		default:
			skipN, err := skipWireValue(data[pos:], wireType)
			if err != nil {
				return fmt.Errorf("user skip field=%d: %w", fieldNum, err)
			}
			pos += skipN
		}
	}
	return nil
}

func parseMieruServer(data []byte, cfg *mieruWireCfg) error {
	var firstPortBinding bool
	for pos := 0; pos < len(data); {
		fieldNum, wireType, n, err := readTag(data[pos:])
		if err != nil {
			return fmt.Errorf("server tag: %w", err)
		}
		pos += n
		switch {
		case (fieldNum == 1 || fieldNum == 2) && wireType == wireBytes:
			// oneof address: ipAddress (1) | domainName (2)
			v, m, err := readString(data[pos:])
			if err != nil {
				return fmt.Errorf("server.address: %w", err)
			}
			if cfg.Server == "" {
				cfg.Server = v
			}
			pos += m
		case fieldNum == 3 && wireType == wireBytes && !firstPortBinding:
			sub, m, err := readBytes(data[pos:])
			if err != nil {
				return fmt.Errorf("server.portBindings[0]: %w", err)
			}
			if err := parseMieruPortBinding(sub, cfg); err != nil {
				return err
			}
			firstPortBinding = true
			pos += m
		default:
			skipN, err := skipWireValue(data[pos:], wireType)
			if err != nil {
				return fmt.Errorf("server skip field=%d: %w", fieldNum, err)
			}
			pos += skipN
		}
	}
	return nil
}

func parseMieruPortBinding(data []byte, cfg *mieruWireCfg) error {
	for pos := 0; pos < len(data); {
		fieldNum, wireType, n, err := readTag(data[pos:])
		if err != nil {
			return fmt.Errorf("portBinding tag: %w", err)
		}
		pos += n
		switch {
		case fieldNum == 1 && wireType == wireVarint:
			v, m, err := readVarint(data[pos:])
			if err != nil {
				return fmt.Errorf("portBinding.port: %w", err)
			}
			cfg.Port = uint32(v)
			pos += m
		case fieldNum == 2 && wireType == wireVarint:
			v, m, err := readVarint(data[pos:])
			if err != nil {
				return fmt.Errorf("portBinding.protocol: %w", err)
			}
			switch v {
			case 1:
				cfg.Protocol = "TCP"
			case 2:
				cfg.Protocol = "UDP"
			}
			pos += m
		default:
			skipN, err := skipWireValue(data[pos:], wireType)
			if err != nil {
				return fmt.Errorf("portBinding skip field=%d: %w", fieldNum, err)
			}
			pos += skipN
		}
	}
	return nil
}

func parseMieruMux(data []byte, cfg *mieruWireCfg) error {
	for pos := 0; pos < len(data); {
		fieldNum, wireType, n, err := readTag(data[pos:])
		if err != nil {
			return fmt.Errorf("mux tag: %w", err)
		}
		pos += n
		switch {
		case fieldNum == 1 && wireType == wireVarint:
			v, m, err := readVarint(data[pos:])
			if err != nil {
				return fmt.Errorf("mux.level: %w", err)
			}
			switch v {
			case 1:
				cfg.Multiplexing = "MULTIPLEXING_OFF"
			case 2:
				cfg.Multiplexing = "MULTIPLEXING_LOW"
			case 3:
				cfg.Multiplexing = "MULTIPLEXING_MIDDLE"
			case 4:
				cfg.Multiplexing = "MULTIPLEXING_HIGH"
			}
			pos += m
		default:
			skipN, err := skipWireValue(data[pos:], wireType)
			if err != nil {
				return fmt.Errorf("mux skip field=%d: %w", fieldNum, err)
			}
			pos += skipN
		}
	}
	return nil
}

// ─── Минимальный protobuf wire-format decoder ──────────────────────────────

const (
	wireVarint = 0
	wireFixed64 = 1
	wireBytes   = 2
	wireFixed32 = 5
)

// readTag читает (fieldNumber, wireType, bytesConsumed). Поддерживает
// field-numbers ≤ 268435455 (4-байтовый varint tag).
func readTag(b []byte) (fieldNum int, wireType int, n int, err error) {
	v, m, e := readVarint(b)
	if e != nil {
		return 0, 0, 0, e
	}
	return int(v >> 3), int(v & 0x7), m, nil
}

// readVarint читает unsigned varint (≤ 10 байт).
func readVarint(b []byte) (uint64, int, error) {
	var v uint64
	for i := 0; i < len(b) && i < 10; i++ {
		v |= uint64(b[i]&0x7f) << (7 * uint(i))
		if b[i]&0x80 == 0 {
			return v, i + 1, nil
		}
	}
	return 0, 0, fmt.Errorf("varint truncated или > 10 байт")
}

// readBytes читает length-delimited значение (length varint + N байт).
func readBytes(b []byte) ([]byte, int, error) {
	length, m, err := readVarint(b)
	if err != nil {
		return nil, 0, err
	}
	if uint64(len(b)-m) < length {
		return nil, 0, fmt.Errorf("bytes: длина %d превышает остаток %d", length, len(b)-m)
	}
	return b[m : m+int(length)], m + int(length), nil
}

// readString читает length-delimited значение как строку.
func readString(b []byte) (string, int, error) {
	v, n, err := readBytes(b)
	if err != nil {
		return "", 0, err
	}
	return string(v), n, nil
}

// skipWireValue пропускает значение указанного wire-type и возвращает число
// потраченных байт.
func skipWireValue(b []byte, wireType int) (int, error) {
	switch wireType {
	case wireVarint:
		_, n, err := readVarint(b)
		return n, err
	case wireFixed64:
		if len(b) < 8 {
			return 0, fmt.Errorf("fixed64 truncated")
		}
		return 8, nil
	case wireBytes:
		_, n, err := readBytes(b)
		return n, err
	case wireFixed32:
		if len(b) < 4 {
			return 0, fmt.Errorf("fixed32 truncated")
		}
		return 4, nil
	default:
		return 0, fmt.Errorf("неподдерживаемый wire-type=%d", wireType)
	}
}

package types

// Этот файл вводит canonical-типы для Outbound, общие между sing-box, xray и
// mihomo. Каждое прокси-ядро имеет собственный JSON/YAML-формат; canonical
// представление — нейтральное, оно живёт в state.json и парсится из URL.
//
// Phase D.0: типы определены additive — Outbound получает Protocol/Transport/
// TLS поля наряду с существующим Settings. Render функции каждого ядра решают,
// использовать ли canonical (если заполнено) или fallback на Settings (legacy
// state.json, импортированный из старых версий).
//
// Phase D.1: internal/proxyparse заполняет canonical поля при парсинге URL.
// Settings остаётся для прочих/неизвестных параметров (forward-compat).

// Protocol — каноническое имя протокола outbound. Не привязано к именам
// type-полей конкретного ядра (sing-box использует "shadowsocks", xray —
// "shadowsocks", mihomo — "ss" и т.п.).
type Protocol string

const (
	ProtocolDirect      Protocol = "direct"      // служебный, прямое подключение
	ProtocolBlock       Protocol = "block"       // служебный, отбрасывание
	ProtocolSocks5      Protocol = "socks5"      // SOCKS5
	ProtocolHTTP        Protocol = "http"        // HTTP CONNECT
	ProtocolShadowsocks Protocol = "shadowsocks" // SS / SS2022
	ProtocolVMess       Protocol = "vmess"       // VMess
	ProtocolVLESS       Protocol = "vless"       // VLESS (Reality, Vision, PQ-encryption)
	ProtocolTrojan      Protocol = "trojan"      // Trojan
	ProtocolHysteria2   Protocol = "hysteria2"   // Hysteria 2
	ProtocolTUIC        Protocol = "tuic"        // TUIC v5
	ProtocolWireGuard   Protocol = "wireguard"   // WireGuard клиент
)

// TransportKind — тип транспорта (внутри "сырого" TCP/UDP).
type TransportKind string

const (
	TransportTCP   TransportKind = "tcp"   // raw TCP, без обёртки
	TransportWS    TransportKind = "ws"    // WebSocket
	TransportGRPC  TransportKind = "grpc"  // gRPC
	TransportHTTP  TransportKind = "http"  // HTTP/2
	TransportXHTTP TransportKind = "xhttp" // XHTTP (xray-only); см. XHTTPMode
	TransportQUIC  TransportKind = "quic"  // QUIC
)

// XHTTPMode — режим XHTTP-транспорта (только xray).
//
// Это разные стратегии разделения upload/download для обхода активных DPI.
// Sing-box на момент Phase D имеет лишь базовый XHTTP без таких режимов;
// при выборе ядра sing-box и заполненном XHTTPMode ≠ "" Render должен
// вернуть ошибку с подсказкой переключиться на --core xray.
type XHTTPMode string

const (
	XHTTPNone      XHTTPMode = ""           // mode не задан
	XHTTPStreamUp  XHTTPMode = "stream-up"  // только upload — split поток
	XHTTPStreamOne XHTTPMode = "stream-one" // single stream
	XHTTPPacketUp  XHTTPMode = "packet-up"  // packet-based upload
)

// Transport — параметры транспортного слоя.
//
// Для Kind=TCP остальные поля игнорируются. Для WS/GRPC/HTTP/XHTTP/QUIC
// заполняются по необходимости — рендер каждого ядра берёт нужное.
type Transport struct {
	Kind TransportKind `json:"kind,omitempty"`

	// Path — URL-path для WS/HTTP/XHTTP. Например "/ws" или "/v2".
	Path string `json:"path,omitempty"`
	// Host — Host-header для WS/HTTP/XHTTP. Может отличаться от SNI.
	Host string `json:"host,omitempty"`
	// ServiceName — имя gRPC service (mode=grpc).
	ServiceName string `json:"service_name,omitempty"`
	// Mode — XHTTP-режим (только Kind=xhttp): stream-up / stream-one / packet-up.
	Mode XHTTPMode `json:"mode,omitempty"`
	// HeaderType — тип маскировки QUIC ("none", "srtp", "wechat-video", ...).
	// Используется для QUIC-транспорта.
	HeaderType string `json:"header_type,omitempty"`
}

// RealityConfig — параметры VLESS Reality (TLS-маскировка под чужой
// домен). Используется только при TLS.Reality.Enabled=true.
type RealityConfig struct {
	Enabled bool `json:"enabled,omitempty"`
	// PublicKey — `pbk` из URL, X25519 publik key сервера.
	PublicKey string `json:"public_key,omitempty"`
	// ShortID — `sid` из URL, hex-строка ≤16 символов.
	ShortID string `json:"short_id,omitempty"`
	// SpiderX — `spx` из URL, путь для маскировки запроса.
	SpiderX string `json:"spider_x,omitempty"`

	// MLDSA65Seed / MLDSA65Verify — пост-квантовые параметры PQ-VLESS
	// (`mldsa65Seed`/`mldsa65Verify` в URL). Поддерживается только в
	// xray (sing-box до 1.13 валидирует с ошибкой).
	MLDSA65Seed   string `json:"mldsa65_seed,omitempty"`
	MLDSA65Verify string `json:"mldsa65_verify,omitempty"`
}

// UTLSConfig — uTLS fingerprint для маскировки ClientHello.
type UTLSConfig struct {
	Enabled     bool   `json:"enabled,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"` // chrome, firefox, safari, ios, random...
}

// TLSConfig — параметры TLS-обёртки.
type TLSConfig struct {
	Enabled    bool           `json:"enabled,omitempty"`
	ServerName string         `json:"server_name,omitempty"` // SNI
	ALPN       []string       `json:"alpn,omitempty"`        // ALPN список (h2, http/1.1)
	Insecure   bool           `json:"insecure,omitempty"`    // пропускать verify chain (debug)
	UTLS       *UTLSConfig    `json:"utls,omitempty"`
	Reality    *RealityConfig `json:"reality,omitempty"`
	// ECH — Encrypted Client Hello config; пока stub-поле для будущей
	// настройки (обе ядра — sing-box и xray — добавляют ECH в 2026).
	ECH *ECHConfig `json:"ech,omitempty"`
}

// ECHConfig — Encrypted Client Hello (RFC 9460+draft-tls-esni).
type ECHConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Config  string `json:"config,omitempty"` // base64 encoded ECHConfigList
}

// ProtoOpts — протокол-специфичные параметры canonical-формы.
//
// Хранит то, что не помещается в server/port/transport/tls. Например VMess
// alter_id, Shadowsocks method, Hysteria2 obfs.
type ProtoOpts struct {
	// VLESS / VMess.
	UUID       string `json:"uuid,omitempty"`
	Flow       string `json:"flow,omitempty"`       // VLESS Vision: xtls-rprx-vision[-udp443]
	Encryption string `json:"encryption,omitempty"` // VLESS PQ: mlkem768x25519plus / none
	AlterID    int    `json:"alter_id,omitempty"`   // VMess legacy
	PacketEnc  string `json:"packet_encoding,omitempty"`

	// Shadowsocks / Trojan.
	Password string `json:"password,omitempty"`
	Method   string `json:"method,omitempty"` // SS-cipher: aes-256-gcm, 2022-blake3-aes-256-gcm, ...

	// Hysteria 2.
	Hysteria2Obfs         string `json:"hy2_obfs,omitempty"` // salamander | none
	Hysteria2ObfsPassword string `json:"hy2_obfs_password,omitempty"`
	Hysteria2UpMbps       int    `json:"hy2_up_mbps,omitempty"`
	Hysteria2DownMbps     int    `json:"hy2_down_mbps,omitempty"`

	// TUIC v5.
	TUICCongestion string `json:"tuic_congestion,omitempty"` // bbr | cubic | new_reno
	TUICUDPRelay   string `json:"tuic_udp_relay,omitempty"`  // native | quic

	// WireGuard.
	WGPrivateKey string   `json:"wg_private_key,omitempty"`
	WGPublicKey  string   `json:"wg_public_key,omitempty"`
	WGPreshared  string   `json:"wg_preshared_key,omitempty"`
	WGAddresses  []string `json:"wg_addresses,omitempty"`
}

// Canonical — собранное canonical-описание outbound, не привязанное к ядру.
//
// Phase D.0: типы определены, но НЕ интегрированы в Outbound напрямую — это
// потребует разделения MarshalJSON для state.json и sing-box config.json
// (текущий MarshalJSON обслуживает оба, что не позволит чисто добавить
// canonical-поля без drift'а). Phase D.0a сделает разделение.
//
// До тех пор canonical-данные будут передаваться в render-функции отдельным
// аргументом, не как часть Outbound. internal/proxyparse будет возвращать
// (Outbound, Canonical) пара.
type Canonical struct {
	Protocol  Protocol   `json:"protocol,omitempty"`
	Transport *Transport `json:"transport,omitempty"`
	TLS       *TLSConfig `json:"tls,omitempty"`
	Proto     *ProtoOpts `json:"proto,omitempty"`
}

// IsSet возвращает true если Canonical содержит хоть один значимый параметр.
// Render-функции используют это как переключатель: true → строить из
// canonical, false → fallback на Outbound.Settings (legacy).
func (c Canonical) IsSet() bool {
	return c.Protocol != ""
}

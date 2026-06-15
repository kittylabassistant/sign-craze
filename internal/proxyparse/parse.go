package proxyparse

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// Parse преобразует proxy-URL в types.Outbound.
// Сразу заполняет sing-box-совместимые поля type/server/server_port; специфика
// (uuid, method, password, flow и т.п.) попадает в Settings.
func Parse(input string) (types.Outbound, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return types.Outbound{}, fmt.Errorf("proxyparse: пустая строка")
	}

	scheme := schemeOf(input)
	switch scheme {
	case "socks5", "socks":
		return parseSocks(input)
	case "http", "https":
		return parseHTTP(input)
	case "vless":
		return parseVLESS(input)
	case "vmess":
		return parseVMess(input)
	case "ss":
		return parseShadowsocks(input)
	default:
		return types.Outbound{}, fmt.Errorf("proxyparse: неизвестная схема %q", scheme)
	}
}

func schemeOf(s string) string {
	idx := strings.Index(s, "://")
	if idx < 0 {
		return ""
	}
	return strings.ToLower(s[:idx])
}

func parseSocks(s string) (types.Outbound, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, fmt.Errorf("proxyparse socks: %w", err)
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return types.Outbound{}, fmt.Errorf("proxyparse socks: %w", err)
	}
	o := types.Outbound{
		Tag:    "socks-proxy",
		Type:   "socks",
		Server: host,
		Port:   port,
	}
	if u.User != nil {
		settings := map[string]any{
			"version":  "5",
			"username": u.User.Username(),
		}
		if pw, ok := u.User.Password(); ok {
			settings["password"] = pw
		}
		o.Settings = settings
	}
	return o, nil
}

func parseHTTP(s string) (types.Outbound, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, fmt.Errorf("proxyparse http: %w", err)
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return types.Outbound{}, fmt.Errorf("proxyparse http: %w", err)
	}
	o := types.Outbound{
		Tag:    "http-proxy",
		Type:   "http",
		Server: host,
		Port:   port,
	}
	if u.Scheme == "https" {
		o.Settings = map[string]any{"tls": map[string]any{"enabled": true}}
	}
	if u.User != nil {
		if o.Settings == nil {
			o.Settings = map[string]any{}
		}
		o.Settings["username"] = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			o.Settings["password"] = pw
		}
	}
	return o, nil
}

func parseVLESS(s string) (types.Outbound, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, fmt.Errorf("proxyparse vless: %w", err)
	}
	if u.User == nil || u.User.Username() == "" {
		return types.Outbound{}, fmt.Errorf("proxyparse vless: отсутствует UUID")
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return types.Outbound{}, fmt.Errorf("proxyparse vless: %w", err)
	}

	q := u.Query()
	settings := map[string]any{
		"uuid": u.User.Username(),
	}

	// flat top-level VLESS поля
	if v := q.Get("flow"); v != "" {
		settings["flow"] = v
	}
	// encryption=none — XRay/V2Ray-специфичный URL-параметр, в sing-box VLESS
	// outbound поля encryption нет (см. https://sing-box.sagernet.org/configuration/outbound/vless/).
	// Игнорируем, чтобы не получить FATAL "unknown field encryption" на 1.13+.
	// PQ (mlkem768x25519plus) в sing-box настраивается через отдельные поля,
	// а не encryption — поддержим отдельно если потребуется.
	if v := q.Get("packetEncoding"); v != "" {
		settings["packet_encoding"] = v
	}

	// transport: type=ws/grpc/http/xhttp/quic создаёт transport-объект.
	// type=tcp (default) — без transport.
	if t := q.Get("type"); t != "" && t != "tcp" {
		transport := map[string]any{"type": t}
		if v := q.Get("path"); v != "" {
			transport["path"] = v
		}
		if v := q.Get("host"); v != "" {
			transport["host"] = v
		}
		if v := q.Get("serviceName"); v != "" {
			transport["service_name"] = v
		}
		settings["transport"] = transport
	}

	// tls: security=tls|reality + reality params + utls fingerprint.
	security := q.Get("security")
	if security == "tls" || security == "reality" {
		tls := map[string]any{"enabled": true}
		if v := q.Get("sni"); v != "" {
			tls["server_name"] = v
		}
		if v := q.Get("fp"); v != "" {
			tls["utls"] = map[string]any{
				"enabled":     true,
				"fingerprint": v,
			}
		}
		if alpn := q.Get("alpn"); alpn != "" {
			tls["alpn"] = strings.Split(alpn, ",")
		}
		if security == "reality" {
			reality := map[string]any{"enabled": true}
			if v := q.Get("pbk"); v != "" {
				reality["public_key"] = v
			}
			// sid → short_id; spx → spider_x (xray-only поле маскировки запроса)
			if v := q.Get("sid"); v != "" {
				reality["short_id"] = v
			}
			if v := q.Get("spx"); v != "" {
				reality["spider_x"] = v
			}
			// post-quantum mldsa65 (sing-box >= 1.12)
			if v := q.Get("mldsa65Verify"); v != "" {
				reality["mldsa65_verify"] = v
			}
			if v := q.Get("mldsa65Seed"); v != "" {
				reality["mldsa65_seed"] = v
			}
			tls["reality"] = reality
		}
		settings["tls"] = tls
	}

	return types.Outbound{
		Tag:      "vless-proxy",
		Type:     "vless",
		Server:   host,
		Port:     port,
		Settings: settings,
	}, nil
}

func parseVMess(s string) (types.Outbound, error) {
	const prefix = "vmess://"
	if !strings.HasPrefix(s, prefix) {
		return types.Outbound{}, fmt.Errorf("proxyparse vmess: неверный префикс")
	}
	encoded := s[len(prefix):]
	encoded = strings.TrimSpace(encoded)
	// Стандарт VMess: base64-encoded JSON.
	decoded, err := decodeBase64(encoded)
	if err != nil {
		return types.Outbound{}, fmt.Errorf("proxyparse vmess: base64: %w", err)
	}
	var raw map[string]any
	if jsErr := json.Unmarshal(decoded, &raw); jsErr != nil {
		return types.Outbound{}, fmt.Errorf("proxyparse vmess: json: %w", jsErr)
	}

	host, ok := raw["add"].(string)
	if !ok || host == "" {
		host, ok = raw["address"].(string)
		if !ok {
			host = ""
		}
	}
	portStr := fmt.Sprintf("%v", raw["port"])
	port, parseErr := parsePort(portStr)
	if parseErr != nil {
		return types.Outbound{}, fmt.Errorf("proxyparse vmess: port: %w", parseErr)
	}

	settings := map[string]any{}
	if v, ok := raw["id"].(string); ok && v != "" {
		settings["uuid"] = v
	}
	// alter_id может прийти как string или number в JSON — нормализуем в int.
	if aid, ok := raw["aid"]; ok {
		switch a := aid.(type) {
		case float64:
			settings["alter_id"] = int(a)
		case string:
			if n, err := strconv.Atoi(a); err == nil {
				settings["alter_id"] = n
			}
		}
	}
	if v, ok := raw["scy"].(string); ok && v != "" {
		settings["security"] = v
	}

	// Transport: net=ws/grpc/http/h2/quic создаёт transport-объект.
	if net, ok := raw["net"].(string); ok && net != "" && net != "tcp" {
		transport := map[string]any{"type": net}
		if v, ok := raw["path"].(string); ok && v != "" {
			transport["path"] = v
		}
		if v, ok := raw["host"].(string); ok && v != "" {
			transport["host"] = v
		}
		settings["transport"] = transport
	}

	// TLS: tls=tls включает transport.tls.enabled.
	if tlsField, ok := raw["tls"].(string); ok && tlsField == "tls" {
		tls := map[string]any{"enabled": true}
		if v, ok := raw["sni"].(string); ok && v != "" {
			tls["server_name"] = v
		}
		if v, ok := raw["alpn"].(string); ok && v != "" {
			tls["alpn"] = strings.Split(v, ",")
		}
		settings["tls"] = tls
	}

	return types.Outbound{
		Tag:      "vmess-proxy",
		Type:     "vmess",
		Server:   host,
		Port:     port,
		Settings: settings,
	}, nil
}

// validShadowsocksMethods — поддерживаемые шифры sing-box outbound type=shadowsocks.
// Whitelist отвергает невалидные method'ы на уровне парсера, чтобы sing-box check
// не падал с криптической ошибкой позже.
var validShadowsocksMethods = map[string]bool{
	"none":                          true,
	"aes-128-gcm":                   true,
	"aes-192-gcm":                   true,
	"aes-256-gcm":                   true,
	"aes-128-ctr":                   true,
	"aes-192-ctr":                   true,
	"aes-256-ctr":                   true,
	"aes-128-cfb":                   true,
	"aes-192-cfb":                   true,
	"aes-256-cfb":                   true,
	"chacha20-ietf-poly1305":        true,
	"xchacha20-ietf-poly1305":       true,
	"2022-blake3-aes-128-gcm":       true,
	"2022-blake3-aes-256-gcm":       true,
	"2022-blake3-chacha20-poly1305": true,
}

func parseShadowsocks(s string) (types.Outbound, error) {
	const prefix = "ss://"
	if !strings.HasPrefix(s, prefix) {
		return types.Outbound{}, fmt.Errorf("proxyparse ss: неверный префикс")
	}
	body := s[len(prefix):]
	// Возможны два формата:
	//  1. base64(method:password)@host:port[#tag]
	//  2. method:password@host:port[#tag] (новая SIP002)
	if at := strings.LastIndex(body, "@"); at >= 0 {
		userPart := body[:at]
		hostPart := body[at+1:]

		// Удалить опциональный #tag.
		if hash := strings.Index(hostPart, "#"); hash >= 0 {
			hostPart = hostPart[:hash]
		}

		// Если userPart не содержит ":", считаем base64-encoded.
		var method, password string
		if !strings.Contains(userPart, ":") {
			decoded, err := decodeBase64(userPart)
			if err != nil {
				return types.Outbound{}, fmt.Errorf("proxyparse ss: base64: %w", err)
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) != 2 {
				return types.Outbound{}, fmt.Errorf("proxyparse ss: некорректный формат method:password")
			}
			method = parts[0]
			password = parts[1]
		} else {
			parts := strings.SplitN(userPart, ":", 2)
			method = parts[0]
			password = parts[1]
		}

		if !validShadowsocksMethods[method] {
			return types.Outbound{}, fmt.Errorf("proxyparse ss: неподдерживаемый шифр %q", method)
		}

		host, port, err := splitHostPort(hostPart)
		if err != nil {
			return types.Outbound{}, fmt.Errorf("proxyparse ss: %w", err)
		}
		return types.Outbound{
			Tag:    "ss-proxy",
			Type:   "shadowsocks",
			Server: host,
			Port:   port,
			Settings: map[string]any{
				"method":   method,
				"password": password,
			},
		}, nil
	}
	return types.Outbound{}, fmt.Errorf("proxyparse ss: формат не распознан")
}

// splitHostPort разделяет "host:port" на компоненты с парсингом порта.
func splitHostPort(hp string) (string, types.Port, error) {
	idx := strings.LastIndex(hp, ":")
	if idx < 0 {
		return "", 0, fmt.Errorf("отсутствует :port")
	}
	host := hp[:idx]
	port, err := parsePort(hp[idx+1:])
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func parsePort(s string) (types.Port, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("невозможно распарсить порт %q: %w", s, err)
	}
	if n <= 0 || n > 65535 {
		return 0, fmt.Errorf("порт вне диапазона: %d", n)
	}
	return types.Port(n), nil
}

func decodeBase64(s string) ([]byte, error) {
	// Сначала пробуем стандартный, потом URL-safe; обе разновидности — с/без padding.
	for _, dec := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if data, err := dec.DecodeString(s); err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("base64: ни одна кодировка не подошла")
}

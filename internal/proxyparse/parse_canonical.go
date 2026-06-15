package proxyparse

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// ParseCanonical парсит proxy-URL и возвращает минимальный Outbound (Tag/Type/Server/Port)
// плюс структурное Canonical-описание (Protocol/Transport/TLS/Proto).
// Существующая функция Parse не затрагивается — ParseCanonical работает параллельно.
func ParseCanonical(rawURL string) (types.Outbound, types.Canonical, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse: пустая строка")
	}

	scheme := schemeOf(rawURL)
	switch scheme {
	case "vless":
		return canonicalVLESS(rawURL)
	case "vmess":
		return canonicalVMess(rawURL)
	case "ss":
		return canonicalShadowsocks(rawURL)
	case "trojan":
		return canonicalTrojan(rawURL)
	case "hysteria2", "hy2":
		return canonicalHysteria2(rawURL)
	case "tuic":
		return canonicalTUIC(rawURL)
	case "socks5", "socks":
		return canonicalSocks5(rawURL)
	case "http", "https":
		return canonicalHTTP(rawURL)
	case "mierus":
		return canonicalMieruSimple(rawURL)
	case "mieru":
		return canonicalMieruProto(rawURL)
	case "naive+https", "naive+quic":
		return canonicalNaive(rawURL)
	default:
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse: неизвестная схема %q", scheme)
	}
}

// tagFromFragment декодирует fragment URL (после #) как тег; возвращает fallback если пусто.
func tagFromFragment(u *url.URL, fallback string) string {
	// u.Fragment уже URL-decoded стандартной библиотекой
	if u.Fragment != "" {
		return u.Fragment
	}
	return fallback
}

// canonicalTransportKind маппит строковое значение query-параметра type → TransportKind.
func canonicalTransportKind(t string) types.TransportKind {
	switch strings.ToLower(t) {
	case "ws":
		return types.TransportWS
	case "grpc":
		return types.TransportGRPC
	case "http", "h2":
		return types.TransportHTTP
	case "xhttp":
		return types.TransportXHTTP
	case "quic":
		return types.TransportQUIC
	default:
		return types.TransportTCP
	}
}

// canonicalXHTTPMode маппит строку режима XHTTP → XHTTPMode.
func canonicalXHTTPMode(m string) types.XHTTPMode {
	switch m {
	case "stream-up":
		return types.XHTTPStreamUp
	case "stream-one":
		return types.XHTTPStreamOne
	case "packet-up":
		return types.XHTTPPacketUp
	default:
		return types.XHTTPNone
	}
}

// buildTransport строит Transport из query-параметров (общий для VLESS/Trojan).
// Возвращает nil если transport = TCP без дополнительных полей.
func buildTransport(q url.Values) *types.Transport {
	transportType := q.Get("type")
	kind := canonicalTransportKind(transportType)

	// TCP без дополнительных параметров — transport не нужен
	if kind == types.TransportTCP {
		return nil
	}

	t := &types.Transport{Kind: kind}
	if v := q.Get("path"); v != "" {
		t.Path = v
	}
	if v := q.Get("host"); v != "" {
		t.Host = v
	}
	if v := q.Get("serviceName"); v != "" {
		t.ServiceName = v
	}
	if v := q.Get("headerType"); v != "" {
		t.HeaderType = v
	}
	if kind == types.TransportXHTTP {
		t.Mode = canonicalXHTTPMode(q.Get("mode"))
	}
	return t
}

// buildTLS строит TLSConfig из query-параметров security/sni/alpn/fp/pbk/sid/spx/mldsa65*.
// Возвращает nil если security=none или не задан.
func buildTLS(q url.Values) (*types.TLSConfig, error) {
	security := q.Get("security")
	if security == "" || security == "none" {
		// Проверяем: если pbk или sid присутствует без security=reality — ошибка
		if q.Get("pbk") != "" || q.Get("sid") != "" {
			return nil, fmt.Errorf("proxyparse: pbk/sid заданы, но security != reality")
		}
		return nil, nil
	}

	if security != "tls" && security != "reality" {
		return nil, fmt.Errorf("proxyparse: неизвестный security=%q", security)
	}

	tls := &types.TLSConfig{Enabled: true}

	if v := q.Get("sni"); v != "" {
		tls.ServerName = v
	}
	if alpn := q.Get("alpn"); alpn != "" {
		tls.ALPN = strings.Split(alpn, ",")
	}
	if fp := q.Get("fp"); fp != "" {
		tls.UTLS = &types.UTLSConfig{Enabled: true, Fingerprint: fp}
	}

	if security == "reality" {
		reality := &types.RealityConfig{Enabled: true}
		if v := q.Get("pbk"); v != "" {
			reality.PublicKey = v
		}
		if v := q.Get("sid"); v != "" {
			reality.ShortID = v
		}
		if v := q.Get("spx"); v != "" {
			reality.SpiderX = v
		}
		if v := q.Get("mldsa65Seed"); v != "" {
			reality.MLDSA65Seed = v
		}
		if v := q.Get("mldsa65Verify"); v != "" {
			reality.MLDSA65Verify = v
		}
		tls.Reality = reality
	} else if q.Get("pbk") != "" || q.Get("sid") != "" {
		// security=tls, но pbk/sid заданы — ошибка
		return nil, fmt.Errorf("proxyparse: pbk/sid заданы, но security=tls (ожидалось reality)")
	}

	return tls, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// VLESS
// ─────────────────────────────────────────────────────────────────────────────

func canonicalVLESS(s string) (types.Outbound, types.Canonical, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse vless: %w", err)
	}
	if u.User == nil || u.User.Username() == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse vless: отсутствует UUID")
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse vless: %w", err)
	}

	q := u.Query()

	tls, err := buildTLS(q)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse vless: %w", err)
	}

	proto := &types.ProtoOpts{
		UUID: u.User.Username(),
	}
	if v := q.Get("flow"); v != "" {
		proto.Flow = v
	}
	if v := q.Get("encryption"); v != "" {
		proto.Encryption = v
	}
	if v := q.Get("packetEncoding"); v != "" {
		proto.PacketEnc = v
	}

	canon := types.Canonical{
		Protocol:  types.ProtocolVLESS,
		Transport: buildTransport(q),
		TLS:       tls,
		Proto:     proto,
	}

	ob := types.Outbound{
		Tag:    tagFromFragment(u, "vless-out"),
		Type:   "vless",
		Server: host,
		Port:   port,
	}

	return ob, canon, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// VMess (base64 JSON)
// ─────────────────────────────────────────────────────────────────────────────

// vmessJSON — внутренняя структура VMess base64-JSON.
type vmessJSON struct {
	PS   string `json:"ps"`
	Add  string `json:"add"`
	Port any    `json:"port"` // может быть string или number
	ID   string `json:"id"`
	Aid  any    `json:"aid"` // может быть string или number
	Net  string `json:"net"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
	ALPN string `json:"alpn"`
}

func canonicalVMess(s string) (types.Outbound, types.Canonical, error) {
	const prefix = "vmess://"
	if !strings.HasPrefix(s, prefix) {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse vmess: неверный префикс")
	}
	encoded := strings.TrimSpace(s[len(prefix):])
	decoded, err := decodeBase64(encoded)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse vmess: base64: %w", err)
	}

	var v vmessJSON
	if jsErr := json.Unmarshal(decoded, &v); jsErr != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse vmess: json: %w", jsErr)
	}

	if v.Add == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse vmess: отсутствует host (add)")
	}

	portStr := fmt.Sprintf("%v", v.Port)
	port, err := parsePort(portStr)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse vmess: port: %w", err)
	}

	// AlterID: string или number
	alterID := 0
	switch a := v.Aid.(type) {
	case float64:
		alterID = int(a)
	case string:
		if n, e := strconv.Atoi(a); e == nil {
			alterID = n
		}
	}

	proto := &types.ProtoOpts{
		UUID:    v.ID,
		AlterID: alterID,
	}

	// Transport
	var transport *types.Transport
	if v.Net != "" && v.Net != "tcp" {
		kind := canonicalTransportKind(v.Net)
		transport = &types.Transport{Kind: kind}
		if v.Path != "" {
			transport.Path = v.Path
		}
		if v.Host != "" {
			transport.Host = v.Host
		}
	}

	// TLS
	var tlsCfg *types.TLSConfig
	if v.TLS == "tls" {
		tlsCfg = &types.TLSConfig{Enabled: true}
		if v.SNI != "" {
			tlsCfg.ServerName = v.SNI
		}
		if v.ALPN != "" {
			tlsCfg.ALPN = strings.Split(v.ALPN, ",")
		}
	}

	tag := v.PS
	if tag == "" {
		tag = "vmess-out"
	}

	canon := types.Canonical{
		Protocol:  types.ProtocolVMess,
		Transport: transport,
		TLS:       tlsCfg,
		Proto:     proto,
	}

	ob := types.Outbound{
		Tag:    tag,
		Type:   "vmess",
		Server: v.Add,
		Port:   port,
	}

	return ob, canon, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Shadowsocks
// ─────────────────────────────────────────────────────────────────────────────

func canonicalShadowsocks(s string) (types.Outbound, types.Canonical, error) {
	const prefix = "ss://"
	if !strings.HasPrefix(s, prefix) {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse ss: неверный префикс")
	}
	body := s[len(prefix):]

	at := strings.LastIndex(body, "@")
	if at < 0 {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse ss: формат не распознан")
	}

	userPart := body[:at]
	hostPart := body[at+1:]

	// Извлекаем тег из fragment
	fragment := ""
	if hash := strings.Index(hostPart, "#"); hash >= 0 {
		raw := hostPart[hash+1:]
		if dec, err := url.QueryUnescape(raw); err == nil {
			fragment = dec
		} else {
			fragment = raw
		}
		hostPart = hostPart[:hash]
	}

	// Убираем query-параметры из hostPart
	if q := strings.Index(hostPart, "?"); q >= 0 {
		hostPart = hostPart[:q]
	}

	var method, password string
	if !strings.Contains(userPart, ":") {
		// base64-encoded userinfo
		decoded, err := decodeBase64(userPart)
		if err != nil {
			return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse ss: base64: %w", err)
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse ss: некорректный формат method:password")
		}
		method = parts[0]
		password = parts[1]
	} else {
		parts := strings.SplitN(userPart, ":", 2)
		method = parts[0]
		password = parts[1]
	}

	if !validShadowsocksMethods[method] {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse ss: неподдерживаемый шифр %q", method)
	}

	host, port, err := splitHostPort(hostPart)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse ss: %w", err)
	}

	tag := fragment
	if tag == "" {
		tag = "ss-out"
	}

	canon := types.Canonical{
		Protocol: types.ProtocolShadowsocks,
		Proto: &types.ProtoOpts{
			Method:   method,
			Password: password,
		},
	}

	ob := types.Outbound{
		Tag:    tag,
		Type:   "shadowsocks",
		Server: host,
		Port:   port,
	}

	return ob, canon, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Trojan
// ─────────────────────────────────────────────────────────────────────────────

func canonicalTrojan(s string) (types.Outbound, types.Canonical, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse trojan: %w", err)
	}
	if u.User == nil || u.User.Username() == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse trojan: отсутствует password")
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse trojan: %w", err)
	}

	q := u.Query()

	// Trojan всегда TLS
	tls := &types.TLSConfig{Enabled: true}
	if v := q.Get("sni"); v != "" {
		tls.ServerName = v
	}
	if alpn := q.Get("alpn"); alpn != "" {
		tls.ALPN = strings.Split(alpn, ",")
	}
	if fp := q.Get("fp"); fp != "" {
		tls.UTLS = &types.UTLSConfig{Enabled: true, Fingerprint: fp}
	}
	// Reality для Trojan (редко, но возможно)
	if q.Get("pbk") != "" || q.Get("security") == "reality" {
		reality := &types.RealityConfig{Enabled: true}
		if v := q.Get("pbk"); v != "" {
			reality.PublicKey = v
		}
		if v := q.Get("sid"); v != "" {
			reality.ShortID = v
		}
		if v := q.Get("spx"); v != "" {
			reality.SpiderX = v
		}
		tls.Reality = reality
	}

	canon := types.Canonical{
		Protocol:  types.ProtocolTrojan,
		Transport: buildTransport(q),
		TLS:       tls,
		Proto: &types.ProtoOpts{
			Password: u.User.Username(),
		},
	}

	ob := types.Outbound{
		Tag:    tagFromFragment(u, "trojan-out"),
		Type:   "trojan",
		Server: host,
		Port:   port,
	}

	return ob, canon, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Hysteria2
// ─────────────────────────────────────────────────────────────────────────────

func canonicalHysteria2(s string) (types.Outbound, types.Canonical, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse hysteria2: %w", err)
	}
	if u.User == nil || u.User.Username() == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse hysteria2: отсутствует password")
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse hysteria2: %w", err)
	}

	q := u.Query()

	proto := &types.ProtoOpts{
		Password: u.User.Username(),
	}
	if v := q.Get("obfs"); v != "" {
		proto.Hysteria2Obfs = v
	}
	if v := q.Get("obfs-password"); v != "" {
		proto.Hysteria2ObfsPassword = v
	}
	if v := q.Get("up"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			proto.Hysteria2UpMbps = n
		}
	}
	if v := q.Get("down"); v != "" {
		if n, e := strconv.Atoi(v); e == nil {
			proto.Hysteria2DownMbps = n
		}
	}

	// TLS для Hysteria2 всегда включён
	tls := &types.TLSConfig{Enabled: true}
	if v := q.Get("sni"); v != "" {
		tls.ServerName = v
	}
	if alpn := q.Get("alpn"); alpn != "" {
		tls.ALPN = strings.Split(alpn, ",")
	}
	insecure := q.Get("insecure")
	if insecure == "1" || strings.ToLower(insecure) == "true" {
		tls.Insecure = true
	}

	canon := types.Canonical{
		Protocol: types.ProtocolHysteria2,
		TLS:      tls,
		Proto:    proto,
	}

	ob := types.Outbound{
		Tag:    tagFromFragment(u, "hy2-out"),
		Type:   "hysteria2",
		Server: host,
		Port:   port,
	}

	return ob, canon, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// TUIC v5
// ─────────────────────────────────────────────────────────────────────────────

func canonicalTUIC(s string) (types.Outbound, types.Canonical, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse tuic: %w", err)
	}
	if u.User == nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse tuic: отсутствует uuid:password")
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse tuic: %w", err)
	}

	q := u.Query()

	proto := &types.ProtoOpts{
		UUID: u.User.Username(),
	}
	if pw, ok := u.User.Password(); ok {
		proto.Password = pw
	}
	if v := q.Get("congestion_control"); v != "" {
		proto.TUICCongestion = v
	}
	if v := q.Get("udp_relay_mode"); v != "" {
		proto.TUICUDPRelay = v
	}

	// TUIC всегда TLS
	tls := &types.TLSConfig{Enabled: true}
	if v := q.Get("sni"); v != "" {
		tls.ServerName = v
	}
	if alpn := q.Get("alpn"); alpn != "" {
		tls.ALPN = strings.Split(alpn, ",")
	}

	canon := types.Canonical{
		Protocol: types.ProtocolTUIC,
		TLS:      tls,
		Proto:    proto,
	}

	ob := types.Outbound{
		Tag:    tagFromFragment(u, "tuic-out"),
		Type:   "tuic",
		Server: host,
		Port:   port,
	}

	return ob, canon, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// SOCKS5
// ─────────────────────────────────────────────────────────────────────────────

func canonicalSocks5(s string) (types.Outbound, types.Canonical, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse socks5: %w", err)
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse socks5: %w", err)
	}

	proto := &types.ProtoOpts{}
	if u.User != nil {
		proto.UUID = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			proto.Password = pw
		}
	}

	canon := types.Canonical{
		Protocol: types.ProtocolSocks5,
		Proto:    proto,
	}

	ob := types.Outbound{
		Tag:    tagFromFragment(u, "socks5-out"),
		Type:   "socks",
		Server: host,
		Port:   port,
	}

	return ob, canon, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP / HTTPS
// ─────────────────────────────────────────────────────────────────────────────

func canonicalHTTP(s string) (types.Outbound, types.Canonical, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse http: %w", err)
	}

	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse http: %w", err)
	}

	proto := &types.ProtoOpts{}
	if u.User != nil {
		proto.UUID = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			proto.Password = pw
		}
	}

	var tls *types.TLSConfig
	if u.Scheme == "https" {
		tls = &types.TLSConfig{Enabled: true}
	}

	canon := types.Canonical{
		Protocol: types.ProtocolHTTP,
		TLS:      tls,
		Proto:    proto,
	}

	ob := types.Outbound{
		Tag:    tagFromFragment(u, "http-out"),
		Type:   "http",
		Server: host,
		Port:   port,
	}

	return ob, canon, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Naive (process chain через naiveproxy demon)
// ─────────────────────────────────────────────────────────────────────────────

func canonicalNaive(s string) (types.Outbound, types.Canonical, error) {
	u, err := url.Parse(s)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse naive: %w", err)
	}
	if u.User == nil || u.User.Username() == "" {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse naive: отсутствует username")
	}
	host, port, err := splitHostPort(u.Host)
	if err != nil {
		return types.Outbound{}, types.Canonical{}, fmt.Errorf("proxyparse naive: %w", err)
	}

	q := u.Query()
	proto := &types.ProtoOpts{
		NaiveUsername: u.User.Username(),
	}
	if pw, ok := u.User.Password(); ok {
		proto.NaivePassword = pw
	}
	if v := q.Get("padding"); v == "1" || strings.EqualFold(v, "true") {
		proto.NaivePadding = true
	}
	if v := q.Get("extra-headers"); v != "" {
		proto.NaiveExtraHdrs = v
	}

	canon := types.Canonical{
		Protocol: types.ProtocolNaive,
		Proto:    proto,
	}

	ob := types.Outbound{
		Tag:    tagFromFragment(u, "naive-out"),
		Type:   "socks", // sing-box-side: подключаемся к локальному naive-демону
		Server: host,    // upstream server, сохраняется в state — daemon использует
		Port:   port,    // upstream port
	}
	return ob, canon, nil
}

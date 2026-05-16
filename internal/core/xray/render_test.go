package xray

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// xrayGoldenCases — canonical-случаи, используемые в TestRender_GoldenWriteback.
// Каждый элемент задаёт имя golden-файла (без .json) и ConfigParams для Render.
// При UPDATE_GOLDEN=1 файлы перегенерируются; без него — нормализованный JSON-diff.
var xrayGoldenCases = []struct {
	name   string
	tag    string
	server string
	port   types.Port
	c      types.Canonical
}{
	{
		// VLESS + Vision flow udp443 + Reality
		name: "vless_vision_udp443", tag: "vless-vision-udp443",
		server: "example.com", port: 443,
		c: types.Canonical{
			Protocol: types.ProtocolVLESS,
			TLS: &types.TLSConfig{
				Enabled:    true,
				ServerName: "example.com",
				UTLS:       &types.UTLSConfig{Enabled: true, Fingerprint: "chrome"},
				Reality: &types.RealityConfig{
					Enabled:   true,
					PublicKey: "mwRiBidYJGzBfYLhvYFCGisjZ1sT4kmjqKCyWHPRtXo",
					ShortID:   "deadbeef00",
				},
			},
			Proto: &types.ProtoOpts{
				UUID:       "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				Encryption: "none",
				Flow:       "xtls-rprx-vision-udp443",
			},
		},
	},
	{
		// VLESS + XHTTP packet-up mode + TLS + uTLS chrome
		name: "vless_xhttp_packetup", tag: "vless-xhttp-packetup",
		server: "xhttp.example.com", port: 443,
		c: types.Canonical{
			Protocol: types.ProtocolVLESS,
			Transport: &types.Transport{
				Kind: types.TransportXHTTP,
				Path: "/xhttp",
				Host: "xhttp.example.com",
				Mode: types.XHTTPPacketUp,
			},
			TLS: &types.TLSConfig{
				Enabled:    true,
				ServerName: "xhttp.example.com",
				UTLS:       &types.UTLSConfig{Enabled: true, Fingerprint: "chrome"},
			},
			Proto: &types.ProtoOpts{
				UUID:       "cccccccc-dddd-eeee-ffff-000000000000",
				Encryption: "none",
			},
		},
	},
	{
		// VLESS + TCP + Reality + uTLS chrome
		name: "vless_reality", tag: "vless-reality",
		server: "example.com", port: 443,
		c: types.Canonical{
			Protocol: types.ProtocolVLESS,
			TLS: &types.TLSConfig{
				Enabled:    true,
				ServerName: "example.com",
				UTLS:       &types.UTLSConfig{Enabled: true, Fingerprint: "chrome"},
				Reality: &types.RealityConfig{
					Enabled:   true,
					PublicKey: "mwRiBidYJGzBfYLhvYFCGisjZ1sT4kmjqKCyWHPRtXo",
					ShortID:   "deadbeef00",
				},
			},
			Proto: &types.ProtoOpts{
				UUID:       "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				Encryption: "none",
				Flow:       "xtls-rprx-vision",
			},
		},
	},
	{
		// Trojan + TCP + TLS (для контраста с VLESS)
		name: "trojan_tcp_tls", tag: "trojan-tcp-tls",
		server: "trojan.example.com", port: 443,
		c: types.Canonical{
			Protocol: types.ProtocolTrojan,
			TLS: &types.TLSConfig{
				Enabled:    true,
				ServerName: "trojan.example.com",
			},
			Proto: &types.ProtoOpts{
				Password: "trojan-password-placeholder",
			},
		},
	},
}

// TestRender_GoldenWriteback проверяет соответствие вывода Render golden-файлам
// из testdata/canonical/.
//
// При UPDATE_GOLDEN=1 файлы перегенерируются из in-memory canonical-описаний.
// Используется для синхронизации golden-файлов после изменений render-логики.
func TestRender_GoldenWriteback(t *testing.T) {
	update := os.Getenv("UPDATE_GOLDEN") == "1"

	for _, tc := range xrayGoldenCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			p := DefaultConfigParams()
			p.DefaultTag = tc.tag
			p.Outbounds = []types.Outbound{{Tag: tc.tag, Server: tc.server, Port: tc.port}}
			p.Canonicals = map[string]types.Canonical{tc.tag: tc.c}

			data, err := Render(p)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			// Нормализуем через промежуточный any: гарантируем устойчивый JSON-diff
			var gotObj any
			if unmarshalErr := json.Unmarshal(data, &gotObj); unmarshalErr != nil {
				t.Fatalf("json.Unmarshal вывода Render: %v", unmarshalErr)
			}
			gotNorm, _ := json.MarshalIndent(gotObj, "", "  ")

			goldenPath := filepath.Join("testdata", "canonical", tc.name+".json")

			if update {
				if writeErr := os.WriteFile(goldenPath, append(gotNorm, '\n'), 0o644); writeErr != nil {
					t.Fatalf("запись golden-файла %s: %v", goldenPath, writeErr)
				}
				t.Logf("обновлён golden: %s", goldenPath)
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden-файл не найден %s; запустите с UPDATE_GOLDEN=1 для генерации: %v", goldenPath, err)
			}

			var wantObj any
			if err := json.Unmarshal(want, &wantObj); err != nil {
				t.Fatalf("golden-файл содержит невалидный JSON %s: %v", goldenPath, err)
			}
			wantNorm, _ := json.MarshalIndent(wantObj, "", "  ")

			if string(gotNorm) != string(wantNorm) {
				t.Errorf("Render не совпадает с golden %s:\ngot:\n%s\nwant:\n%s",
					goldenPath, gotNorm, wantNorm)
			}
		})
	}
}

// TestRender_Geosite_NoDat_ReturnsError проверяет, что при отсутствии geosite.dat
// Render возвращает понятную ошибку, а не FATAL внутри xray.
func TestRender_Geosite_NoDat_ReturnsError(t *testing.T) {
	tmp := t.TempDir() // tempdir без geosite.dat/geoip.dat

	p := DefaultConfigParams()
	p.GeoAssetsDir = tmp
	p.RoutingConfig = &types.RoutingConfig{
		Version: 1,
		Rules: []types.RouteRule{
			{RuleSet: []string{"geosite-youtube"}, Outbound: "direct"},
		},
		Final: "direct",
	}
	p.Outbounds = []types.Outbound{{Tag: "direct", Type: "direct"}}

	_, err := Render(p)
	if err == nil {
		t.Fatal("Render должен вернуть ошибку при отсутствии geosite.dat")
	}
	if !strings.Contains(err.Error(), "geosite.dat") {
		t.Errorf("ошибка должна содержать 'geosite.dat', получено: %v", err)
	}
	if !strings.Contains(err.Error(), "--update-geo") {
		t.Errorf("ошибка должна содержать '--update-geo', получено: %v", err)
	}
}

// TestRender_Geoip_NoDat_ReturnsError проверяет, что при отсутствии geoip.dat
// Render возвращает понятную ошибку.
func TestRender_Geoip_NoDat_ReturnsError(t *testing.T) {
	tmp := t.TempDir() // tempdir без geoip.dat

	p := DefaultConfigParams()
	p.GeoAssetsDir = tmp
	p.RoutingConfig = &types.RoutingConfig{
		Version: 1,
		Rules: []types.RouteRule{
			{RuleSet: []string{"geoip-ru"}, Outbound: "direct"},
		},
		Final: "direct",
	}
	p.Outbounds = []types.Outbound{{Tag: "direct", Type: "direct"}}

	_, err := Render(p)
	if err == nil {
		t.Fatal("Render должен вернуть ошибку при отсутствии geoip.dat")
	}
	if !strings.Contains(err.Error(), "geoip.dat") {
		t.Errorf("ошибка должна содержать 'geoip.dat', получено: %v", err)
	}
}

// TestRender_Geosite_DatPresent_OK проверяет, что при наличии geosite.dat
// Render не возвращает ошибку.
func TestRender_Geosite_DatPresent_OK(t *testing.T) {
	tmp := t.TempDir()
	// создаём фиктивные .dat файлы
	if err := os.WriteFile(filepath.Join(tmp, "geosite.dat"), []byte("fake"), 0o644); err != nil {
		t.Fatalf("не удалось создать geosite.dat: %v", err)
	}

	p := DefaultConfigParams()
	p.GeoAssetsDir = tmp
	p.RoutingConfig = &types.RoutingConfig{
		Version: 1,
		Rules: []types.RouteRule{
			{RuleSet: []string{"geosite-youtube"}, Outbound: "direct"},
		},
		Final: "direct",
	}
	p.Outbounds = []types.Outbound{{Tag: "direct", Type: "direct"}}

	_, err := Render(p)
	if err != nil {
		t.Fatalf("Render не должен возвращать ошибку при наличии geosite.dat: %v", err)
	}
}

// mustRender — вспомогательная функция: вызывает Render и проваливает тест при ошибке.
func mustRender(t *testing.T, p ConfigParams) map[string]any {
	t.Helper()
	data, err := Render(p)
	if err != nil {
		t.Fatalf("Render вернул ошибку: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("невозможно разобрать JSON вывода: %v", err)
	}
	return result
}

// getPath извлекает значение по цепочке ключей из вложенного map.
func getPath(m map[string]any, path ...string) (any, bool) {
	var cur any = m
	for _, key := range path {
		switch v := cur.(type) {
		case map[string]any:
			val, ok := v[key]
			if !ok {
				return nil, false
			}
			cur = val
		default:
			return nil, false
		}
	}
	return cur, true
}

// getSlicePath извлекает элемент слайса по индексу в цепочке.
func getOutboundByTag(result map[string]any, tag string) (map[string]any, bool) {
	obs, ok := result["outbounds"]
	if !ok {
		return nil, false
	}
	slice, ok := obs.([]any)
	if !ok {
		return nil, false
	}
	for _, item := range slice {
		ob, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if ob["tag"] == tag {
			return ob, true
		}
	}
	return nil, false
}

// makeParams строит минимальный ConfigParams с одним outbound.
func makeParams(tag string, c types.Canonical, server string, port types.Port) ConfigParams {
	p := DefaultConfigParams()
	p.DefaultTag = tag
	p.Outbounds = []types.Outbound{
		{Tag: tag, Server: server, Port: port},
	}
	p.Canonicals = map[string]types.Canonical{tag: c}
	return p
}

// TestRender_VLESS_Reality_XHTTP_PacketUp проверяет рендер VLESS + Reality + XHTTP PacketUp.
func TestRender_VLESS_Reality_XHTTP_PacketUp(t *testing.T) {
	c := types.Canonical{
		Protocol: types.ProtocolVLESS,
		Transport: &types.Transport{
			Kind: types.TransportXHTTP,
			Path: "/v2",
			Host: "example.com",
			Mode: types.XHTTPPacketUp,
		},
		TLS: &types.TLSConfig{
			Enabled:    true,
			ServerName: "example.com",
			UTLS:       &types.UTLSConfig{Enabled: true, Fingerprint: "chrome"},
			Reality: &types.RealityConfig{
				Enabled:       true,
				PublicKey:     "pbk123",
				ShortID:       "sid456",
				SpiderX:       "/spx",
				MLDSA65Verify: "mldsaverify",
			},
		},
		Proto: &types.ProtoOpts{
			UUID:       "test-uuid",
			Encryption: "mlkem768x25519plus",
			Flow:       "xtls-rprx-vision-udp443",
		},
	}

	p := makeParams("proxy", c, "1.2.3.4", 443)
	result := mustRender(t, p)

	ob, ok := getOutboundByTag(result, "proxy")
	if !ok {
		t.Fatal("outbound с тегом 'proxy' не найден")
	}

	// Проверяем encryption в users[0]
	enc, ok := getPath(ob, "settings", "vnext")
	if !ok {
		t.Fatal("settings.vnext отсутствует")
	}
	vnext := enc.([]any)
	if len(vnext) == 0 {
		t.Fatal("vnext пуст")
	}
	users := vnext[0].(map[string]any)["users"].([]any)
	user := users[0].(map[string]any)
	if user["encryption"] != "mlkem768x25519plus" {
		t.Errorf("encryption = %v, ожидалось mlkem768x25519plus", user["encryption"])
	}

	// Проверяем mldsa65Verify в realitySettings
	mldsa, ok := getPath(ob, "streamSettings", "realitySettings", "mldsa65Verify")
	if !ok {
		t.Fatal("realitySettings.mldsa65Verify отсутствует")
	}
	if mldsa != "mldsaverify" {
		t.Errorf("mldsa65Verify = %v, ожидалось mldsaverify", mldsa)
	}

	// Проверяем xhttpSettings.mode
	mode, ok := getPath(ob, "streamSettings", "xhttpSettings", "mode")
	if !ok {
		t.Fatal("xhttpSettings.mode отсутствует")
	}
	if mode != "packet-up" {
		t.Errorf("xhttpSettings.mode = %v, ожидалось packet-up", mode)
	}

	// Проверяем security=reality
	sec, ok := getPath(ob, "streamSettings", "security")
	if !ok {
		t.Fatal("streamSettings.security отсутствует")
	}
	if sec != "reality" {
		t.Errorf("security = %v, ожидалось reality", sec)
	}
}

// TestRender_VLESS_Vision_UDP443_Flow проверяет flow=xtls-rprx-vision-udp443.
func TestRender_VLESS_Vision_UDP443_Flow(t *testing.T) {
	c := types.Canonical{
		Protocol: types.ProtocolVLESS,
		TLS: &types.TLSConfig{
			Enabled:    true,
			ServerName: "example.com",
			Reality: &types.RealityConfig{
				Enabled:   true,
				PublicKey: "pk",
				ShortID:   "00",
			},
		},
		Proto: &types.ProtoOpts{
			UUID:       "uuid-flow",
			Encryption: "none",
			Flow:       "xtls-rprx-vision-udp443",
		},
	}

	p := makeParams("vless-flow", c, "host.example.com", 443)
	result := mustRender(t, p)

	ob, ok := getOutboundByTag(result, "vless-flow")
	if !ok {
		t.Fatal("outbound vless-flow не найден")
	}

	vnext, _ := getPath(ob, "settings", "vnext")
	users := vnext.([]any)[0].(map[string]any)["users"].([]any)
	user := users[0].(map[string]any)
	if user["flow"] != "xtls-rprx-vision-udp443" {
		t.Errorf("flow = %v, ожидалось xtls-rprx-vision-udp443", user["flow"])
	}
}

// TestRender_Shadowsocks_2022 проверяет метод 2022-blake3-aes-256-gcm.
func TestRender_Shadowsocks_2022(t *testing.T) {
	c := types.Canonical{
		Protocol: types.ProtocolShadowsocks,
		Proto: &types.ProtoOpts{
			Method:   "2022-blake3-aes-256-gcm",
			Password: "secret-pass",
		},
	}

	p := makeParams("ss-2022", c, "ss.example.com", 8388)
	result := mustRender(t, p)

	ob, ok := getOutboundByTag(result, "ss-2022")
	if !ok {
		t.Fatal("outbound ss-2022 не найден")
	}
	if ob["protocol"] != "shadowsocks" {
		t.Errorf("protocol = %v, ожидалось shadowsocks", ob["protocol"])
	}

	method, ok := getPath(ob, "settings", "servers")
	if !ok {
		t.Fatal("settings.servers отсутствует")
	}
	servers := method.([]any)
	srv := servers[0].(map[string]any)
	if srv["method"] != "2022-blake3-aes-256-gcm" {
		t.Errorf("method = %v, ожидалось 2022-blake3-aes-256-gcm", srv["method"])
	}
	if srv["password"] != "secret-pass" {
		t.Errorf("password = %v, ожидалось secret-pass", srv["password"])
	}
}

// TestRender_Trojan_WS проверяет Trojan + WS transport + security=tls.
func TestRender_Trojan_WS(t *testing.T) {
	c := types.Canonical{
		Protocol: types.ProtocolTrojan,
		Transport: &types.Transport{
			Kind: types.TransportWS,
			Path: "/ws",
			Host: "ws.example.com",
		},
		TLS: &types.TLSConfig{
			Enabled:    true,
			ServerName: "ws.example.com",
		},
		Proto: &types.ProtoOpts{
			Password: "trojan-pass",
		},
	}

	p := makeParams("trojan-ws", c, "trojan.example.com", 443)
	result := mustRender(t, p)

	ob, ok := getOutboundByTag(result, "trojan-ws")
	if !ok {
		t.Fatal("outbound trojan-ws не найден")
	}
	if ob["protocol"] != "trojan" {
		t.Errorf("protocol = %v, ожидалось trojan", ob["protocol"])
	}

	// Проверяем security=tls
	sec, ok := getPath(ob, "streamSettings", "security")
	if !ok {
		t.Fatal("streamSettings.security отсутствует")
	}
	if sec != "tls" {
		t.Errorf("security = %v, ожидалось tls", sec)
	}

	// Проверяем network=ws
	net, ok := getPath(ob, "streamSettings", "network")
	if !ok {
		t.Fatal("streamSettings.network отсутствует")
	}
	if net != "ws" {
		t.Errorf("network = %v, ожидалось ws", net)
	}

	// Проверяем wsSettings
	wsPath, ok := getPath(ob, "streamSettings", "wsSettings", "path")
	if !ok {
		t.Fatal("wsSettings.path отсутствует")
	}
	if wsPath != "/ws" {
		t.Errorf("wsSettings.path = %v, ожидалось /ws", wsPath)
	}
}

// TestRender_VMess_TCP_NoTLS проверяет простой VMess без TLS.
func TestRender_VMess_TCP_NoTLS(t *testing.T) {
	c := types.Canonical{
		Protocol: types.ProtocolVMess,
		Proto: &types.ProtoOpts{
			UUID: "vmess-uuid",
		},
	}

	p := makeParams("vmess-plain", c, "vmess.example.com", 10086)
	result := mustRender(t, p)

	ob, ok := getOutboundByTag(result, "vmess-plain")
	if !ok {
		t.Fatal("outbound vmess-plain не найден")
	}
	if ob["protocol"] != "vmess" {
		t.Errorf("protocol = %v, ожидалось vmess", ob["protocol"])
	}

	// Проверяем security=none (нет TLS)
	sec, ok := getPath(ob, "streamSettings", "security")
	if !ok {
		t.Fatal("streamSettings.security отсутствует")
	}
	if sec != "none" {
		t.Errorf("security = %v, ожидалось none", sec)
	}

	// Проверяем UUID
	vnext, _ := getPath(ob, "settings", "vnext")
	users := vnext.([]any)[0].(map[string]any)["users"].([]any)
	user := users[0].(map[string]any)
	if user["id"] != "vmess-uuid" {
		t.Errorf("users[0].id = %v, ожидалось vmess-uuid", user["id"])
	}
	if user["security"] != "auto" {
		t.Errorf("users[0].security = %v, ожидалось auto", user["security"])
	}
}

// TestRender_Direct_NoSettings проверяет protocol=freedom без settings.
func TestRender_Direct_NoSettings(t *testing.T) {
	c := types.Canonical{
		Protocol: types.ProtocolDirect,
	}

	p := DefaultConfigParams()
	p.DefaultTag = "direct"
	p.Outbounds = []types.Outbound{
		{Tag: "direct"},
	}
	p.Canonicals = map[string]types.Canonical{"direct": c}

	result := mustRender(t, p)

	ob, ok := getOutboundByTag(result, "direct")
	if !ok {
		t.Fatal("outbound direct не найден")
	}
	if ob["protocol"] != "freedom" {
		t.Errorf("protocol = %v, ожидалось freedom", ob["protocol"])
	}
	// settings должен отсутствовать для direct
	if _, hasSettings := ob["settings"]; hasSettings {
		t.Error("direct outbound не должен иметь settings")
	}
}

// TestRender_TproxyInbound_HasFwmark проверяет что inbound содержит sockopt.mark=83.
func TestRender_TproxyInbound_HasFwmark(t *testing.T) {
	p := DefaultConfigParams()
	// Нет outbound'ов для простоты
	p.Outbounds = nil
	p.Canonicals = nil

	data, err := Render(p)
	if err != nil {
		t.Fatalf("Render вернул ошибку: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("невозможно разобрать JSON: %v", err)
	}

	inbounds, ok := result["inbounds"].([]any)
	if !ok || len(inbounds) == 0 {
		t.Fatal("inbounds пуст или отсутствует")
	}
	inbound := inbounds[0].(map[string]any)

	if inbound["tag"] != "tproxy-in" {
		t.Errorf("inbound tag = %v, ожидалось tproxy-in", inbound["tag"])
	}
	if inbound["protocol"] != "dokodemo-door" {
		t.Errorf("inbound protocol = %v, ожидалось dokodemo-door", inbound["protocol"])
	}

	// Проверяем mark=83 (0x53)
	mark, ok := getPath(inbound, "streamSettings", "sockopt", "mark")
	if !ok {
		t.Fatal("streamSettings.sockopt.mark отсутствует")
	}
	// JSON числа декодируются как float64
	markFloat, ok := mark.(float64)
	if !ok {
		t.Fatalf("mark не float64: %T %v", mark, mark)
	}
	if markFloat != 83 {
		t.Errorf("sockopt.mark = %v, ожидалось 83", markFloat)
	}

	// Проверяем tproxy mode
	tproxyMode, ok := getPath(inbound, "streamSettings", "sockopt", "tproxy")
	if !ok {
		t.Fatal("streamSettings.sockopt.tproxy отсутствует")
	}
	if tproxyMode != "tproxy" {
		t.Errorf("sockopt.tproxy = %v, ожидалось tproxy", tproxyMode)
	}

	// Проверяем порт по умолчанию
	port, ok := inbound["port"]
	if !ok {
		t.Fatal("port отсутствует в inbound")
	}
	portFloat, ok := port.(float64)
	if !ok {
		t.Fatalf("port не float64: %T %v", port, port)
	}
	if portFloat != 7895 {
		t.Errorf("port = %v, ожидалось 7895", portFloat)
	}
}

// TestValidate_Rejects_Hysteria2 проверяет отклонение Hysteria2.
func TestValidate_Rejects_Hysteria2(t *testing.T) {
	c := types.Canonical{Protocol: types.ProtocolHysteria2}
	err := Validate(c)
	if err == nil {
		t.Fatal("Validate должен вернуть ошибку для Hysteria2")
	}
	if !strings.Contains(err.Error(), "Hysteria2") {
		t.Errorf("ошибка должна содержать 'Hysteria2', получено: %v", err)
	}
	if !strings.Contains(err.Error(), "--core sing-box") {
		t.Errorf("ошибка должна содержать '--core sing-box', получено: %v", err)
	}
}

// TestValidate_Rejects_TUIC_v5 проверяет отклонение TUIC v5.
func TestValidate_Rejects_TUIC_v5(t *testing.T) {
	c := types.Canonical{Protocol: types.ProtocolTUIC}
	err := Validate(c)
	if err == nil {
		t.Fatal("Validate должен вернуть ошибку для TUIC")
	}
	if !strings.Contains(err.Error(), "TUIC") {
		t.Errorf("ошибка должна содержать 'TUIC', получено: %v", err)
	}
}

// TestValidate_Rejects_WireGuard проверяет отклонение WireGuard.
func TestValidate_Rejects_WireGuard(t *testing.T) {
	c := types.Canonical{Protocol: types.ProtocolWireGuard}
	err := Validate(c)
	if err == nil {
		t.Fatal("Validate должен вернуть ошибку для WireGuard")
	}
	if !strings.Contains(err.Error(), "WireGuard") {
		t.Errorf("ошибка должна содержать 'WireGuard', получено: %v", err)
	}
}

// TestRender_MissingCanonical_Errors проверяет понятную ошибку при отсутствии canonical.
func TestRender_MissingCanonical_Errors(t *testing.T) {
	p := DefaultConfigParams()
	p.DefaultTag = "proxy"
	p.Outbounds = []types.Outbound{
		{Tag: "proxy", Server: "1.2.3.4", Port: 443},
	}
	p.Canonicals = map[string]types.Canonical{} // пустой — canonical отсутствует

	_, err := Render(p)
	if err == nil {
		t.Fatal("Render должен вернуть ошибку при отсутствии canonical")
	}
	if !strings.Contains(err.Error(), "proxy") {
		t.Errorf("ошибка должна содержать тег 'proxy', получено: %v", err)
	}
	if !strings.Contains(err.Error(), "canonical отсутствует") {
		t.Errorf("ошибка должна содержать 'canonical отсутствует', получено: %v", err)
	}
}

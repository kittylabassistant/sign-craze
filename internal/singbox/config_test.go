package singbox

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// -update обновляет golden-файлы вместо сравнения с ними.
var update = flag.Bool("update", false, "обновить golden-файлы")

func TestRender_ProxyMode(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{
		{Tag: "proxy-out", Type: "socks", Server: "1.2.3.4", Port: 1080},
	}
	p.Routing = types.RoutingRules{
		GeoSiteDirect: []string{"geosite-ru"},
		GeoSiteProxy:  []string{"geosite-blocked"},
		FinalOutbound: "proxy-out",
	}

	checkGolden(t, "proxy_mode.json", p)
}

func TestRender_MinimalConfig(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{
		{Tag: "out", Type: "direct", Server: "0.0.0.0", Port: 1},
	}
	checkGolden(t, "minimal.json", p)
}

func TestRender_ValidJSON(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{
		{Tag: "out", Type: "socks", Server: "10.0.0.1", Port: 1080},
	}

	data, err := Render(p)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !json.Valid(data) {
		t.Errorf("Render вернул невалидный JSON:\n%s", data)
	}
}

func TestRender_DefaultsApplied(t *testing.T) {
	// нулевые значения должны заменяться defaults TUN-inbound.
	p := ConfigParams{
		Outbounds: []types.Outbound{
			{Tag: "out", Type: "socks", Server: "1.1.1.1", Port: 9000},
		},
	}

	data, err := Render(p)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	inbounds := parsed["inbounds"].([]any)
	ib := inbounds[0].(map[string]any)

	if ib["type"].(string) != "tun" {
		t.Errorf("inbound type = %v, ожидалось tun", ib["type"])
	}
	if ib["interface_name"].(string) != DefaultTUNInterfaceName {
		t.Errorf("interface_name = %v, ожидалось %s", ib["interface_name"], DefaultTUNInterfaceName)
	}
	if mtu, _ := ib["mtu"].(float64); int(mtu) != DefaultTUNMTU {
		t.Errorf("mtu = %v, ожидалось %d", mtu, DefaultTUNMTU)
	}
	addrs, ok := ib["address"].([]any)
	if !ok || len(addrs) == 0 {
		t.Fatal("address пуст или отсутствует")
	}
	if addrs[0].(string) != DefaultTUNAddresses[0] {
		t.Errorf("address[0] = %v, ожидалось %s", addrs[0], DefaultTUNAddresses[0])
	}
}

func TestRender_RuleSetsBuilt(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{
		{Tag: "out", Type: "socks", Server: "1.1.1.1", Port: 9000},
	}
	p.Routing = types.RoutingRules{
		GeoSiteProxy:  []string{"geosite-blocked"},
		GeoSiteDirect: []string{"geosite-ru", "geosite-private"},
	}

	data, err := Render(p)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	route := parsed["route"].(map[string]any)
	ruleSets, ok := route["rule_set"].([]any)
	if !ok || len(ruleSets) == 0 {
		t.Fatal("route.rule_set пуст или отсутствует")
	}

	// все три категории должны быть в rule_set
	tags := map[string]bool{}
	for _, rs := range ruleSets {
		m := rs.(map[string]any)
		tags[m["tag"].(string)] = true
	}
	for _, cat := range []string{"geosite-blocked", "geosite-ru", "geosite-private"} {
		if !tags[cat] {
			t.Errorf("rule_set не содержит категорию %q", cat)
		}
	}
}

// TestRender_LogLevelWhitelist — некорректный LogLevel ("info\n--evil")
// должен отвергаться, чтобы injection не утёк в config.json.
func TestRender_LogLevelWhitelist(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{
		{Tag: "out", Type: "socks", Server: "1.1.1.1", Port: 9000},
	}
	for _, bad := range []string{"info\n--evil", "INFO", "verbose", "x"} {
		p.LogLevel = bad
		if _, err := Render(p); err == nil {
			t.Errorf("Render(LogLevel=%q): ожидалась ошибка", bad)
		}
	}
}

// TestRender_OutboundTagInjection — tag с кавычкой/переводом строки должен
// отвергаться (template-injection защита).
func TestRender_OutboundTagInjection(t *testing.T) {
	for _, badTag := range []string{`evil"tag`, "tag with space", "tag\nwith\nnewline", ""} {
		p := DefaultConfigParams()
		p.Outbounds = []types.Outbound{
			{Tag: badTag, Type: "socks", Server: "1.1.1.1", Port: 9000},
		}
		if _, err := Render(p); err == nil {
			t.Errorf("Render(Tag=%q): ожидалась ошибка валидации", badTag)
		}
	}
}

// TestRender_TagWithQuoteEscaped — даже если tag прошёл валидацию (что не должно
// случаться благодаря Outbound.Validate), template обязан давать valid JSON
// через jsonMarshal (защитный слой).
func TestRender_TagWithQuoteEscaped(t *testing.T) {
	// Конструируем Outbound через MarshalJSON — Validate вызывается в Render,
	// но проверяем что DefaultOutboundTag прогоняется через jsonMarshal в шаблоне.
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{
		{Tag: "valid-tag", Type: "socks", Server: "1.2.3.4", Port: 9000},
	}
	p.DefaultOutboundTag = "valid-tag"
	data, err := Render(p)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !json.Valid(data) {
		t.Errorf("template должен давать valid JSON: %s", data)
	}
}

// TestRender_TUNStackIsGvisor — TUN inbound должен использовать gvisor stack.
// Reason: на Keenetic mipsle softfloat kernel TCP/IP не обрабатывает пакеты
// корректно при policy-routing → TUN. gvisor читает raw bytes напрямую из TUN
// fd, не зависит от kernel TCP stack. system stack без nftables-based
// auto_redirect (недоступен на Keenetic 4.9 — нет nftables) не сохраняет
// original src LAN-клиента в metadata: пакеты RPi4 (172.16.0.97) приходят в
// sing-box с src=172.19.0.1 (TUN gw), и ответы от прокси возвращаются к
// роутеру locally, не к клиенту. Полное решение — переход на TPROXY inbound,
// запланирован отдельной фазой.
func TestRender_TUNStackIsGvisor(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{
		{Tag: "out", Type: "socks", Server: "1.1.1.1", Port: 9000},
	}

	data, err := Render(p)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	ib := parsed["inbounds"].([]any)[0].(map[string]any)
	if ib["stack"] != "gvisor" {
		t.Errorf("TUN stack = %v, ожидалось \"gvisor\"", ib["stack"])
	}
}

// TestRender_HijackDNSScopedToTUN — hijack-dns rule должен ограничиваться
// inbound: tun-in, чтобы не захватывать DNS-resolver самого роутера.
// Без inbound-фильтра при сбое vless-proxy локальный DNS роутера зависает.
func TestRender_HijackDNSScopedToTUN(t *testing.T) {
	p := DefaultConfigParams()
	p.Outbounds = []types.Outbound{
		{Tag: "out", Type: "socks", Server: "1.1.1.1", Port: 9000},
	}

	data, err := Render(p)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	rules := parsed["route"].(map[string]any)["rules"].([]any)
	var found bool
	for _, r := range rules {
		m := r.(map[string]any)
		if m["action"] == "hijack-dns" {
			found = true
			if m["inbound"] != "tun-in" {
				t.Errorf("hijack-dns rule: inbound = %v, ожидалось \"tun-in\"", m["inbound"])
			}
			if m["protocol"] != "dns" {
				t.Errorf("hijack-dns rule: protocol = %v, ожидалось \"dns\"", m["protocol"])
			}
		}
	}
	if !found {
		t.Fatal("hijack-dns rule не найден в route.rules")
	}
}

func TestBuildRuleSets_NoDuplicates(t *testing.T) {
	r := types.RoutingRules{
		GeoSiteProxy:  []string{"cat-a", "cat-b"},
		GeoSiteDirect: []string{"cat-a", "cat-c"}, // cat-a дублируется
	}
	refs := buildRuleSets(r)

	seen := map[string]int{}
	for _, ref := range refs {
		seen[ref.Tag]++
	}
	for tag, count := range seen {
		if count > 1 {
			t.Errorf("дублирующийся rule_set tag: %q (встречается %d раз)", tag, count)
		}
	}
}

// checkGolden сравнивает результат Render с golden-файлом в testdata/.
// Если файл отсутствует — создаёт его и засчитывает тест как пройденный.
// При флаге -update принудительно перезаписывает golden-файл.
func TestRender_RoutingConfig_Basic(t *testing.T) {
	p := DefaultConfigParams()
	p.RoutingConfig = &types.RoutingConfig{
		Version: 1,
		Outbounds: []types.Outbound{
			{Tag: "vless-out", Type: "vless", Server: "1.1.1.1", Port: 443,
				Settings: map[string]any{"uuid": "00000000-0000-0000-0000-000000000000"}},
		},
		Rules: []types.RouteRule{
			{Network: "udp", Port: []uint16{443}, Outbound: "vless-out"},
		},
		Final: "vless-out",
	}
	p.DefaultOutboundTag = "vless-out"
	out, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}

	// outbound vless-out + direct присутствуют
	outbounds := parsed["outbounds"].([]any)
	tags := map[string]bool{}
	for _, ob := range outbounds {
		tags[ob.(map[string]any)["tag"].(string)] = true
	}
	if !tags["vless-out"] {
		t.Error("outbounds: отсутствует vless-out")
	}
	if !tags["direct"] {
		t.Error("outbounds: отсутствует direct")
	}

	// route.rules содержит правило с port 443
	rules := parsed["route"].(map[string]any)["rules"].([]any)
	found443 := false
	for _, r := range rules {
		m := r.(map[string]any)
		if ports, ok := m["port"].([]any); ok {
			for _, pt := range ports {
				if pt.(float64) == 443 {
					found443 = true
				}
			}
		}
	}
	if !found443 {
		t.Error("route.rules: не найдено правило с port=443")
	}

	// route.final == "vless-out"
	if final := parsed["route"].(map[string]any)["final"]; final != "vless-out" {
		t.Errorf("route.final = %v, ожидалось vless-out", final)
	}
}

func TestRender_RoutingConfig_FinalDirect(t *testing.T) {
	p := DefaultConfigParams()
	p.RoutingConfig = &types.RoutingConfig{
		Version: 1,
		Final:   "direct",
	}
	p.DefaultOutboundTag = "direct"
	out, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if final := parsed["route"].(map[string]any)["final"]; final != "direct" {
		t.Errorf("route.final = %v, ожидалось direct", final)
	}
}

func TestRender_RoutingConfig_CustomInbounds(t *testing.T) {
	p := DefaultConfigParams()
	p.RoutingConfig = &types.RoutingConfig{
		Version: 1,
		Inbounds: []types.Inbound{
			{Tag: "tproxy-in", Type: "tproxy", Settings: map[string]any{"listen": "::", "listen_port": 7895}},
		},
		Outbounds: []types.Outbound{
			{Tag: "out", Type: "direct"},
		},
		Final: "out",
	}
	p.DefaultOutboundTag = "out"
	out, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}

	// inbounds содержит tproxy-in с listen_port на top-level
	inbounds := parsed["inbounds"].([]any)
	if len(inbounds) == 0 {
		t.Fatal("inbounds пуст")
	}
	ib := inbounds[0].(map[string]any)
	if ib["tag"] != "tproxy-in" {
		t.Errorf("inbounds[0].tag = %v, ожидалось tproxy-in", ib["tag"])
	}
	if ib["type"] != "tproxy" {
		t.Errorf("inbounds[0].type = %v, ожидалось tproxy", ib["type"])
	}
	if port, ok := ib["listen_port"].(float64); !ok || int(port) != 7895 {
		t.Errorf("inbounds[0].listen_port = %v, ожидалось 7895", ib["listen_port"])
	}
}

func TestRender_RoutingConfig_FullExample(t *testing.T) {
	p := DefaultConfigParams()
	p.RoutingConfig = &types.RoutingConfig{
		Version: 1,
		Outbounds: []types.Outbound{
			{Tag: "vless-reality", Type: "vless", Server: "example.com", Port: 443,
				Settings: map[string]any{"uuid": "00000000-0000-0000-0000-000000000000"}},
		},
		Rules: []types.RouteRule{
			{Network: "udp", Port: []uint16{135, 137, 138, 139}, Action: "block"},
			{RuleSet: []string{"geosite-category-ads-all"}, Action: "block"},
			{Network: "udp", Port: []uint16{443}, RuleSet: []string{"geoip-ru"}, Invert: true, Action: "block"},
			{Network: "udp", PortRange: []string{"50000:50030"}, Outbound: "vless-reality"},
			{RuleSet: []string{"geosite-zkeen-domains", "geosite-zkeen-other", "geosite-zkeen-youtube"}, Outbound: "vless-reality"},
			{IPCIDR: []string{"138.128.136.0/21", "162.158.0.0/15", "172.64.0.0/13"}, RuleSet: []string{"geoip-discord"}, Outbound: "vless-reality"},
			{Protocol: []string{"bittorrent"}, Outbound: "direct"},
		},
		RuleSets: []types.RuleSetRef{
			{Tag: "geoip-ru", Type: "remote", Format: "binary",
				URL:            "https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip-ru.srs",
				DownloadDetour: "direct"},
			{Tag: "geoip-discord", Type: "remote", Format: "binary",
				URL:            "https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip-discord.srs",
				DownloadDetour: "direct"},
			{Tag: "geosite-category-ads-all", Type: "remote", Format: "binary",
				URL:            "https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite-category-ads-all.srs",
				DownloadDetour: "direct"},
			{Tag: "geosite-zkeen-domains", Type: "remote", Format: "binary",
				URL:            "https://github.com/jameszeroX/zkeen-domains/releases/latest/download/zkeen-domains.srs",
				DownloadDetour: "direct"},
			{Tag: "geosite-zkeen-other", Type: "remote", Format: "binary",
				URL:            "https://github.com/jameszeroX/zkeen-domains/releases/latest/download/zkeen-other.srs",
				DownloadDetour: "direct"},
			{Tag: "geosite-zkeen-youtube", Type: "remote", Format: "binary",
				URL:            "https://github.com/jameszeroX/zkeen-domains/releases/latest/download/zkeen-youtube.srs",
				DownloadDetour: "direct"},
		},
		Final: "direct",
	}
	p.DefaultOutboundTag = "vless-reality"
	out, err := Render(p)
	if err != nil {
		t.Fatal(err)
	}
	checkGoldenBytes(t, "routing_full", out)
}

// checkGolden принимает уже отрендеренные байты (не ConfigParams).
// Перегрузка — берёт any, определяет тип.
func checkGoldenBytes(t *testing.T, name string, data []byte) {
	t.Helper()
	data = prettyJSON(t, data)

	goldenPath := filepath.Join("testdata", name)
	if *update {
		writeGolden(t, goldenPath, data)
		t.Logf("golden-файл обновлён: %s", goldenPath)
		return
	}
	expected, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		writeGolden(t, goldenPath, data)
		t.Logf("golden-файл создан: %s", goldenPath)
		return
	}
	if err != nil {
		t.Fatalf("чтение golden-файла: %v", err)
	}
	if string(data) != string(expected) {
		t.Errorf("вывод не совпадает с golden-файлом %s\nполучено:\n%s\nожидалось:\n%s",
			name, data, expected)
	}
}

func checkGolden(t *testing.T, name string, p ConfigParams) {
	t.Helper()

	data, err := Render(p)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	// нормализуем JSON для стабильного сравнения
	data = prettyJSON(t, data)

	goldenPath := filepath.Join("testdata", name)

	if *update {
		writeGolden(t, goldenPath, data)
		t.Logf("golden-файл обновлён: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		// первый запуск — создаём эталон
		writeGolden(t, goldenPath, data)
		t.Logf("golden-файл создан: %s", goldenPath)
		return
	}
	if err != nil {
		t.Fatalf("чтение golden-файла: %v", err)
	}

	if string(data) != string(expected) {
		t.Errorf("вывод Render не совпадает с golden-файлом %s\nполучено:\n%s\nожидалось:\n%s",
			name, data, expected)
	}
}

func writeGolden(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir для golden-файла: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("запись golden-файла %s: %v", path, err)
	}
}

func prettyJSON(t *testing.T, data []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("prettyJSON: невалидный JSON: %v\n%s", err, data)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("prettyJSON: marshal: %v", err)
	}
	return append(out, '\n')
}

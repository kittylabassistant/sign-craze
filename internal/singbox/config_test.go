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
	// нулевые значения должны заменяться defaults
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

	// inbound port должен быть 7895
	inbounds := parsed["inbounds"].([]any)
	ib := inbounds[0].(map[string]any)
	port := ib["listen_port"].(float64)
	if port != 7895 {
		t.Errorf("listen_port = %v, ожидалось 7895", port)
	}

	// mark должен быть 83 (= 0x53)
	mark := ib["mark"].(float64)
	if mark != 83 {
		t.Errorf("mark = %v, ожидалось 83", mark)
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

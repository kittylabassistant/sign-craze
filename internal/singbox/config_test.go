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
// При флаге -update записывает новый golden-файл.
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
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, data, 0o644); err != nil {
			t.Fatalf("запись golden-файла: %v", err)
		}
		t.Logf("golden-файл обновлён: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden-файл не найден: %s (запустите с -update для создания)", goldenPath)
	}

	if string(data) != string(expected) {
		t.Errorf("вывод Render не совпадает с golden-файлом %s\nполучено:\n%s\nожидалось:\n%s",
			name, data, expected)
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

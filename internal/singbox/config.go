package singbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"text/template"

	_ "embed"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

//go:embed templates/tproxy.json.tmpl
var tproxyTmpl string

// ConfigParams задаёт параметры для генерации конфига sing-box.
type ConfigParams struct {
	Mode               types.Mode
	InboundPort        uint16          // по умолчанию 7895
	Mark               uint32          // fwmark; по умолчанию 0x53 = 83
	LogLevel           string          // "info", "debug", "warn", "error"
	Outbounds          []types.Outbound
	Routing            types.RoutingRules
	DefaultOutboundTag string          // тег первого outbound, используемый как final
	// RuleSets — список дескрипторов rule_set для секции route.rule_set.
	// Генерируется автоматически из Routing.GeoSiteProxy + GeoSiteDirect.
	RuleSets []ruleSetRef
}

type ruleSetRef struct {
	Tag            string `json:"tag"`
	Type           string `json:"type"`
	Format         string `json:"format"`
	URL            string `json:"url"`
	DownloadDetour string `json:"download_detour"`
}

// DefaultConfigParams возвращает ConfigParams с разумными значениями по умолчанию.
func DefaultConfigParams() ConfigParams {
	return ConfigParams{
		Mode:        types.ModeProxy,
		InboundPort: 7895,
		Mark:        0x53,
		LogLevel:    "info",
	}
}

// Render генерирует содержимое config.json из шаблона и параметров.
// Возвращает валидный JSON или ошибку.
func Render(p ConfigParams) ([]byte, error) {
	if p.InboundPort == 0 {
		p.InboundPort = 7895
	}
	if p.Mark == 0 {
		p.Mark = 0x53
	}
	if p.LogLevel == "" {
		p.LogLevel = "info"
	}
	if p.DefaultOutboundTag == "" && len(p.Outbounds) > 0 {
		p.DefaultOutboundTag = p.Outbounds[0].Tag
	}
	p.RuleSets = buildRuleSets(p.Routing)

	funcMap := template.FuncMap{
		"jsonMarshal": func(v any) (string, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
	}

	tmpl, err := template.New("config").Funcs(funcMap).Parse(tproxyTmpl)
	if err != nil {
		return nil, fmt.Errorf("singbox config: парсинг шаблона: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, p); err != nil {
		return nil, fmt.Errorf("singbox config: рендеринг шаблона: %w", err)
	}

	out := buf.Bytes()

	// проверяем, что результат — валидный JSON
	if !json.Valid(out) {
		return nil, fmt.Errorf("singbox config: сгенерированный JSON невалиден")
	}

	return out, nil
}

// WriteConfig рендерит конфиг и атомарно записывает его в path.
func WriteConfig(ctx context.Context, runner exectx.Runner, p ConfigParams, binPath, configPath string) error {
	data, err := Render(p)
	if err != nil {
		return err
	}

	if err := atomicfs.WriteFileAtomic(configPath, data, 0o640); err != nil {
		return fmt.Errorf("singbox config: запись файла: %w", err)
	}

	// валидация через sing-box check -c
	if err := checkConfig(ctx, runner, binPath, configPath); err != nil {
		return fmt.Errorf("singbox config: %w", err)
	}

	log.L().Info("конфиг sing-box записан", "path", configPath)
	return nil
}

// buildRuleSets формирует список rule_set дескрипторов для секции route.
func buildRuleSets(r types.RoutingRules) []ruleSetRef {
	const (
		ruleSetBaseURL = "https://github.com/kittylabassistant/sign-craze-dats/releases/latest/download/"
		detour         = "direct"
	)

	seen := map[string]bool{}
	var refs []ruleSetRef

	addCategory := func(cat string) {
		if seen[cat] {
			return
		}
		seen[cat] = true
		refs = append(refs, ruleSetRef{
			Tag:            cat,
			Type:           "remote",
			Format:         "binary",
			URL:            ruleSetBaseURL + cat + ".srs",
			DownloadDetour: detour,
		})
	}

	for _, cat := range r.GeoSiteProxy {
		addCategory(cat)
	}
	for _, cat := range r.GeoSiteDirect {
		addCategory(cat)
	}
	for _, cat := range r.GeoSiteDPI {
		addCategory(cat)
	}

	return refs
}

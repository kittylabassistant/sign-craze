package mihomo

import (
	"bytes"
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// ConfigParams — параметры для генерации mihomo config.yaml.
type ConfigParams struct {
	LogLevel   string                     // "info" по умолчанию
	TProxyPort uint16                     // TPROXY-порт sign-craze (7895)
	FWMark     uint32                     // fwmark sign-craze (0x53 = 83)
	Outbounds  []types.Outbound           // legacy: tag/server/port
	Canonicals map[string]types.Canonical // основной источник; ключ = Outbound.Tag
	DefaultTag string                     // тег первого proxy (для proxy-group)
}

// DefaultConfigParams возвращает ConfigParams с разумными значениями по умолчанию.
func DefaultConfigParams() ConfigParams {
	return ConfigParams{
		LogLevel:   "info",
		TProxyPort: 7895,
		FWMark:     83, // 0x53
	}
}

//go:embed templates/config.yaml.tmpl
var configTmpl string

// templateData — данные, передаваемые в шаблон config.yaml.tmpl.
type templateData struct {
	LogLevel   string
	TProxyPort uint16
	FWMark     uint32
	DefaultTag string
	// Proxies — сериализованные YAML-строки для каждого proxy-entry.
	// Шаблон вставляет их через range + yamlInline.
	Proxies    []map[string]any
	ProxyNames []string
}

// Render генерирует mihomo config.yaml как []byte.
//
// Алгоритм:
//  1. Для каждого Outbound в Outbounds:
//     - Если это direct/block — пропускается (не является proxy-entry).
//     - Если Canonical для Tag отсутствует → ошибка "outbound %q: canonical отсутствует".
//     - Иначе renderProxyEntry(outbound, canonical) → map[string]any.
//  2. Передать собранные данные в шаблон config.yaml.tmpl.
func Render(p ConfigParams) ([]byte, error) {
	var proxies []map[string]any
	var proxyNames []string

	for _, ob := range p.Outbounds {
		// direct/block не являются proxy-entry в mihomo
		if ob.Type == "direct" || ob.Type == "block" {
			// Проверяем canonical если есть, чтобы не потерять явные direct canonical
			if c, ok := p.Canonicals[ob.Tag]; ok && c.IsSet() {
				if c.Protocol == types.ProtocolDirect || c.Protocol == types.ProtocolBlock {
					continue
				}
			} else {
				continue
			}
		}

		c, ok := p.Canonicals[ob.Tag]
		if !ok {
			return nil, fmt.Errorf("mihomo render: outbound %q: canonical отсутствует", ob.Tag)
		}

		// direct/block canonical — пропускаем
		if c.Protocol == types.ProtocolDirect || c.Protocol == types.ProtocolBlock {
			continue
		}

		entry, err := renderProxyEntry(ob, c)
		if err != nil {
			return nil, fmt.Errorf("mihomo render: %w", err)
		}
		proxies = append(proxies, entry)
		proxyNames = append(proxyNames, ob.Tag)
	}

	data := templateData{
		LogLevel:   p.LogLevel,
		TProxyPort: p.TProxyPort,
		FWMark:     p.FWMark,
		DefaultTag: p.DefaultTag,
		Proxies:    proxies,
		ProxyNames: proxyNames,
	}

	// Регистрируем функцию yamlInline для шаблона.
	// Сериализует map[string]any в однострочный YAML-mapping (inline).
	funcMap := template.FuncMap{
		"yamlInline": marshalYAMLInline,
	}

	tmpl, err := template.New("config").Funcs(funcMap).Parse(configTmpl)
	if err != nil {
		return nil, fmt.Errorf("mihomo render: parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("mihomo render: execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// marshalYAMLInline сериализует map[string]any в однострочный YAML flow mapping.
// Пример: {name: proxy, type: vless, server: h, port: 443}
//
// Используется шаблоном через функцию yamlInline для вставки proxy-entry.
// Поддерживает: string, bool, int/uint*, float*, []string, []any, map[string]any.
func marshalYAMLInline(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}

	// Сортируем ключи для детерминированного вывода.
	// Приоритетные ключи идут первыми.
	keys := priorityKeys(m)

	var sb strings.Builder
	sb.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(yamlValue(m[k]))
	}
	sb.WriteByte('}')
	return sb.String()
}

// priorityKeys возвращает ключи map в порядке: name, type, server, port — первыми,
// остальные отсортированы алфавитно.
func priorityKeys(m map[string]any) []string {
	priority := []string{"name", "type", "server", "port"}
	seen := make(map[string]bool, len(m))
	result := make([]string, 0, len(m))

	for _, k := range priority {
		if _, ok := m[k]; ok {
			result = append(result, k)
			seen[k] = true
		}
	}

	rest := make([]string, 0, len(m)-len(result))
	for k := range m {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(result, rest...)
}

// yamlValue сериализует произвольное значение в YAML flow-стиль.
func yamlValue(v any) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		return yamlString(val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", val)
	case int8:
		return fmt.Sprintf("%d", val)
	case int16:
		return fmt.Sprintf("%d", val)
	case int32:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case uint:
		return fmt.Sprintf("%d", val)
	case uint8:
		return fmt.Sprintf("%d", val)
	case uint16:
		return fmt.Sprintf("%d", val)
	case uint32:
		return fmt.Sprintf("%d", val)
	case uint64:
		return fmt.Sprintf("%d", val)
	case float32:
		return fmt.Sprintf("%g", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case []string:
		return yamlStringSlice(val)
	case []any:
		return yamlAnySlice(val)
	case map[string]any:
		return marshalYAMLInline(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// yamlString заключает строку в кавычки если она содержит спецсимволы YAML.
func yamlString(s string) string {
	// Символы требующие кавычек
	needsQuote := strings.ContainsAny(s, ": #{}[]|>&*!,'\"\\%@`\n\r\t") ||
		s == "" || s == "true" || s == "false" || s == "null" ||
		s == "yes" || s == "no" || s == "on" || s == "off"
	if needsQuote {
		// Экранируем обратный слэш и двойные кавычки
		escaped := strings.ReplaceAll(s, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}

// yamlStringSlice сериализует []string в YAML flow sequence.
func yamlStringSlice(ss []string) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, s := range ss {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(yamlString(s))
	}
	sb.WriteByte(']')
	return sb.String()
}

// yamlAnySlice сериализует []any в YAML flow sequence.
func yamlAnySlice(ss []any) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, s := range ss {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(yamlValue(s))
	}
	sb.WriteByte(']')
	return sb.String()
}

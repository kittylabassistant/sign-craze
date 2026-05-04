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

//go:embed templates/tun.json.tmpl
var tunTmpl string

// Defaults для TUN-inbound.
//
// MTU 1280 (минимум IPv6) выбран чтобы избежать фрагментации на upstream-пути
// VLESS+Reality+gRPC: outer-encapsulation добавляет ~60-100 байт TLS+gRPC+H2
// overhead, а path MTU до homelabcloud.ru может быть меньше 1500 из-за
// PPPoE/туннелей у провайдера. На mipsle softfloat + gvisor TCP stack крупные
// сегменты (>1280) с PSH ломают handshake (server retransmits 544-byte
// Finished не доставляются клиенту). 1280 даёт запас и для PMTUD-чёрных дыр.
const (
	DefaultTUNInterfaceName = "signbox-tun"
	DefaultTUNMTU           = 1280
)

// DefaultTUNAddresses — IPv4 /30 + IPv6 /126 для transparent gateway внутри TUN.
// 172.19.0.0/30 и fdfe:dcba:9876::/126 не пересекаются с типичными Keenetic LAN-подсетями.
var DefaultTUNAddresses = []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"}

// validLogLevels — разрешённые значения LogLevel sing-box. Whitelisted, чтобы
// state.json с инжекцией ("info\n--evil") не утёк в config.json.
var validLogLevels = map[string]bool{
	"trace": true,
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
	"fatal": true,
	"panic": true,
}

// ConfigParams задаёт параметры для генерации конфига sing-box.
type ConfigParams struct {
	Mode     types.Mode
	LogLevel string // "info", "debug", "warn", "error"

	// TUN inbound: sing-box создаёт TUN-интерфейс при старте. Маршрутизация
	// (ip rule fwmark → table → default dev <TUN>) делается отдельно
	// firewall-слоем; sing-box auto_route отключён.
	TUNInterfaceName string   // default "signbox-tun"
	TUNAddresses     []string // default ["172.19.0.1/30", "fdfe:dcba:9876::1/126"]
	TUNMTU           int      // default 1280

	Outbounds          []types.Outbound
	Routing            types.RoutingRules
	DefaultOutboundTag string // тег первого outbound, используемый как final
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
		Mode:             types.ModePolicy,
		LogLevel:         "info",
		TUNInterfaceName: DefaultTUNInterfaceName,
		TUNAddresses:     append([]string(nil), DefaultTUNAddresses...),
		TUNMTU:           DefaultTUNMTU,
	}
}

// Render генерирует содержимое config.json из шаблона и параметров.
// Возвращает валидный JSON или ошибку.
func Render(p ConfigParams) ([]byte, error) {
	if p.TUNInterfaceName == "" {
		p.TUNInterfaceName = DefaultTUNInterfaceName
	}
	if len(p.TUNAddresses) == 0 {
		p.TUNAddresses = append([]string(nil), DefaultTUNAddresses...)
	}
	if p.TUNMTU == 0 {
		p.TUNMTU = DefaultTUNMTU
	}
	if p.LogLevel == "" {
		p.LogLevel = "info"
	}
	if !validLogLevels[p.LogLevel] {
		return nil, fmt.Errorf("singbox config: некорректный LogLevel %q (допустимо: trace/debug/info/warn/error/fatal/panic)", p.LogLevel)
	}
	if p.DefaultOutboundTag == "" && len(p.Outbounds) > 0 {
		p.DefaultOutboundTag = p.Outbounds[0].Tag
	}
	for _, o := range p.Outbounds {
		if err := o.Validate(); err != nil {
			return nil, fmt.Errorf("singbox config: %w", err)
		}
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

	tmpl, err := template.New("config").Funcs(funcMap).Parse(tunTmpl)
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
		ruleSetBaseURL = "https://github.com/kittylabassistant/sign-craze-dat/releases/latest/download/"
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

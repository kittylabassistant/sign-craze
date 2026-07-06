package types

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"runtime"
)

// outboundTagRe — допустимый формат тега outbound. Запрещаем кавычки, пробелы,
// JSON-meta-символы — это устраняет template-injection в config.json (sing-box
// требует JSON-валидного тега в строковых полях rules/dns).
var outboundTagRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

// Mode — режим маршрутизации трафика.
type Mode string

const (
	// ModePolicy — режим интеграции с Keenetic IP Policy.
	// Sign-craze создаёт policy в RCI Keenetic, читает присвоенный mark
	// и ставит TPROXY-фильтр по этому mark. Выбор устройств идёт через
	// штатный web-UI Keenetic «Приоритеты подключений».
	ModePolicy Mode = "policy"
	// ModeFull — legacy-режим: ipset signcraze_ipv4/v6 по dst-IP +
	// fwmark 0x53 + signcraze chains, плюс опционально NFQUEUE/nfqws2
	// при включённом --dpi on. Эквивалент бывшего hybrid.
	ModeFull Mode = "full"
)

// LegacyModes — устаревшие значения режима, мигрируемые в ModePolicy
// при загрузке state. Константы удалены, миграция работает через literal-сравнение.
var LegacyModes = map[Mode]Mode{
	"proxy":  ModePolicy,
	"dpi":    ModePolicy,
	"hybrid": ModePolicy,
}

// Validate проверяет допустимость значения Mode.
func (m Mode) Validate() error {
	switch m {
	case ModePolicy, ModeFull:
		return nil
	default:
		return fmt.Errorf("неизвестный режим %q (допустимо: policy, full)", m)
	}
}

// Arch — целевая архитектура бинаря.
type Arch string

const (
	ArchARM64  Arch = "arm64"
	ArchARM7   Arch = "arm7"
	ArchMIPSLE Arch = "mipsle"
	ArchMIPS   Arch = "mips"
	ArchAMD64  Arch = "amd64" // только для self-test в Docker
)

// Validate проверяет допустимость архитектуры.
func (a Arch) Validate() error {
	switch a {
	case ArchARM64, ArchARM7, ArchMIPSLE, ArchMIPS, ArchAMD64:
		return nil
	default:
		return fmt.Errorf("неподдерживаемая архитектура %q", a)
	}
}

// DetectHostArch возвращает архитектуру текущего бинаря через runtime.GOARCH.
// Используется для выбора правильного asset при загрузке (sing-box, nfqws2).
func DetectHostArch() (Arch, error) {
	switch runtime.GOARCH {
	case "arm64":
		return ArchARM64, nil
	case "arm":
		return ArchARM7, nil
	case "mipsle":
		return ArchMIPSLE, nil
	case "mips":
		return ArchMIPS, nil
	case "amd64":
		return ArchAMD64, nil
	default:
		return "", fmt.Errorf("неподдерживаемая host-архитектура %q", runtime.GOARCH)
	}
}

// GoEnv возвращает переменные окружения Go для кросс-компиляции под данную архитектуру.
func (a Arch) GoEnv() map[string]string {
	switch a {
	case ArchARM64:
		return map[string]string{"GOOS": "linux", "GOARCH": "arm64"}
	case ArchARM7:
		return map[string]string{"GOOS": "linux", "GOARCH": "arm", "GOARM": "7"}
	case ArchMIPSLE:
		return map[string]string{"GOOS": "linux", "GOARCH": "mipsle", "GOMIPS": "softfloat"}
	case ArchMIPS:
		return map[string]string{"GOOS": "linux", "GOARCH": "mips", "GOMIPS": "softfloat"}
	case ArchAMD64:
		return map[string]string{"GOOS": "linux", "GOARCH": "amd64"}
	default:
		return nil
	}
}

// Port — номер TCP/UDP-порта.
type Port uint16

// Validate проверяет, что порт ненулевой.
func (p Port) Validate() error {
	if p == 0 {
		return fmt.Errorf("порт не может быть 0")
	}
	return nil
}

// PortRange — диапазон портов [From, To] включительно.
type PortRange struct {
	From Port
	To   Port
}

// Validate проверяет корректность диапазона.
func (r PortRange) Validate() error {
	if err := r.From.Validate(); err != nil {
		return fmt.Errorf("начало диапазона: %w", err)
	}
	if err := r.To.Validate(); err != nil {
		return fmt.Errorf("конец диапазона: %w", err)
	}
	if r.From > r.To {
		return fmt.Errorf("начало диапазона %d > конца %d", r.From, r.To)
	}
	return nil
}

// IPSetName — имя ipset-набора.
type IPSetName string

const (
	IPSetIPv4 IPSetName = "signcraze_ipv4"
	IPSetIPv6 IPSetName = "signcraze_ipv6"
)

// Outbound описывает исходящее соединение.
//
// Поля Protocol/Transport/TLS/Proto — canonical-представление, общее между
// всеми ядрами (sing-box, xray, mihomo). Заполняются парсером URL (proxyparse).
// Settings — legacy: используется как fallback, когда Protocol пустой
// (старые state.json, импортированные до Phase D).
//
// При маршалинге в state.json используются стандартные struct-теги Go.
// Для рендера sing-box config.json используется renderOutboundJSON в
// internal/singbox/render.go — она знает про flatten Settings и canonical-путь.
type Outbound struct {
	Tag      string         `json:"tag"`                   // уникальный тег (обязателен)
	Type     string         `json:"type"`                  // "socks", "vmess", "vless", "shadowsocks" и т.д.
	Server   string         `json:"server,omitempty"`      // не требуется для type=direct
	Port     Port           `json:"server_port,omitempty"` // sing-box/state.json имя (legacy "port" мигрируется при Load)
	Settings map[string]any `json:"settings,omitempty"`    // legacy: параметры до Phase D

	// Canonical-поля (Phase D.4): заполняются parserом из URL, используются
	// всеми ядрами как primary источник (fallback — Settings, если Protocol=="").
	Protocol  Protocol   `json:"protocol,omitempty"`
	Transport *Transport `json:"transport,omitempty"`
	TLS       *TLSConfig `json:"tls,omitempty"`
	Proto     *ProtoOpts `json:"proto,omitempty"`
}

// UnmarshalJSON обеспечивает совместимость при чтении legacy state.json:
// поле "port" (старое имя) распознаётся наравне с "server_port" (новое имя).
// После Phase D.4 state.json пишет только "server_port"; "port" встречается
// только в файлах, созданных до рефактора.
func (o *Outbound) UnmarshalJSON(data []byte) error {
	// outboundAlias — alias без UnmarshalJSON, чтобы избежать рекурсии.
	type outboundAlias Outbound
	var a outboundAlias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*o = Outbound(a)

	// legacy compat: если "server_port" пустой — пробуем "port".
	if o.Port == 0 {
		var raw struct {
			Port Port `json:"port"`
		}
		if err := json.Unmarshal(data, &raw); err == nil && raw.Port != 0 {
			o.Port = raw.Port
		}
	}
	return nil
}

// Validate проверяет обязательные поля Outbound.
//
// Server/Port не требуются для type=direct/block/dns (служебные outbound'ы
// sing-box без удалённого endpoint'а).
func (o Outbound) Validate() error {
	if o.Tag == "" {
		return fmt.Errorf("outbound.Tag не может быть пустым")
	}
	if !outboundTagRe.MatchString(o.Tag) {
		return fmt.Errorf("outbound.Tag %q: допустимы только [a-zA-Z0-9._-], 1-64 символов", o.Tag)
	}
	if o.Type == "" {
		return fmt.Errorf("outbound.Type не может быть пустым")
	}
	switch o.Type {
	case "direct", "block", "dns":
		return nil
	}
	if o.Server == "" {
		return fmt.Errorf("outbound.Server не может быть пустым (type=%s)", o.Type)
	}
	return o.Port.Validate()
}

// RoutingRules содержит настройки маршрутизации sing-box.
type RoutingRules struct {
	// GeoSiteProxy — список geosite-категорий, направляемых через прокси.
	GeoSiteProxy []string
	// GeoSiteDPI — список geosite-категорий, направляемых через DPI-обход (только hybrid).
	GeoSiteDPI []string
	// GeoSiteDirect — список geosite-категорий для прямого подключения.
	GeoSiteDirect []string
	// ExcludeIPs — IP-адреса/подсети, всегда обходящие прокси.
	ExcludeIPs []netip.Prefix
	// FinalOutbound — тег outbound для трафика, не совпавшего ни с одним правилом.
	FinalOutbound string
}

// Release описывает релиз на GitHub.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset — отдельный файл релиза на GitHub.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// CoreRenderParams — параметры для Core.RenderConfig.
//
// Содержит только поля из pkg/types, чтобы не создавать circular import
// (internal/core → internal/state → internal/web → internal/core).
// CLI заполняет эту структуру из state.State перед вызовом RenderConfig.
//
// Начиная с Phase D.4.2 canonical-данные хранятся непосредственно в каждом
// Outbound (поля Protocol/Transport/TLS/Proto), поэтому OutboundCanonicals
// параллельная карта упразднена.
type CoreRenderParams struct {
	Mode      Mode
	Outbounds []Outbound

	// RoutingConfig — пользовательский routing (inbounds/outbounds/rules/rule_sets).
	// Если nil — ядро использует только Outbounds (legacy путь, backward compat
	// для тестов и raw-CLI flows без routing.json).
	//
	// Каждое ядро транслирует RoutingConfig в свою натив-форму:
	//   - sing-box: route.rules[]/inbounds[]/outbounds[] (1:1 mapping)
	//   - xray:     routing.rules[] (RuleSet→geosite:/geoip: matchers, dokodemo-door inbound)
	//   - mihomo:   rules:/rule-providers:/proxies: (TYPE,VALUE,ACTION format)
	RoutingConfig *RoutingConfig

	// InboundMode — режим входящего соединения: "tun" или "tproxy".
	// Пустая строка — backward compat (sing-box: TUN; xray/mihomo: TProxy всегда).
	// Управляется флагом state.Inbound. Эффективен только для sing-box (xray/mihomo
	// игнорируют, у них inbound фиксированный).
	InboundMode string

	// DefaultOutboundTag — fallback тег для final outbound, если RoutingConfig.Final
	// пустой. Берётся из state.Outbounds[0].Tag в CLI слое.
	DefaultOutboundTag string

	// Preview — рендер для предпросмотра/валидации (Web UI /api/preview,
	// Routing Editor), а не для реального старта ядра. Отключает проверки
	// наличия локальных geo-ассетов (geosite.dat/geoip.dat у xray): preview
	// не должен падать в окружении без скачанных файлов, тогда как
	// production-путь (doStart) обязан вернуть понятную ошибку до запуска
	// ядра (инцидент 2026-05-12, tasks/lessons.md).
	Preview bool
}

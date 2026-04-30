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

// Outbound описывает исходящее соединение sing-box.
type Outbound struct {
	Tag      string         `json:"tag"`                // уникальный тег (обязателен)
	Type     string         `json:"type"`               // "socks", "vmess", "vless", "shadowsocks" и т.д.
	Server   string         `json:"server,omitempty"`   // не требуется для type=direct
	Port     Port           `json:"port,omitempty"`     // не требуется для type=direct
	Settings map[string]any `json:"settings,omitempty"` // тип-специфичные параметры
}

// MarshalJSON эмитит Outbound в формате sing-box: tag/type/server/server_port
// + Settings мерджатся как top-level поля (uuid, flow, tls, transport и т.п.).
// Это отличает sing-box от XRay-формата, где все спец-поля живут в "settings".
//
// Для служебных типов (direct/block/dns) поля server/server_port не эмитятся —
// sing-box ≥ 1.12 не принимает эти поля для direct (override_address/port были
// removed), block и dns как outbound-типы тоже удалены (заменены route actions).
func (o Outbound) MarshalJSON() ([]byte, error) {
	out := map[string]any{
		"tag":  o.Tag,
		"type": o.Type,
	}
	switch o.Type {
	case "direct", "block", "dns":
		// служебный outbound — без server/server_port
	default:
		if o.Server != "" {
			out["server"] = o.Server
		}
		if o.Port != 0 {
			out["server_port"] = uint16(o.Port)
		}
	}
	for k, v := range o.Settings {
		out[k] = v
	}
	return json.Marshal(out)
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

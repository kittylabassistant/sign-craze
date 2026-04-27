package types

import (
	"fmt"
	"net/netip"
	"runtime"
)

// Mode — режим маршрутизации трафика.
type Mode string

const (
	ModeProxy  Mode = "proxy"  // весь трафик через sing-box
	ModeDPI    Mode = "dpi"    // DPI-обход через nfqws2, без прокси
	ModeHybrid Mode = "hybrid" // sing-box + nfqws2 параллельно, выбор per-domain
)

// Validate проверяет допустимость значения Mode.
func (m Mode) Validate() error {
	switch m {
	case ModeProxy, ModeDPI, ModeHybrid:
		return nil
	default:
		return fmt.Errorf("неизвестный режим %q (допустимо: proxy, dpi, hybrid)", m)
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

// Validate проверяет обязательные поля Outbound.
func (o Outbound) Validate() error {
	if o.Tag == "" {
		return fmt.Errorf("outbound.Tag не может быть пустым")
	}
	if o.Type == "" {
		return fmt.Errorf("outbound.Type не может быть пустым")
	}
	if o.Server == "" {
		return fmt.Errorf("outbound.Server не может быть пустым")
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

package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"runtime"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// ValidCores — список валидных значений State.Core. Должен совпадать с
// ядрами, зарегистрированными в registry (см. internal/cli/cores.go init()).
// При добавлении нового ядра обновляйте здесь и в registry параллельно;
// тест internal/cli/cores_sync_test.go ловит расхождение через core.Names().
//
// Не используем core.Names() напрямую: state-пакет должен быть foundation-уровня
// без зависимостей на core (cli → state, cli → core; state → core нарушил бы
// слоистую иерархию).
var ValidCores = []string{"sing-box", "xray", "mihomo"}

// DefaultPolicyName — имя IP-policy в Keenetic RCI для режима ModePolicy.
const DefaultPolicyName = "sign-craze"

// DefaultRoutingUIPort — порт веб-интерфейса редактора routing по умолчанию.
const DefaultRoutingUIPort uint16 = 9092

// DefaultPath — стандартное расположение state.json.
const DefaultPath = "/opt/etc/sign-craze/state.json"

// DefaultCore — ядро по умолчанию для новых установок.
// Старые state.json без поля core мигрируются в это значение.
const DefaultCore = "sing-box"

// DefaultExcludes — безопасные CIDR-исключения, добавляемые в Default().
// Покрывают: loopback, link-local, multicast, зарезервированные, RFC1918.
// Защищают от потери LAN-доступа и SSH-lockout при первом запуске,
// если в signcraze_ipv4 окажется широкий префикс (см. safety-fixes.md #2).
var DefaultExcludes = []string{
	"127.0.0.0/8",    // loopback
	"169.254.0.0/16", // link-local
	"224.0.0.0/4",    // multicast
	"240.0.0.0/4",    // reserved
	"10.0.0.0/8",     // RFC1918
	"172.16.0.0/12",  // RFC1918
	"192.168.0.0/16", // RFC1918
}

// DefaultAdminPorts — порты SSH/админки роутера, исключаемые из проксирования
// по умолчанию: 22 (Entware/dropbear), 222 (Keenetic admin SSH).
var DefaultAdminPorts = []uint16{22, 222}

// State — персистентное состояние sign-craze.
type State struct {
	Mode      types.Mode       `json:"mode"`
	Outbounds []types.Outbound `json:"outbounds"`
	Ports     []uint16         `json:"ports"`
	Excludes  []string         `json:"excludes"`
	// AdminPort — legacy single-port поле. Сохранено для миграции старых state.json;
	// после Load() значение переносится в AdminPorts и обнуляется.
	//
	// Deprecated: использовать AdminPorts.
	AdminPort   uint16   `json:"admin_port,omitempty"`
	AdminPorts  []uint16 `json:"admin_ports,omitempty"`
	DPIEnabled  bool     `json:"dpi_enabled"`
	DPIStrategy string   `json:"dpi_strategy,omitempty"`
	// DPITargets — список доменов/паттернов для selective DPI desync.
	// Пусто = nfqws2 пытается desync для всего трафика (текущее поведение).
	// Заполнено = nfqws2 запускается с --hostlist, desync только для match.
	DPITargets []string `json:"dpi_targets,omitempty"`
	// DPIExcludeIPs — список IP-адресов, для которых nfqws2-desync не применяется.
	// Используется для VPN-эндпоинтов (sign-box → Reality-сервер, downstream-VPN
	// клиентов): десинхронизация TLS ClientHello к VPN-IP ломает Reality-маскировку
	// либо бесполезна. RETURN-правила вставляются в начало signcraze_policy_dpi
	// перед NFQUEUE-портами.
	DPIExcludeIPs []string `json:"dpi_exclude_ips,omitempty"`
	// DPIUpdateURLs — URL-источники с hostlist-доменами для auto-update.
	// Каждая строка файла = один домен (комментарии # и пустые строки игнорируются).
	// Пусто = auto-update выключен. Скачанные домены merge с пресетами и пишутся
	// в /opt/etc/sign-craze/hostlist.txt при каждом цикле обновления.
	// Рекомендуемые источники:
	//   github.com/bol-van/zapret (ipset/zapret-hosts-user.txt.example)
	//   github.com/flowseal/zapret-discord-youtube (lists/list-*.txt)
	DPIUpdateURLs []string `json:"dpi_update_urls,omitempty"`
	// DPIUpdateIntervalHours — период auto-update в часах. 0 = выключено,
	// рекомендуется 24h. Меньше 1h игнорируется (защита от DDoS upstream).
	DPIUpdateIntervalHours int `json:"dpi_update_interval_hours,omitempty"`
	// DPILastUpdate — RFC3339-таймштамп последнего успешного auto-update.
	// Watchdog запускает следующее обновление, когда now() > DPILastUpdate + interval.
	DPILastUpdate  string `json:"dpi_last_update,omitempty"`
	BootTimeoutSec int    `json:"boot_timeout_sec,omitempty"` // таймаут waitDefaultRoute, 0 = default 60

	// Поля режима ModePolicy: интеграция с Keenetic IP Policy через RCI.
	// PolicyMark и PolicyTable — кеш runtime-значений, актуальные читаются
	// при каждом --start через ndm.GetPolicy().
	PolicyName   string `json:"policy_name,omitempty"`   // имя policy в RCI, default "sign-craze"
	PolicyMark   uint32 `json:"policy_mark,omitempty"`   // присвоенный Keenetic'ом fwmark (cache)
	PolicyTable  int    `json:"policy_table,omitempty"`  // routing table policy в Keenetic (cache, IPv4)
	WANInterface string `json:"wan_interface,omitempty"` // Keenetic-имя WAN, e.g. GigabitEthernet1

	// Поля UI-редактора routing-конфигурации.
	RoutingUIEnabled bool   `json:"routing_ui_enabled,omitempty"` // включён ли веб-редактор routing
	RoutingUIPort    uint16 `json:"routing_ui_port,omitempty"`    // порт веб-редактора routing
	RoutingUIBind    string `json:"routing_ui_bind,omitempty"`    // bind-адрес веб-редактора routing

	// Core — выбранное прокси-ядро. Допустимо: "sing-box", "xray", "mihomo".
	// Пустое значение (старые state.json) мигрируется в DefaultCore при Load().
	Core string `json:"core,omitempty"`

	// Inbound — режим входящего соединения для sing-box. Допустимо: "tun", "tproxy".
	// Пустое значение (старые state.json) мигрируется в "tproxy" при Load().
	// Влияет на генерацию конфига sing-box и firewall-правила (TPROXY vs TUN).
	Inbound string `json:"inbound,omitempty"`

	// NaiveEnabled — включена ли поддержка naiveproxy daemon. При true и
	// наличии outbound'ов с Protocol=ProtocolNaive, --start запускает naive-демон
	// на 127.0.0.1:NaiveListenPort для каждого. См. ADR-0021-naiveproxy-process-chain.
	NaiveEnabled bool `json:"naive_enabled,omitempty"`
}

// DefaultInbound — режим inbound для новых установок.
const DefaultInbound = "tproxy"

// Default возвращает state с настройками по умолчанию: режим policy, stub direct outbound,
// безопасные локальные исключения и admin port 22.
func Default() *State {
	excludes := make([]string, len(DefaultExcludes))
	copy(excludes, DefaultExcludes)
	return &State{
		Mode: types.ModePolicy,
		Outbounds: []types.Outbound{
			{Tag: "direct", Type: "direct"},
		},
		Ports:            []uint16{},
		Excludes:         excludes,
		AdminPorts:       append([]uint16(nil), DefaultAdminPorts...),
		PolicyName:       DefaultPolicyName,
		RoutingUIEnabled: true,
		RoutingUIPort:    DefaultRoutingUIPort,
		RoutingUIBind:    "0.0.0.0",
		Core:             DefaultCore,
		Inbound:          DefaultInbound,
	}
}

// stateLegacy используется при десериализации state.json для захвата
// устаревшего поля outbound_canonicals (Phase D.0a). После миграции поле
// переносится в Outbound.Protocol/Transport/TLS/Proto и не сохраняется.
type stateLegacy struct {
	State
	OutboundCanonicals map[string]types.Canonical `json:"outbound_canonicals,omitempty"`
}

// Load читает state.json. Если файл отсутствует — возвращает Default().
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return nil, fmt.Errorf("state: чтение %s: %w", path, err)
	}

	var legacy stateLegacy
	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("state: парсинг %s: %w", path, err)
	}
	s := &legacy.State
	migrateOutboundCanonicals(s, legacy.OutboundCanonicals)
	migrateLegacyMode(s)
	migrateAdminPort(s)
	migrateRoutingUI(s)
	migrateInbound(s)
	if s.Outbounds == nil {
		s.Outbounds = []types.Outbound{}
	}
	if s.Ports == nil {
		s.Ports = []uint16{}
	}
	if s.Excludes == nil {
		s.Excludes = []string{}
	}
	if s.AdminPorts == nil {
		s.AdminPorts = []uint16{}
	}
	if s.DPITargets == nil {
		s.DPITargets = []string{}
	}
	if s.DPIExcludeIPs == nil {
		s.DPIExcludeIPs = []string{}
	}
	if s.DPIUpdateURLs == nil {
		s.DPIUpdateURLs = []string{}
	}
	if s.PolicyName == "" {
		s.PolicyName = DefaultPolicyName
	}
	if s.Core == "" {
		s.Core = DefaultCore
	}
	return s, nil
}

// migrateInbound устанавливает Inbound = DefaultInbound для старых state.json,
// где поле отсутствует (пустое значение).
func migrateInbound(s *State) {
	if s.Inbound == "" {
		s.Inbound = DefaultInbound
	}
}

// migrateOutboundCanonicals переносит данные из устаревшего поля
// OutboundCanonicals в inline-поля каждого Outbound.
// Вызывается однократно при Load() для state.json, созданных до Phase D.4.
func migrateOutboundCanonicals(s *State, canonicals map[string]types.Canonical) {
	if len(canonicals) == 0 {
		return
	}
	for i := range s.Outbounds {
		o := &s.Outbounds[i]
		if o.Protocol != "" {
			continue // canonical уже внутри Outbound
		}
		c, ok := canonicals[o.Tag]
		if !ok || !c.IsSet() {
			continue
		}
		o.Protocol = c.Protocol
		o.Transport = c.Transport
		o.TLS = c.TLS
		o.Proto = c.Proto
		log.L().Info("state: мигрирован canonical из outbound_canonicals",
			"tag", o.Tag,
			"protocol", string(o.Protocol),
		)
	}
}

// migrateLegacyMode конвертирует legacy-режимы (proxy/dpi/hybrid) в ModePolicy
// и пишет WARN в лог. Подсказка про --mode full даётся, чтобы пользователь
// мог вернуть старое поведение, если сознательно его выбирал.
func migrateLegacyMode(s *State) {
	if newMode, ok := types.LegacyModes[s.Mode]; ok {
		log.L().Warn("state: режим устарел, переключено в policy",
			"old_mode", string(s.Mode),
			"new_mode", string(newMode),
			"hint", "для возврата старого поведения: sign-craze --mode full --restart",
		)
		s.Mode = newMode
	}
}

// migrateRoutingUI заполняет RoutingUI-поля для старых state.json, где их не было.
func migrateRoutingUI(s *State) {
	if s.RoutingUIPort == 0 {
		s.RoutingUIPort = DefaultRoutingUIPort
		s.RoutingUIEnabled = true
	}
	if s.RoutingUIBind == "" {
		s.RoutingUIBind = "0.0.0.0"
	}
}

// migrateAdminPort переносит legacy-поле AdminPort в AdminPorts.
// Если AdminPorts пуст и AdminPort задан — копируем; AdminPort обнуляется
// чтобы при следующем Save() в JSON попало только admin_ports.
func migrateAdminPort(s *State) {
	if len(s.AdminPorts) == 0 && s.AdminPort != 0 {
		s.AdminPorts = []uint16{s.AdminPort}
	}
	s.AdminPort = 0
}

// GetOutbounds возвращает список outbound'ов из state.
// Реализует интерфейс routing.BootstrapState.
func (s *State) GetOutbounds() []types.Outbound {
	return s.Outbounds
}

// Validate проверяет корректность критичных полей State.
// Нулевые значения для optional-полей (BootTimeoutSec=0) допустимы.
// AdminPorts пустой = bypass выключен.
func (s *State) Validate() error {
	if s == nil {
		return fmt.Errorf("state: nil State")
	}
	if s.Mode != "" {
		if err := s.Mode.Validate(); err != nil {
			return err
		}
	}
	for _, p := range s.Ports {
		if p == 0 {
			return fmt.Errorf("state: ports содержит 0 (некорректный порт)")
		}
	}
	for _, p := range s.AdminPorts {
		if p == 0 {
			return fmt.Errorf("state: admin_ports содержит 0 (некорректный порт)")
		}
	}
	if s.Core != "" {
		valid := false
		for _, name := range ValidCores {
			if s.Core == name {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("state: неизвестное ядро %q (допустимо: %s)",
				s.Core, strings.Join(ValidCores, ", "))
		}
	}
	if s.Inbound != "" {
		switch s.Inbound {
		case "tun", "tproxy":
		default:
			return fmt.Errorf("state: неизвестный inbound %q (допустимо: tun, tproxy)", s.Inbound)
		}
	}
	for _, ip := range s.DPIExcludeIPs {
		if _, err := netip.ParseAddr(ip); err != nil {
			return fmt.Errorf("state: dpi_exclude_ips содержит некорректный IP %q: %w", ip, err)
		}
	}
	if s.DPIUpdateIntervalHours < 0 {
		return fmt.Errorf("state: dpi_update_interval_hours не может быть отрицательным")
	}
	// mieru supervised peer: MieruLocalPort должен быть в валидном диапазоне,
	// если задан. Ноль допустим (ещё не выделен через peer.AllocateMieruPort).
	for _, ob := range s.Outbounds {
		if ob.Proto == nil || ob.Proto.MieruLocalPort == 0 {
			continue
		}
		if ob.Proto.MieruLocalPort < 1024 || ob.Proto.MieruLocalPort > 65535 {
			return fmt.Errorf("state: outbound %q: mieru_local_port=%d вне диапазона 1024..65535", ob.Tag, ob.Proto.MieruLocalPort)
		}
	}
	// naive supervised peer (ADR-0021-naiveproxy-process-chain): MVP допускает
	// ровно один naive outbound, big-endian MIPS не поддерживается upstream
	// klzgrad/naiveproxy.
	naiveCount := 0
	for _, ob := range s.Outbounds {
		if ob.Protocol == types.ProtocolNaive {
			naiveCount++
			if ob.Proto != nil && ob.Proto.NaiveListenPort != 0 {
				if ob.Proto.NaiveListenPort < 1024 || ob.Proto.NaiveListenPort > 65535 {
					return fmt.Errorf("state: outbound %q: naive_listen_port=%d вне диапазона 1024..65535", ob.Tag, ob.Proto.NaiveListenPort)
				}
			}
		}
	}
	if naiveCount > 1 {
		return fmt.Errorf("state: MVP допускает не более одного naive outbound (найдено %d)", naiveCount)
	}
	if naiveCount > 0 && runtime.GOARCH == "mips" {
		return fmt.Errorf("state: naive outbound не поддерживается на big-endian MIPS (klzgrad/naiveproxy публикует только linux-mipsel)")
	}
	return nil
}

// Save атомарно записывает state в файл с правами 0o600 (содержит outbound credentials).
// Перед записью валидирует State — некорректное состояние не должно попадать на диск.
func Save(path string, s *State) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal: %w", err)
	}
	if err := atomicfs.WriteFileAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("state: запись %s: %w", path, err)
	}
	return nil
}

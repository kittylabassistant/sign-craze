package web

import "context"

// StatusInfo — ответ GET /api/status.
type StatusInfo struct {
	SingBox ServiceState `json:"singbox"`
	Nfqws2  ServiceState `json:"nfqws2"`
	Mode    string       `json:"mode"`
	Version VersionInfo  `json:"version"`
	UptimeS int64        `json:"uptime_s"`
}

// ServiceState описывает состояние одного сервиса.
type ServiceState struct {
	Running bool `json:"running"`
	PID     int  `json:"pid"`
}

// VersionInfo содержит версии компонентов.
type VersionInfo struct {
	SignCraze string `json:"sign_craze"`
	SingBox   string `json:"sing_box"`
}

// StatusReader возвращает текущее состояние сервисов.
type StatusReader interface {
	Status(ctx context.Context) (StatusInfo, error)
}

// ConfigRW читает и обновляет config.json sing-box.
type ConfigRW interface {
	ReadConfig(ctx context.Context) ([]byte, error)
	WriteConfig(ctx context.Context, data []byte) error
}

// PortsManager управляет списком проксируемых портов.
type PortsManager interface {
	ListPorts(ctx context.Context) ([]int, error)
	AddPort(ctx context.Context, port int) error
	DeletePort(ctx context.Context, port int) error
}

// ExcludesManager управляет IP/CIDR-исключениями.
type ExcludesManager interface {
	ListExcludes(ctx context.Context) ([]string, error)
	AddExclude(ctx context.Context, cidr string) error
	DeleteExclude(ctx context.Context, cidr string) error
}

// DPITargetsManager управляет списком доменов для selective DPI desync (nfqws2 --hostlist).
// Пустой список = nfqws2 пытается desync для всего трафика (текущее поведение).
// Заполненный список = desync только для соединений с matching SNI/hostname.
type DPITargetsManager interface {
	ListTargets(ctx context.Context) ([]string, error)
	SetTargets(ctx context.Context, targets []string) error
}

// DPIPreset — именованный набор готовых доменов (Discord, YouTube и т.п.).
type DPIPreset struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Targets     []string `json:"targets"`
}

// BuiltinDPIPresets — встроенные пресеты DPI targets.
// Источники доменов: официальные docs, community-наблюдения. Список консервативный
// и охватывает Discord (signaling+voice+CDN) и YouTube (страница+видео+QUIC).
var BuiltinDPIPresets = []DPIPreset{
	{
		Name:        "discord",
		Description: "Discord: чат, голос, видео, CDN.",
		Targets: []string{
			"discord.com",
			"discordapp.com",
			"discord.gg",
			"discordapp.net",
			"discord.media",
			"discord.gift",
			"cdn.discordapp.com",
			"media.discordapp.net",
		},
	},
	{
		Name:        "youtube",
		Description: "YouTube: страница, видео, QUIC, мобильное API.",
		Targets: []string{
			"youtube.com",
			"www.youtube.com",
			"m.youtube.com",
			"youtu.be",
			"youtubei.googleapis.com",
			"yt3.ggpht.com",
			"i.ytimg.com",
			"googlevideo.com",
			"ytimg.com",
		},
	},
	{
		Name:        "discord-youtube",
		Description: "Объединённый пресет: Discord + YouTube.",
		Targets: []string{
			// discord
			"discord.com", "discordapp.com", "discord.gg", "discordapp.net",
			"discord.media", "discord.gift", "cdn.discordapp.com", "media.discordapp.net",
			// youtube
			"youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be",
			"youtubei.googleapis.com", "yt3.ggpht.com", "i.ytimg.com",
			"googlevideo.com", "ytimg.com",
		},
	},
}

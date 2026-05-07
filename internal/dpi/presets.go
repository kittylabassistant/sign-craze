package dpi

// Preset — именованный набор доменов для selective DPI desync.
// Передаётся в nfqws2 через --hostlist=<file>, где каждый домен на отдельной строке.
type Preset struct {
	Name        string
	Description string
	Targets     []string
}

// Builtin — встроенные пресеты DPI targets.
// Источники доменов: официальные docs, community-наблюдения. Список консервативный
// и охватывает Discord (signaling+voice+CDN) и YouTube (страница+видео+QUIC).
//
// Используется CLI (--install --with-dpi) и Web UI (POST /api/dpi/presets/.../apply).
var Builtin = []Preset{
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
			"discord.com", "discordapp.com", "discord.gg", "discordapp.net",
			"discord.media", "discord.gift", "cdn.discordapp.com", "media.discordapp.net",
			"youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be",
			"youtubei.googleapis.com", "yt3.ggpht.com", "i.ytimg.com",
			"googlevideo.com", "ytimg.com",
		},
	},
}

// FindPreset возвращает Preset по имени. Возвращает nil если не найден.
func FindPreset(name string) *Preset {
	for i := range Builtin {
		if Builtin[i].Name == name {
			return &Builtin[i]
		}
	}
	return nil
}

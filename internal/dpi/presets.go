package dpi

import "slices"

// Preset — именованный набор доменов для selective DPI desync.
// Передаётся в nfqws2 через --hostlist=<file>, где каждый домен на отдельной строке.
type Preset struct {
	Name        string
	Description string
	Targets     []string
}

// discordTargets — домены Discord: чат, голос, видео, CDN.
var discordTargets = []string{
	"discord.com",
	"discordapp.com",
	"discord.gg",
	"discordapp.net",
	"discord.media",
	"discord.gift",
	"cdn.discordapp.com",
	"media.discordapp.net",
}

// youtubeTargets — домены YouTube: страница, видео, QUIC, мобильное API.
var youtubeTargets = []string{
	"youtube.com",
	"www.youtube.com",
	"m.youtube.com",
	"youtu.be",
	"youtubei.googleapis.com",
	"yt3.ggpht.com",
	"i.ytimg.com",
	"googlevideo.com",
	"ytimg.com",
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
		Targets:     discordTargets,
	},
	{
		Name:        "youtube",
		Description: "YouTube: страница, видео, QUIC, мобильное API.",
		Targets:     youtubeTargets,
	},
	{
		Name:        "discord-youtube",
		Description: "Объединённый пресет: Discord + YouTube.",
		// Конкатенация вместо повторного перечисления доменов — порядок сохранён
		// (сначала discord, затем youtube), дрейф между списками ловит тест
		// TestBuiltin_DiscordYoutubeРавенОбъединениюDiscordИYoutube.
		Targets: slices.Concat(discordTargets, youtubeTargets),
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

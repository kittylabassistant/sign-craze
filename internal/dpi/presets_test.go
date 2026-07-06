package dpi

import (
	"slices"
	"testing"
)

// TestBuiltin_DiscordYoutubeРавенОбъединениюDiscordИYoutube проверяет, что пресет
// "discord-youtube" — это объединение доменов пресетов "discord" и "youtube" в
// текущем порядке (сначала discord, затем youtube). Ловит будущий дрейф: если один
// из базовых пресетов изменят, а "discord-youtube" забудут обновить синхронно —
// тест упадёт.
func TestBuiltin_DiscordYoutubeРавенОбъединениюDiscordИYoutube(t *testing.T) {
	discord := FindPreset("discord")
	youtube := FindPreset("youtube")
	discordYoutube := FindPreset("discord-youtube")

	if discord == nil || youtube == nil || discordYoutube == nil {
		t.Fatal("ожидались встроенные пресеты discord, youtube и discord-youtube")
	}

	want := slices.Concat(discord.Targets, youtube.Targets)
	if !slices.Equal(discordYoutube.Targets, want) {
		t.Errorf("discord-youtube.Targets = %v, want (discord ∪ youtube в текущем порядке) = %v",
			discordYoutube.Targets, want)
	}
}

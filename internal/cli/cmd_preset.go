package cli

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/preset"
)

func init() {
	Register(Cmd{Long: "--preset-list", Help: "показать встроенные routing-пресеты", Handler: handlePresetList})
}

func handlePresetList(_ context.Context, _ []string) error {
	fmt.Println(Header("Встроенные routing-пресеты:"))
	fmt.Println()
	for _, p := range preset.List() {
		fmt.Printf("  %s\n", Bold(p.Name))
		fmt.Printf("    %s\n", p.Description)
		if p.Source != "" {
			fmt.Printf("    %s %s\n", Hint("источник:"), p.Source)
		}
		if preset.UsesVPN(&p) {
			fmt.Printf("    %s требует VPN-outbound (--proxy <URL>)\n", Hint("⚠"))
		}
		fmt.Println()
	}
	fmt.Println(Hint("Применить: sign-craze --install --proxy <URL> --preset <name>"))
	return nil
}

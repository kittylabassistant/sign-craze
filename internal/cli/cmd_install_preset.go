package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/preset"
	"github.com/kittylabassistant/sign-craze/internal/routing"
	"github.com/kittylabassistant/sign-craze/internal/state"
)

// applyInstallPreset применяет встроенный routing-preset к routing.json и
// перегенерирует конфиг активного ядра. Вызывается на шаге 7.7 doInstall,
// когда юзер передал --preset <name>.
//
// Алгоритм:
//  1. Resolve preset через preset.Find — fail-fast если имя неизвестно.
//  2. Если preset.UsesVPN(p) && state.Outbounds[0] direct/stub — fail-fast
//     (preset с {vpn}-placeholder требует реальный VLESS/Trojan/SS outbound).
//  3. Bootstrap routing.json через routing.BootstrapFromState (no-op если уже есть).
//  4. Load routing.json через routing.Load — после bootstrap всегда non-nil.
//  5. Apply preset (ModeReplace): warnings ← preset.ApplyToConfig(cfg, p, vpnTag, geoFormat, ModeReplace).
//  6. Print warnings (если есть).
//  7. Save routing.json через routing.Save.
//  8. Re-render config движка через regenerateConfig — обеспечивает что
//     первый --start стартует с правильным routing'ом.
func applyInstallPreset(ctx context.Context, name string, c core.Core, st *state.State) error {
	p := preset.Find(name)
	if p == nil {
		available := make([]string, 0, len(preset.List()))
		for _, pp := range preset.List() {
			available = append(available, pp.Name)
		}
		return fmt.Errorf("неизвестный preset %q. Доступные: %s (см. --preset-list)",
			name, strings.Join(available, ", "))
	}

	// Определить VPN tag для подстановки.
	vpnTag := ""
	if len(st.Outbounds) > 0 && st.Outbounds[0].Type != "direct" {
		vpnTag = st.Outbounds[0].Tag
	}

	if preset.UsesVPN(p) && vpnTag == "" {
		return fmt.Errorf("preset %q использует placeholder {vpn}: требуется VPN-outbound (--proxy <URL>)", name)
	}

	// Bootstrap routing.json если ещё не существует.
	if err := routing.BootstrapFromState(routing.DefaultPath, st); err != nil {
		return fmt.Errorf("bootstrap routing.json: %w", err)
	}

	cfg, err := routing.Load(routing.DefaultPath)
	if err != nil {
		return fmt.Errorf("load routing.json: %w", err)
	}
	if cfg == nil {
		// Не должно случиться после BootstrapFromState, но защищаемся.
		return fmt.Errorf("routing.json не создан после bootstrap")
	}

	warnings := preset.ApplyToConfig(cfg, p, vpnTag, c.GeoFormat(), preset.ModeReplace)
	for _, w := range warnings {
		fmt.Printf("%s %s\n", Warn("preset:"), w)
	}

	if err := routing.Save(routing.DefaultPath, cfg); err != nil {
		return fmt.Errorf("save routing.json: %w", err)
	}

	// Re-render конфига движка чтобы --start подхватил новый routing.
	if err := regenerateConfig(ctx, st); err != nil {
		return fmt.Errorf("regenerate config: %w", err)
	}

	fmt.Printf("%s preset %q применён к routing.json\n", OK("✓"), name)
	return nil
}

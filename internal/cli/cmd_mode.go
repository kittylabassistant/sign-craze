package cli

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func init() {
	Register(Cmd{Long: "--mode", Help: "переключить режим: policy (default) | full (legacy)", Handler: handleMode})
}

func handleMode(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--mode: требуется аргумент policy|full")
	}
	m := types.Mode(args[0])
	// Принимаем legacy-имена и сразу мигрируем в современные.
	if mapped, ok := types.LegacyModes[m]; ok {
		fmt.Printf("режим %q устарел, переключено в %q (для возврата старого поведения: sign-craze --mode full)\n", m, mapped)
		m = mapped
	}
	if err := m.Validate(); err != nil {
		return fmt.Errorf("--mode: %w", err)
	}

	var st *state.State
	if err := withStateMutation(ctx, func(s *state.State) error {
		s.Mode = m
		st = s
		return nil
	}); err != nil {
		return err
	}
	if err := regenerateConfig(ctx, st); err != nil {
		return fmt.Errorf("--mode: regenerate config: %w", err)
	}
	fmt.Printf("Режим установлен в %q. Применить: sign-craze --restart\n", m)
	return nil
}

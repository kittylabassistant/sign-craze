package cli

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func init() {
	Register(Cmd{Long: "--mode", Help: "переключить режим: proxy|dpi|hybrid", Handler: handleMode})
}

func handleMode(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--mode: требуется аргумент proxy|dpi|hybrid")
	}
	m := types.Mode(args[0])
	if err := m.Validate(); err != nil {
		return fmt.Errorf("--mode: %w", err)
	}

	return withLock(ctx, func() error {
		st, err := loadState()
		if err != nil {
			return err
		}
		st.Mode = m
		if err := saveState(st); err != nil {
			return err
		}
		if err := regenerateConfig(ctx, st); err != nil {
			return fmt.Errorf("--mode: regenerate config: %w", err)
		}
		fmt.Printf("Режим установлен в %q. Применить: sign-craze --restart\n", m)
		return nil
	})
}

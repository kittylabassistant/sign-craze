package cli

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/state"
)

func init() {
	Register(Cmd{Long: "--exclude-add", Help: "добавить IP/CIDR в исключения", Handler: handleExcludeAdd})
	Register(Cmd{Long: "--exclude-del", Help: "удалить IP/CIDR из исключений", Handler: handleExcludeDel})
	Register(Cmd{Long: "--exclude-list", Help: "перечислить исключения", Handler: handleExcludeList})
}

func handleExcludeAdd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--exclude-add: требуется CIDR или IP")
	}
	return withLock(ctx, func() error {
		mgr := state.NewExcludesManager(state.DefaultPath)
		if err := mgr.AddExclude(ctx, args[0]); err != nil {
			return fmt.Errorf("--exclude-add: %w", err)
		}
		fmt.Printf("Исключение %q добавлено. Применить: sign-craze --restart\n", args[0])
		return nil
	})
}

func handleExcludeDel(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--exclude-del: требуется CIDR или IP")
	}
	return withLock(ctx, func() error {
		mgr := state.NewExcludesManager(state.DefaultPath)
		if err := mgr.DeleteExclude(ctx, args[0]); err != nil {
			return fmt.Errorf("--exclude-del: %w", err)
		}
		fmt.Println("Исключение удалено. Применить: sign-craze --restart")
		return nil
	})
}

func handleExcludeList(ctx context.Context, _ []string) error {
	mgr := state.NewExcludesManager(state.DefaultPath)
	list, err := mgr.ListExcludes(ctx)
	if err != nil {
		return fmt.Errorf("--exclude-list: %w", err)
	}
	if len(list) == 0 {
		fmt.Println("(пусто)")
		return nil
	}
	for _, e := range list {
		fmt.Println(e)
	}
	return nil
}

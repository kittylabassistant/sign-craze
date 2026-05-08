package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/backup"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

func init() {
	Register(Cmd{Short: "-b", Long: "--backup", Help: "создать архив /opt/etc/sign-craze/", Handler: handleBackup})
	Register(Cmd{Long: "--restore", Help: "восстановить архив <путь>", Handler: handleRestore})
}

func handleBackup(_ context.Context, _ []string) error {
	dst := filepath.Join(backup.DefaultDir, backup.TimestampedName("backup"))
	if err := backup.Create(mustActiveCore().ConfigDir(), dst); err != nil {
		return fmt.Errorf("--backup: %w", err)
	}
	fmt.Printf("Архив создан: %s\n", dst)
	return nil
}

func handleRestore(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--restore: требуется путь к архиву")
	}
	return withLock(ctx, func() error {
		if err := doStop(ctx); err != nil {
			log.L().Warn("--restore: ошибка stop, продолжаем", "err", err)
		}
		if err := backup.Restore(args[0], mustActiveCore().ConfigDir()); err != nil {
			return fmt.Errorf("--restore: %w", err)
		}
		fmt.Println("Восстановление завершено. Сервис не запущен — запустите --start вручную.")
		return nil
	})
}

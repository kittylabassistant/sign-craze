package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/backup"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
)

func init() {
	Register(Cmd{Short: "-b", Long: "--backup", Help: "создать архив /opt/etc/sign-craze/", Handler: handleBackup})
	Register(Cmd{Long: "--restore", Help: "восстановить архив <путь>", Handler: handleRestore})
	Register(Cmd{Long: "--config-backup", Help: "архив только config.json", Handler: handleConfigBackup})
	Register(Cmd{Long: "--config-restore", Help: "восстановить config.json из архива <путь>", Handler: handleConfigRestore})
}

func handleBackup(_ context.Context, _ []string) error {
	dst := filepath.Join(backup.DefaultDir, backup.TimestampedName("backup"))
	if err := backup.Create(singbox.DefaultConfigDir, dst); err != nil {
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
		if err := backup.Restore(args[0], singbox.DefaultConfigDir); err != nil {
			return fmt.Errorf("--restore: %w", err)
		}
		fmt.Println("Восстановление завершено. Сервис не запущен — запустите --start вручную.")
		return nil
	})
}

func handleConfigBackup(_ context.Context, _ []string) error {
	dst := filepath.Join(backup.DefaultDir, backup.TimestampedName("config"))
	if err := backup.Create(configPath(), dst); err != nil {
		return fmt.Errorf("--config-backup: %w", err)
	}
	fmt.Printf("Архив config.json создан: %s\n", dst)
	return nil
}

func handleConfigRestore(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--config-restore: требуется путь к архиву")
	}
	return withLock(ctx, func() error {
		if err := backup.Restore(args[0], singbox.DefaultConfigDir); err != nil {
			return fmt.Errorf("--config-restore: %w", err)
		}
		fmt.Println("config.json восстановлен. Перезапустите сервис: sign-craze --restart")
		return nil
	})
}

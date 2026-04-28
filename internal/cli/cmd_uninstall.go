package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
)

func init() {
	Register(Cmd{Long: "--uninstall", Help: "удалить sing-box и конфигурацию", Handler: handleUninstall})
	Register(Cmd{Long: "--purge", Help: "удалить всё включая sign-craze, логи, geo", Handler: handlePurge})
}

func handleUninstall(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		if err := doStop(ctx); err != nil {
			log.L().Warn("--uninstall: ошибка stop, продолжаем", "err", err)
		}
		paths := []string{
			service.DefaultShimPath,
			singbox.DefaultBinPath,
		}
		for _, p := range paths {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				log.L().Warn("--uninstall: ошибка удаления", "path", p, "err", err)
			}
		}
		dirs := []string{
			singbox.DefaultConfigDir,
			"/opt/var/lib/sign-craze",
			"/opt/var/run", // PID-файлы — но не sign-craze.lock в /opt/var/lock
		}
		for _, d := range dirs {
			if err := os.RemoveAll(d); err != nil {
				log.L().Warn("--uninstall: ошибка удаления", "dir", d, "err", err)
			}
		}
		fmt.Println("sing-box и конфиги удалены. Бинарь sign-craze и логи сохранены.")
		return nil
	})
}

func handlePurge(ctx context.Context, _ []string) error {
	if err := handleUninstall(ctx, nil); err != nil {
		log.L().Warn("--purge: ошибка uninstall, продолжаем", "err", err)
	}
	if err := os.Remove(service.DefaultSignCrazeBin); err != nil && !os.IsNotExist(err) {
		log.L().Warn("--purge: удаление sign-craze бинаря", "err", err)
	}
	if err := os.RemoveAll("/opt/var/log/sign-craze"); err != nil {
		log.L().Warn("--purge: удаление логов", "err", err)
	}
	fmt.Println("Полная очистка завершена.")
	return nil
}

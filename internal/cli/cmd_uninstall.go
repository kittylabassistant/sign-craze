package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/ndm"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/pkg/types"
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

		// В режиме ModePolicy: удалить policy из Keenetic RCI до того, как
		// очистим state. Используем policy_name из state, fallback на default.
		if st, err := loadState(); err == nil && st.Mode == types.ModePolicy && st.PolicyName != "" {
			client := ndm.NewClient()
			if err := client.DeletePolicy(ctx, st.PolicyName); err != nil {
				log.L().Warn("--uninstall: ndm.DeletePolicy", "name", st.PolicyName, "err", err)
			} else if err := client.SaveConfig(ctx); err != nil {
				log.L().Warn("--uninstall: ndm.SaveConfig", "err", err)
			}
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

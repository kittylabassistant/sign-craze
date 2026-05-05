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
		removed := doUninstall(ctx, "--uninstall")
		printRemoved("Удалено", removed)
		fmt.Println("Бинарь sign-craze и логи сохранены — для полной очистки используйте --purge.")
		return nil
	})
}

func handlePurge(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		removed := doUninstall(ctx, "--purge")

		// purge-extras: бинарь sign-craze + логи. /opt/var/lib/sign-craze
		// уже снесён внутри doUninstall (включая cache/, geo/, backups/).
		extras := map[string]string{
			service.DefaultSignCrazeBin: "бинарь sign-craze",
			"/opt/var/log/sign-craze":   "логи",
		}
		for path, label := range extras {
			if removeIfExists(path) {
				removed = append(removed, label+" ("+path+")")
			} else if _, err := os.Stat(path); err == nil {
				log.L().Warn("--purge: удаление не удалось", "path", path)
			}
		}

		printRemoved("Полная очистка завершена. Удалено", removed)
		return nil
	})
}

// doUninstall выполняет stop + удаление sing-box, конфигов, runtime-каталогов.
// Возвращает список удалённых сущностей с человеко-читаемыми описаниями.
// label используется только в WARN-логах для идентификации источника.
func doUninstall(ctx context.Context, label string) []string {
	var removed []string

	if err := doStop(ctx); err != nil {
		log.L().Warn(label+": ошибка stop, продолжаем", "err", err)
	}

	// ModePolicy: убрать policy из Keenetic RCI до удаления state.
	if st, err := loadState(); err == nil && st.Mode == types.ModePolicy && st.PolicyName != "" {
		client := ndm.NewClient()
		if err := client.DeletePolicy(ctx, st.PolicyName); err != nil {
			log.L().Warn(label+": ndm.DeletePolicy", "name", st.PolicyName, "err", err)
		} else {
			if err := client.SaveConfig(ctx); err != nil {
				log.L().Warn(label+": ndm.SaveConfig", "err", err)
			}
			removed = append(removed, "Keenetic policy "+st.PolicyName)
		}
	}

	files := map[string]string{
		service.DefaultShimPath:               "init.d shim",
		service.DefaultNetfilterHookPath:      "NDM netfilter hook",
		singbox.DefaultBinPath:                "бинарь sing-box",
		"/opt/var/run/sign-craze-singbox.pid": "PID-файл sing-box",
		"/opt/var/run/sign-craze-nfqws2.pid":  "PID-файл nfqws2",
		"/opt/var/run/sign-craze-ui.pid":      "PID-файл Web UI",
	}
	for path, lbl := range files {
		if removeIfExists(path) {
			removed = append(removed, lbl+" ("+path+")")
		}
	}

	dirs := map[string]string{
		singbox.DefaultConfigDir:  "конфиги sing-box",
		"/opt/var/lib/sign-craze": "state, cache, geo, backups",
	}
	for path, lbl := range dirs {
		if removeAllIfExists(path) {
			removed = append(removed, lbl+" ("+path+")")
		}
	}

	return removed
}

// removeIfExists удаляет файл если есть; возвращает true если удалили.
func removeIfExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return os.Remove(path) == nil
}

// removeAllIfExists удаляет каталог рекурсивно если есть; true если удалили.
func removeAllIfExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return os.RemoveAll(path) == nil
}

// printRemoved печатает шапку + bullet-list удалённых сущностей. Если ничего
// не удалено — пишет "ничего не найдено", чтобы пользователь не подумал что
// команда отработала с ошибкой.
func printRemoved(header string, removed []string) {
	if len(removed) == 0 {
		fmt.Println(header + ": ничего не найдено (sign-craze уже не установлен).")
		return
	}
	fmt.Println(header + ":")
	for _, r := range removed {
		fmt.Println("  - " + r)
	}
}

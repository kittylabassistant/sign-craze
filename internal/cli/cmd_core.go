package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func init() {
	Register(Cmd{
		Long:    "--core-list",
		Help:    "список зарегистрированных ядер и активное",
		Handler: handleCoreList,
	})
	Register(Cmd{
		Long:    "--core",
		Help:    "показать или сменить активное ядро: <имя>",
		Handler: handleCore,
	})
	Register(Cmd{
		Long:    "--core-install",
		Help:    "установить дополнительное ядро без активации: <имя>",
		Handler: handleCoreInstall,
	})
}

// handleCore — `--core [<name>]`: без аргумента показывает активное ядро,
// с аргументом меняет state.Core.
// Runtime hot-swap (без --uninstall + --install) не реализован.
// Для смены ядра: --uninstall → --install --core <name> → --restart.
// Пока разрешено сменить только на уже зарегистрированное и установленное ядро.
func handleCore(ctx context.Context, args []string) error {
	if len(args) == 0 {
		c := mustActiveCore()
		fmt.Printf("Активное ядро: %s\n", c.Name())
		fmt.Printf("Бинарь:        %s\n", c.BinaryPath())
		fmt.Printf("Конфиг:        %s\n", c.ConfigPath())
		return nil
	}

	name := args[0]
	target, err := core.Get(name)
	if err != nil {
		return fmt.Errorf("--core: %w", err)
	}

	st, err := loadState()
	if err != nil {
		return fmt.Errorf("--core: %w", err)
	}
	current := st.Core
	if current == "" {
		current = "sing-box"
	}
	if current == name {
		fmt.Printf("Активное ядро: %s (без изменений)\n", name)
		return nil
	}

	// Runtime hot-swap не реализован.
	// Безопасная семантика: меняем state.Core только если бинарь целевого ядра
	// уже установлен. Иначе явно отсылаем к --core-install.
	if _, statErr := os.Stat(target.BinaryPath()); statErr != nil {
		return fmt.Errorf(
			"--core: ядро %q зарегистрировано, но бинарь не установлен в %s. "+
				"Runtime hot-swap (без --uninstall + --install) не реализован. "+
				"Для смены ядра: --uninstall → --install --core %s → --restart",
			name, target.BinaryPath(), name)
	}

	st.Core = name
	if err := saveState(st); err != nil {
		return fmt.Errorf("--core: сохранение state: %w", err)
	}
	fmt.Printf("Активное ядро в state.json изменено: %s → %s\n", current, name)
	fmt.Println("Внимание: для применения требуется --restart (config будет регенерирован под новое ядро).")
	return nil
}

// handleCoreInstall — `--core-install <name>`: скачивает и устанавливает
// бинарь указанного ядра без активации (state.Core не меняется). Это
// позволяет иметь несколько ядер на диске одновременно — потом переключаться
// через `--core <name> --restart`.
//
// Идемпотентен: если бинарь уже установлен той же версии (ETag совпадает),
// загрузка не повторяется. Перезапись поверх установленного бинаря допустима
// (atomicfs делает atomic temp+rename).
//
// Не валидирует config — на этом этапе config ядра ещё не существует
// (он создастся только при `--core <name> --restart` или `--install --core <name>`).
func handleCoreInstall(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--core-install: требуется имя ядра (sing-box | xray | mihomo)")
	}
	name := args[0]
	c, err := core.Get(name)
	if err != nil {
		return fmt.Errorf("--core-install: %w", err)
	}

	arch, err := types.DetectHostArch()
	if err != nil {
		return fmt.Errorf("--core-install: %w", err)
	}

	if err = os.MkdirAll(c.CacheDir(), 0o755); err != nil {
		return fmt.Errorf("--core-install: создание cache dir: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(c.BinaryPath()), 0o755); err != nil {
		return fmt.Errorf("--core-install: создание bin dir: %w", err)
	}

	fmt.Printf("Загрузка %s (arch=%s)...\n", c.Name(), arch)
	dl, err := c.Download(ctx, arch, c.CacheDir())
	if err != nil {
		return fmt.Errorf("--core-install: download: %w", err)
	}
	if !dl.Downloaded {
		fmt.Printf("Архив актуален (ETag): %s\n", dl.Path)
	} else {
		fmt.Printf("Скачано: %s (%s)\n", dl.Path, dl.Version)
	}

	fmt.Printf("Установка в %s...\n", c.BinaryPath())
	if err := c.Install(ctx, exectx.OS, dl.Path); err != nil {
		return fmt.Errorf("--core-install: install: %w", err)
	}

	ver, vErr := c.BinaryVersion(ctx, exectx.OS)
	if vErr != nil {
		fmt.Printf("Установлено %s в %s (версия не прочитана: %v)\n", c.Name(), c.BinaryPath(), vErr)
	} else {
		fmt.Printf("Установлено %s v%s в %s\n", c.Name(), ver, c.BinaryPath())
	}

	st, stErr := loadState()
	active := state.DefaultCore
	if stErr == nil && st != nil && st.Core != "" {
		active = st.Core
	}
	if name != active {
		fmt.Printf("Активное ядро не изменено (sign-craze --core %s для переключения).\n", name)
	}
	return nil
}

func handleCoreList(ctx context.Context, _ []string) error {
	st, stErr := loadState()
	active := ""
	if stErr == nil && st != nil {
		active = st.Core
	}
	if active == "" {
		active = "sing-box"
	}

	names := core.Names()
	if len(names) == 0 {
		fmt.Println("Нет зарегистрированных ядер.")
		return nil
	}

	fmt.Println("Зарегистрированные ядра:")
	for _, name := range names {
		c, err := core.Get(name)
		if err != nil {
			continue
		}
		marker := "  "
		if name == active {
			marker = "* "
		}
		installed := "не установлено"
		if _, err := os.Stat(c.BinaryPath()); err == nil {
			ver, vErr := c.BinaryVersion(ctx, exectx.OS)
			if vErr == nil {
				installed = "v" + ver
			} else {
				installed = "установлено (версия не прочитана)"
			}
		}
		fmt.Printf("%s%-10s  %-32s  %s\n", marker, name, c.BinaryPath(), installed)
	}
	fmt.Println()
	fmt.Println("* — активное ядро (state.core)")
	return nil
}

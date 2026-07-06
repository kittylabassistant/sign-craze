package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/geo"
	"github.com/kittylabassistant/sign-craze/internal/log"
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
		fmt.Printf("%s %s\n", Key("Активное ядро:"), Bold(c.Name()))
		fmt.Printf("%s        %s\n", Key("Бинарь:"), Hint(c.BinaryPath()))
		fmt.Printf("%s        %s\n", Key("Конфиг:"), Hint(c.ConfigPath()))
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
		fmt.Printf("%s %s %s\n", Key("Активное ядро:"), Bold(name), Hint("(без изменений)"))
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
	fmt.Printf("%s %s → %s\n", OK("Активное ядро в state.json изменено:"), current, Bold(name))
	fmt.Println(Warn("Внимание:") + " для применения требуется --restart (config будет регенерирован под новое ядро).")
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

	fmt.Printf("%s %s (arch=%s)...\n", Info("Загрузка"), c.Name(), arch)
	dl, err := c.Download(ctx, arch, c.CacheDir())
	if err != nil {
		return fmt.Errorf("--core-install: download: %w", err)
	}
	if !dl.Downloaded {
		fmt.Printf("%s %s\n", Info("Архив актуален (ETag):"), Hint(dl.Path))
	} else {
		fmt.Printf("%s %s (%s)\n", OK("Скачано:"), Hint(dl.Path), dl.Version)
	}

	fmt.Printf("%s %s...\n", Info("Установка в"), Hint(c.BinaryPath()))
	if err := c.Install(ctx, exectx.OS, dl.Path); err != nil {
		return fmt.Errorf("--core-install: install: %w", err)
	}

	ver, vErr := c.BinaryVersion(ctx, exectx.OS)
	if vErr != nil {
		fmt.Printf("%s %s в %s (%s: %v)\n", OK("Установлено"), c.Name(), Hint(c.BinaryPath()), Warn("версия не прочитана"), vErr)
	} else {
		fmt.Printf("%s %s v%s в %s\n", OK("Установлено"), c.Name(), ver, Hint(c.BinaryPath()))
	}

	// Geo-данные (.dat) — только для ядер с GeoFormat()==GeoDAT (xray).
	// Неудача скачивания НЕ фейлит установку бинаря — только Warn + подсказка
	// повторить вручную (см. installGeoDATAssets).
	installGeoDATAssets(ctx, c)

	st, stErr := loadState()
	active := state.DefaultCore
	if stErr == nil && st != nil && st.Core != "" {
		active = st.Core
	}
	if name != active {
		fmt.Printf("%s %s\n", Hint("Активное ядро не изменено"), Hint("(sign-craze --core "+name+" для переключения)"))
	}
	return nil
}

// installGeoDATAssets скачивает geosite.dat/geoip.dat для только что
// установленного ядра c, если c.GeoFormat()==GeoDAT (xray). No-op для
// остальных форматов (sing-box/.srs качается через --update-geo отдельно;
// mihomo/.mrs — mihomo качает rule-providers сам).
//
// Ошибка скачивания НЕ фейлит --core-install: бинарь уже установлен успешно,
// поэтому geo-данные — только Warn-лог + подсказка повторить вручную через
// --update-geo --core <name>. Без geo-данных xray упадёт при первом --start с
// понятной ошибкой (internal/core/xray/render_rules.go) — это приемлемо для
// установки "про запас" (--core-install не активирует ядро сразу).
func installGeoDATAssets(ctx context.Context, c core.Core) {
	if c.GeoFormat() != core.GeoDAT {
		return
	}

	dstDir := filepath.Join(c.ConfigDir(), "assets")
	fmt.Printf("%s %s...\n", Info("Загрузка geo-данных (.dat) для"), Bold(c.Name()))

	updated, err := geoDownloadDATFn(ctx, c.CacheDir(), dstDir)
	if err != nil {
		log.L().Warn("--core-install: скачивание geo .dat не удалось", "core", c.Name(), "err", err)
		fmt.Printf("%s %v\n", Warn("Не удалось скачать geo-данные (.dat):"), err)
		fmt.Printf("  %s\n", Hint("Повторите позже: sign-craze --update-geo --core "+c.Name()))
		return
	}
	fmt.Printf("%s %d/%d geo-файлов (.dat) в %s\n", OK("Готово:"), updated, len(geo.DATFileNames), Hint(dstDir))
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
		fmt.Println(Warn("Нет зарегистрированных ядер."))
		return nil
	}

	fmt.Println(Header("Зарегистрированные ядра:"))
	for _, name := range names {
		c, err := core.Get(name)
		if err != nil {
			continue
		}
		marker := "  "
		nameStr := name
		if name == active {
			marker = Green("* ")
			nameStr = Bold(name)
		}
		installed := Err("не установлено")
		if _, err := os.Stat(c.BinaryPath()); err == nil {
			ver, vErr := c.BinaryVersion(ctx, exectx.OS)
			if vErr == nil {
				installed = OK("v" + ver)
			} else {
				installed = Warn("установлено (версия не прочитана)")
			}
		}
		fmt.Printf("%s%-10s  %-32s  %s\n", marker, nameStr, Hint(c.BinaryPath()), installed)
	}
	fmt.Println()
	fmt.Println(Hint("* — активное ядро (state.core)"))
	return nil
}

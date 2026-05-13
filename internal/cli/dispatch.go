package cli

import (
	"context"
	"fmt"
	"strings"
)

// Handler — сигнатура, которую реализует каждая подкоманда.
type Handler func(ctx context.Context, args []string) error

// Cmd описывает одну CLI-команду.
type Cmd struct {
	Short   string // однобуквенный короткий флаг, например "-i"; пустая строка если нет
	Long    string // длинный флаг, например "--install"
	Help    string
	Handler Handler
	Hidden  bool // если true — не показывать в --help (для --service-start)
}

var registry []Cmd

// Register добавляет команду в таблицу диспетчера.
// Вызывается из init() в файле каждой подкоманды.
func Register(c Cmd) {
	registry = append(registry, c)
}

// Dispatch маршрутизирует os.Args[1:] к нужному обработчику.
func Dispatch(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return showHelp()
	}

	arg := args[0]
	if arg == "--help" || arg == "-h" {
		return showHelp()
	}

	if err := validateFlag(arg); err != nil {
		return err
	}

	for _, c := range registry {
		if arg == c.Short || arg == c.Long {
			return c.Handler(ctx, args[1:])
		}
	}

	return fmt.Errorf("неизвестная команда %q (используйте --help)", arg)
}

// validateFlag отклоняет многобуквенные короткие флаги вида -ia, -ug.
func validateFlag(s string) error {
	if !strings.HasPrefix(s, "-") {
		return nil
	}
	if strings.HasPrefix(s, "--") {
		if len(s) <= 2 {
			return fmt.Errorf("флаг %q не завершён", s)
		}
		return nil
	}
	// одиночный дефис: после него допускается ровно один символ
	if len(s) != 2 {
		return fmt.Errorf("многосимвольные короткие флаги не поддерживаются: %q — используйте --%s", s, s[1:])
	}
	return nil
}

func showHelp() error {
	fmt.Println(Header("Использование:") + " sign-craze <команда> [опции]")
	fmt.Println()
	fmt.Println(Header("Команды:"))
	for _, c := range registry {
		if c.Hidden {
			continue
		}
		short := "  "
		if c.Short != "" {
			short = c.Short
		}
		fmt.Printf("  %-4s  %-24s  %s\n", short, Key(c.Long), c.Help)
	}
	fmt.Println()
	fmt.Println(Header("Глобальные опции:"))
	fmt.Printf("  %-4s  %-24s  %s\n", "", Key("--no-color"), "отключить ANSI-цвет в выводе")
	fmt.Println()
	fmt.Println(Hint("Переменные окружения: NO_COLOR (выкл.), FORCE_COLOR (вкл. даже в pipe), TERM=dumb (выкл.)."))
	return nil
}

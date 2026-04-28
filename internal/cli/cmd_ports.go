package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func init() {
	Register(Cmd{Long: "--port-add", Help: "добавить порт или диапазон (80 / 1000-1100)", Handler: handlePortAdd})
	Register(Cmd{Long: "--port-del", Help: "удалить порт или диапазон", Handler: handlePortDel})
	Register(Cmd{Long: "--port-list", Help: "перечислить настроенные порты", Handler: handlePortList})
}

const maxPortRangeSize = 1000

func handlePortAdd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--port-add: требуется порт или диапазон")
	}
	ports, err := parsePortSpec(args[0])
	if err != nil {
		return fmt.Errorf("--port-add: %w", err)
	}
	return withLock(ctx, func() error {
		st, err := loadState()
		if err != nil {
			return err
		}
		exist := make(map[uint16]bool, len(st.Ports))
		for _, p := range st.Ports {
			exist[p] = true
		}
		for _, p := range ports {
			if !exist[p] {
				st.Ports = append(st.Ports, p)
				exist[p] = true
			}
		}
		if err := saveState(st); err != nil {
			return err
		}
		fmt.Printf("Добавлено %d портов. Применить: sign-craze --restart\n", len(ports))
		return nil
	})
}

func handlePortDel(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--port-del: требуется порт или диапазон")
	}
	ports, err := parsePortSpec(args[0])
	if err != nil {
		return fmt.Errorf("--port-del: %w", err)
	}
	delSet := make(map[uint16]bool, len(ports))
	for _, p := range ports {
		delSet[p] = true
	}
	return withLock(ctx, func() error {
		st, err := loadState()
		if err != nil {
			return err
		}
		out := st.Ports[:0]
		for _, p := range st.Ports {
			if !delSet[p] {
				out = append(out, p)
			}
		}
		st.Ports = out
		if err := saveState(st); err != nil {
			return err
		}
		fmt.Println("Порты удалены. Применить: sign-craze --restart")
		return nil
	})
}

func handlePortList(_ context.Context, _ []string) error {
	st, err := loadState()
	if err != nil {
		return err
	}
	if len(st.Ports) == 0 {
		fmt.Println("(пусто)")
		return nil
	}
	for _, p := range st.Ports {
		fmt.Println(p)
	}
	return nil
}

// parsePortSpec разбирает "80" или "1000-1100".
func parsePortSpec(s string) ([]uint16, error) {
	if i := strings.Index(s, "-"); i >= 0 {
		from, err := strconv.Atoi(s[:i])
		if err != nil {
			return nil, fmt.Errorf("начало диапазона: %w", err)
		}
		to, err := strconv.Atoi(s[i+1:])
		if err != nil {
			return nil, fmt.Errorf("конец диапазона: %w", err)
		}
		if from <= 0 || to <= 0 || from > 65535 || to > 65535 || from > to {
			return nil, fmt.Errorf("некорректный диапазон %d-%d", from, to)
		}
		if to-from+1 > maxPortRangeSize {
			return nil, fmt.Errorf("диапазон превышает лимит %d", maxPortRangeSize)
		}
		out := make([]uint16, 0, to-from+1)
		for p := from; p <= to; p++ {
			out = append(out, uint16(p))
		}
		return out, nil
	}
	p, err := strconv.Atoi(s)
	if err != nil || p <= 0 || p > 65535 {
		return nil, fmt.Errorf("некорректный порт %q", s)
	}
	return []uint16{uint16(p)}, nil
}

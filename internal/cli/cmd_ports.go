package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/state"
)

func init() {
	Register(Cmd{Long: "--port-add", Help: "добавить порт или диапазон (80 / 1000-1100)", Handler: handlePortAdd})
	Register(Cmd{Long: "--port-del", Help: "удалить порт или диапазон", Handler: handlePortDel})
	Register(Cmd{Long: "--port-list", Help: "перечислить настроенные порты", Handler: handlePortList})
}

const maxPortRangeSize = 1000

// portsStatePath — путь к state.json, которым пользуются handlePortAdd/Del/List.
// var (а не прямая ссылка на константу state.DefaultPath), чтобы юнит-тесты
// могли подменить путь на временный файл: без этого тесты били бы напрямую в
// /opt/etc/sign-craze/state.json, который недоступен на запись вне роутера.
// В проде значение равно state.DefaultPath.
var portsStatePath = state.DefaultPath

// handlePortAdd, handlePortDel и handlePortList раньше мутировали state.json
// напрямую через loadState()/saveState() под CLI-локом (locks.DefaultPath).
// Web UI для тех же данных использует state.NewPortsManager, который лочит
// ДРУГОЙ файл (statePath + ".lock", см. withStateLock в internal/state/managers.go).
// Два независимых flock на один state.json не исключали гонку CLI vs Web UI
// (TOCTOU: raw Load → mutate → raw Save мог затереть конкурентную правку,
// применённую и сохранённую менеджером между raw Load и raw Save) — баг B4.
// Фикс: обе стороны используют один и тот же state.NewPortsManager, а значит
// один и тот же lock-файл.

func handlePortAdd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--port-add: требуется порт или диапазон")
	}
	ports, err := parsePortSpec(args[0])
	if err != nil {
		return fmt.Errorf("--port-add: %w", err)
	}
	return withLock(ctx, func() error {
		return addPorts(ctx, portsStatePath, ports)
	})
}

// addPorts добавляет каждый порт из уже развёрнутого диапазона через
// state.NewPortsManager — тот же менеджер (и тот же lock-файл на state.json),
// которым пользуется Web UI. AddPort вызывается поштучно: дедуп и идемпотентность
// уже реализованы в PortsManager.AddPort, батчить незачем.
//
// Вынесено из handlePortAdd отдельной функцией, чтобы тестировать саму мутацию
// без внешнего withLock: тот лочит системный /opt/var/lock/sign-craze.lock —
// путь захардкожен в lock.go и не подменяется в юнит-тестах (доп. сериализация
// CLI, к самой гонке B4 отношения не имеет).
func addPorts(ctx context.Context, statePath string, ports []uint16) error {
	mgr := state.NewPortsManager(statePath)
	for _, p := range ports {
		if err := mgr.AddPort(ctx, int(p)); err != nil {
			return fmt.Errorf("--port-add: %w", err)
		}
	}
	fmt.Printf("Добавлено %d портов. Применить: sign-craze --restart\n", len(ports))
	return nil
}

func handlePortDel(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--port-del: требуется порт или диапазон")
	}
	ports, err := parsePortSpec(args[0])
	if err != nil {
		return fmt.Errorf("--port-del: %w", err)
	}
	return withLock(ctx, func() error {
		return delPorts(ctx, portsStatePath, ports)
	})
}

// delPorts удаляет каждый порт из уже развёрнутого диапазона через
// state.NewPortsManager (см. комментарий addPorts). Удаление порта, которого
// нет в state — no-op (семантика PortsManager.DeletePort), ошибка не возвращается.
func delPorts(ctx context.Context, statePath string, ports []uint16) error {
	mgr := state.NewPortsManager(statePath)
	for _, p := range ports {
		if err := mgr.DeletePort(ctx, int(p)); err != nil {
			return fmt.Errorf("--port-del: %w", err)
		}
	}
	fmt.Println("Порты удалены. Применить: sign-craze --restart")
	return nil
}

func handlePortList(ctx context.Context, _ []string) error {
	mgr := state.NewPortsManager(portsStatePath)
	list, err := mgr.ListPorts(ctx)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Println("(пусто)")
		return nil
	}
	for _, p := range list {
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

package exectx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/log"
)

// entwarePaths — дополнительные каталоги для PATH. На Keenetic с Entware
// бинарники (ipset, iptables, ip из iproute2-tiny и т.д.) живут в /opt/sbin
// и /opt/bin; при запуске sign-craze через init.d shim родительский PATH
// часто ограничен `/usr/sbin:/usr/bin:/sbin:/bin` и эти бинари не находятся.
// Префиксуем PATH в каждом exec, чтобы LookPath отрабатывал корректно.
var entwarePaths = []string{"/opt/sbin", "/opt/bin", "/opt/usr/sbin", "/opt/usr/bin"}

// lookPathExtended ищет name по расширенному PATH (entwarePaths + текущий PATH).
// Аналог exec.LookPath, но с гарантированным включением /opt/{sbin,bin}.
func lookPathExtended(name string) (string, error) {
	seen := make(map[string]bool)
	dirs := make([]string, 0, len(entwarePaths)+8)
	for _, d := range entwarePaths {
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	for _, d := range strings.Split(os.Getenv("PATH"), ":") {
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	for _, d := range dirs {
		full := d + "/" + name
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}
		// Mode bit check: выполнимый.
		if info.Mode()&0o111 != 0 {
			return full, nil
		}
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

// Result содержит вывод завершённой команды.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Runner выполняет внешние команды.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (Result, error)
}

// OS — стандартная реализация Runner через os/exec.
var OS Runner = &osRunner{}

type osRunner struct{}

func (r *osRunner) Run(ctx context.Context, name string, args ...string) (Result, error) {
	log.L().Debug("exec", "cmd", name, "args", args)

	// Resolve binary через расширенный PATH (включая /opt/{sbin,bin} для
	// Entware на Keenetic). Нельзя полагаться на exec.Command default LookPath:
	// он использует PATH текущего процесса, который при запуске через init.d
	// shim часто не содержит /opt-каталогов.
	resolved := name
	if !strings.ContainsRune(name, '/') {
		if p, lookErr := lookPathExtended(name); lookErr == nil {
			resolved = p
		}
	}

	cmd := exec.CommandContext(ctx, resolved, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	res := Result{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: exitCode,
	}

	log.L().Debug("exec завершён", "cmd", name, "exit", res.ExitCode,
		"stderr", string(res.Stderr))

	if err != nil {
		return res, fmt.Errorf("exec %s: %w (stderr: %s)", name, err, res.Stderr)
	}
	return res, nil
}

// Mock возвращает Runner, отвечающий на команды из таблицы подстановок.
// Ключ формата: "<name> <arg1> <arg2>..." (через пробел). Неизвестные команды возвращают ошибку.
func Mock(table map[string]Result) Runner {
	return &mockRunner{table: table}
}

// MockMatcher возвращает Runner с произвольной логикой матчинга.
// Полезно когда аргументы команды содержат динамические пути (tempdir и т.п.).
func MockMatcher(fn func(name string, args ...string) (Result, error)) Runner {
	return &matcherRunner{fn: fn}
}

type matcherRunner struct {
	fn func(name string, args ...string) (Result, error)
}

func (m *matcherRunner) Run(_ context.Context, name string, args ...string) (Result, error) {
	return m.fn(name, args...)
}

type mockRunner struct {
	table map[string]Result
}

func (m *mockRunner) Run(_ context.Context, name string, args ...string) (Result, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	res, ok := m.table[key]
	if !ok {
		return Result{ExitCode: 1}, fmt.Errorf("mock: нет записи для %q", key)
	}
	if res.ExitCode != 0 {
		return res, fmt.Errorf("mock: команда %q завершилась с кодом %d (stderr: %s)", key, res.ExitCode, res.Stderr)
	}
	return res, nil
}

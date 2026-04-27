package exectx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/kittylabassistant/sign-craze/internal/log"
)

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

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		ExitCode: cmd.ProcessState.ExitCode(),
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
		return res, fmt.Errorf("mock: команда %q завершилась с кодом %d", key, res.ExitCode)
	}
	return res, nil
}

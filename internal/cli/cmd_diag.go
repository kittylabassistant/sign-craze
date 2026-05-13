package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/diag"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

func init() {
	Register(Cmd{Short: "-D", Long: "--diag", Help: "самодиагностика", Handler: handleDiag})
}

func handleDiag(ctx context.Context, _ []string) error {
	c := mustActiveCore()
	deps := diag.DefaultDeps(exectx.OS, c.NewLifecycle(), newDPILifecycle())
	results := diag.Run(ctx, diag.DefaultChecks(deps))

	for _, r := range results {
		fmt.Printf("%-20s [%s] %s\n", r.Name, coloredDiagStatus(r.Status), r.Detail)
	}
	if diag.AnyFail(results) {
		return errors.New("диагностика не пройдена")
	}
	return nil
}

// coloredDiagStatus раскрашивает [PASS]/[WARN]/[FAIL] для интерактивного
// вывода диагностики. Палитра консистентна с --status (зелёный = OK,
// жёлтый = предупреждение, красный = ошибка).
func coloredDiagStatus(s diag.Status) string {
	switch s {
	case diag.PASS:
		return OK(string(s))
	case diag.WARN:
		return Warn(string(s))
	case diag.FAIL:
		return Err(string(s))
	default:
		return string(s)
	}
}

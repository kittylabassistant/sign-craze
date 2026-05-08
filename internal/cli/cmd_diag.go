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
		fmt.Printf("%-20s [%s] %s\n", r.Name, r.Status, r.Detail)
	}
	if diag.AnyFail(results) {
		return errors.New("диагностика не пройдена")
	}
	return nil
}

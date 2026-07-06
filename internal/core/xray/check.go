package xray

import (
	"context"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// CheckConfig валидирует конфиг через xray.
// Xray v25.x убрал отдельную подкоманду `test`; теперь канонический способ —
// `xray run -test -c <config>`. Старые версии (≤ v24) понимали `xray test -c`.
// Сначала пробуем новый синтаксис; при "unknown command" из stderr — fallback
// на legacy. Изолирован от parent ctx cancel — Ctrl+C не прерывает проверку
// (см. core.DeadlineCtx).
func CheckConfig(ctx context.Context, runner exectx.Runner, binPath, configPath string) error {
	log.L().Info("xray test: проверка конфига (slow MIPS до 60s)")
	cctx, cancel := core.DeadlineCtx(ctx, core.CheckConfigTimeout)
	defer cancel()

	start := time.Now()
	res, err := runner.Run(cctx, binPath, "run", "-test", "-c", configPath)
	dur := time.Since(start)
	if err != nil && isUnknownCommand(string(res.Stderr)) {
		// Fallback для старых версий (≤ v24): подкоманда test существовала.
		log.L().Warn("xray run -test не поддерживается, пробую legacy 'xray test -c'")
		start = time.Now()
		res, err = runner.Run(cctx, binPath, "test", "-c", configPath)
		dur = time.Since(start)
	}
	if err != nil {
		return core.CheckConfigError(cctx, "xray test", core.CheckConfigTimeout, dur, res, err)
	}
	log.L().Info("xray test: ok", "duration", dur.String())
	return nil
}

// isUnknownCommand детектирует stderr вида "xray <name>: unknown command".
func isUnknownCommand(stderr string) bool {
	return strings.Contains(stderr, "unknown command") || strings.Contains(stderr, "unknown subcommand")
}

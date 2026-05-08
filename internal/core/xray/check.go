package xray

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// checkConfigTimeout — потолок длительности `xray test`. На MIPS softfloat
// проверка большого конфига может занимать десятки секунд; SIGINT от пользователя
// не должен убивать процесс посреди валидации.
const checkConfigTimeout = 180 * time.Second

// CheckConfig валидирует конфиг через xray.
// Xray v25.x убрал отдельную подкоманду `test`; теперь канонический способ —
// `xray run -test -c <config>`. Старые версии (≤ v24) понимали `xray test -c`.
// Сначала пробуем новый синтаксис; при "unknown command" из stderr — fallback
// на legacy. Изолирован от parent ctx cancel — Ctrl+C не прерывает проверку.
func CheckConfig(ctx context.Context, runner exectx.Runner, binPath, configPath string) error {
	log.L().Info("xray test: проверка конфига (slow MIPS до 60s)")
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), checkConfigTimeout)
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
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("xray test: таймаут %s — медленный CPU или зависание (stderr: %s, stdout: %s)",
				checkConfigTimeout, res.Stderr, res.Stdout)
		}
		return fmt.Errorf("xray test: %w (длительность: %s, exit: %d, stderr: %s, stdout: %s)",
			err, dur, res.ExitCode, res.Stderr, res.Stdout)
	}
	log.L().Info("xray test: ok", "duration", dur.String())
	return nil
}

// isUnknownCommand детектирует stderr вида "xray <name>: unknown command".
func isUnknownCommand(stderr string) bool {
	return strings.Contains(stderr, "unknown command") || strings.Contains(stderr, "unknown subcommand")
}

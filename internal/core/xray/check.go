package xray

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// checkConfigTimeout — потолок длительности `xray test`. На MIPS softfloat
// проверка большого конфига может занимать десятки секунд; SIGINT от пользователя
// не должен убивать процесс посреди валидации.
const checkConfigTimeout = 180 * time.Second

// CheckConfig запускает `xray test -c configPath` для валидации конфига.
// Изолирован от parent ctx cancel — Ctrl+C пользователя не прерывает проверку.
//
// Поддерживается также legacy-флаг `-test`: некоторые форки/старые версии
// не понимают подкоманду `test`. Сначала пробуем новый синтаксис, при
// `unknown command` падаем на legacy.
func CheckConfig(ctx context.Context, runner exectx.Runner, binPath, configPath string) error {
	log.L().Info("xray test: проверка конфига (slow MIPS до 60s)")
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), checkConfigTimeout)
	defer cancel()

	start := time.Now()
	res, err := runner.Run(cctx, binPath, "test", "-c", configPath)
	dur := time.Since(start)
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

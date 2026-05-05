package mihomo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// checkConfigTimeout — потолок длительности `mihomo -t`. На MIPS softfloat
// проверка большого конфига может занимать десятки секунд.
const checkConfigTimeout = 180 * time.Second

// CheckConfig запускает `mihomo -t -d <dir>` для валидации конфига.
//
// Mihomo принимает директорию (не файл), поэтому если configPath указывает
// на файл — берётся его родительская директория. Это обеспечивает совместимость
// с core.Core.CheckConfig(configPath string), где caller передаёт путь к файлу.
func CheckConfig(ctx context.Context, runner exectx.Runner, binPath, configPath string) error {
	configDir := configPath
	// Если указан путь до файла — берём его dir.
	// Эвристика: имя оканчивается на .yaml/.yml/.json.
	switch filepath.Ext(configPath) {
	case ".yaml", ".yml", ".json":
		configDir = filepath.Dir(configPath)
	}

	log.L().Info("mihomo -t: проверка конфига", "dir", configDir)
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), checkConfigTimeout)
	defer cancel()

	start := time.Now()
	res, err := runner.Run(cctx, binPath, "-t", "-d", configDir)
	dur := time.Since(start)
	if err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("mihomo -t: таймаут %s — медленный CPU или зависание (stderr: %s, stdout: %s)",
				checkConfigTimeout, res.Stderr, res.Stdout)
		}
		return fmt.Errorf("mihomo -t: %w (длительность: %s, exit: %d, stderr: %s, stdout: %s)",
			err, dur, res.ExitCode, res.Stderr, res.Stdout)
	}
	log.L().Info("mihomo -t: ok", "duration", dur.String())
	return nil
}

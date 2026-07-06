package mihomo

import (
	"context"
	"path/filepath"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// CheckConfig запускает `mihomo -t -d <dir>` для валидации конфига.
//
// Mihomo принимает директорию (не файл), поэтому если configPath указывает
// на файл — берётся его родительская директория. Это обеспечивает совместимость
// с core.Core.CheckConfig(configPath string), где caller передаёт путь к файлу.
// Изолирован от parent ctx cancel — Ctrl+C не прерывает проверку (см. core.DeadlineCtx).
func CheckConfig(ctx context.Context, runner exectx.Runner, binPath, configPath string) error {
	configDir := configPath
	// Если указан путь до файла — берём его dir.
	// Эвристика: имя оканчивается на .yaml/.yml/.json.
	switch filepath.Ext(configPath) {
	case ".yaml", ".yml", ".json":
		configDir = filepath.Dir(configPath)
	}

	log.L().Info("mihomo -t: проверка конфига", "dir", configDir)
	cctx, cancel := core.DeadlineCtx(ctx, core.CheckConfigTimeout)
	defer cancel()

	start := time.Now()
	res, err := runner.Run(cctx, binPath, "-t", "-d", configDir)
	dur := time.Since(start)
	if err != nil {
		return core.CheckConfigError(cctx, "mihomo -t", core.CheckConfigTimeout, dur, res, err)
	}
	log.L().Info("mihomo -t: ok", "duration", dur.String())
	return nil
}

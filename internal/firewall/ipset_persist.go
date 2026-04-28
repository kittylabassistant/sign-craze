package firewall

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// DefaultDumpFile — путь файла дампа ipset, переживающего ребут.
// Лежит на entware/USB вместе с гео-файлами (safety-fixes #17).
const DefaultDumpFile = "/opt/share/sign-craze/ipset.dump"

// SaveIPSet вызывает `ipset save <names...>` и атомарно записывает результат
// в file. Используется после --update-geo для сохранения состояния kernel
// ipset в файл, который читается при --service-start (после ребута).
func SaveIPSet(ctx context.Context, runner exectx.Runner, file string, names []string) error {
	if len(names) == 0 {
		return errors.New("firewall save: список наборов пуст")
	}
	args := append([]string{"save"}, names...)
	res, err := runner.Run(ctx, "ipset", args...)
	if err != nil {
		return fmt.Errorf("firewall save: ipset save: %w (stderr: %s)", err, res.Stderr)
	}
	if err := atomicfs.WriteFileAtomic(file, res.Stdout, 0o644); err != nil {
		return fmt.Errorf("firewall save: запись %s: %w", file, err)
	}
	log.L().Info("ipset dump сохранён", "path", file, "bytes", len(res.Stdout))
	return nil
}

// RestoreIPSet вызывает `ipset restore -! -f file`. Если файл отсутствует —
// возвращает nil (первый запуск до --update-geo). Флаг -! заставляет ipset
// игнорировать "уже существует" — идемпотентно.
func RestoreIPSet(ctx context.Context, runner exectx.Runner, file string) error {
	info, err := os.Stat(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.L().Debug("ipset dump отсутствует, restore пропущен", "path", file)
			return nil
		}
		return fmt.Errorf("firewall restore: stat %s: %w", file, err)
	}
	if info.Size() == 0 {
		log.L().Debug("ipset dump пуст, restore пропущен", "path", file)
		return nil
	}
	res, err := runner.Run(ctx, "ipset", "restore", "-!", "-f", file)
	if err != nil {
		return fmt.Errorf("firewall restore: ipset restore: %w (stderr: %s)", err, res.Stderr)
	}
	log.L().Info("ipset dump восстановлен", "path", file, "size", info.Size())
	return nil
}

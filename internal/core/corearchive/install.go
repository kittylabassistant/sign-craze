package corearchive

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// installBinPerm — права финального бинаря ядра. Совпадает с прежним
// хардкодом в xray.Install/mihomo.Install (0o755, исполняемый для всех).
const installBinPerm = 0o755

// InstallParams описывает один прогон InstallWithRollback.
type InstallParams struct {
	// Label — имя ядра для логов и текстов ошибок ("xray", "mihomo").
	Label string
	// ArchivePath — путь к архиву-источнику; используется только в
	// структурированном логе начала установки (поле "archive").
	ArchivePath string
	// BinDst — путь установки бинаря.
	BinDst string
	// Opener открывает архив своего формата, находит бинарь, проверяет
	// ELF-magic (см. CheckELF) и возвращает поток на его содержимое.
	// Специфичен для каждого ядра (zip у xray, plain gzip у mihomo).
	// Ошибка Opener'а оборачивается InstallWithRollback только префиксом
	// "<Label> install: " — конкретный глагол-контекст (напр. "распаковка
	// zip: %w") caller должен добавить сам, чтобы сохранить диагностику.
	Opener func() (*BinaryStream, error)
	// CheckConfig — валидация уже записанного бинаря (напр. `<bin> check -c
	// config`). nil — валидация пропускается (аналог пустого configPath у
	// caller'а старых xray.Install/mihomo.Install).
	CheckConfig func(ctx context.Context) error
}

// InstallWithRollback — общая рамка установки бинаря ядра из архива:
//
//  1. Opener открывает архив и отдаёт поток на ELF-бинарь.
//  2. Backup существующего BinDst (если есть) и атомарная потоковая запись
//     нового бинаря (atomicfs.BackupAndReplaceFromReader).
//  3. Если задан CheckConfig — валидация; при ошибке — откат backup'а и
//     возврат ошибки, упоминающей причину.
//
// Это разделяемый скелет xray.Install/mihomo.Install: единственное, что
// варьируется между ядрами — формат архива (Opener) и вызов конкретного
// бинаря для валидации конфига (CheckConfig).
//
// singbox эту рамку не использует — там валидация конфига происходит ДО
// подмены бинаря (см. internal/singbox.PrepareAndValidate), а не после с
// откатом; архитектуры сознательно разные.
func InstallWithRollback(ctx context.Context, p InstallParams) error {
	log.L().Info("установка "+p.Label, "archive", p.ArchivePath, "dst", p.BinDst)

	stream, err := p.Opener()
	if err != nil {
		return fmt.Errorf("%s install: %w", p.Label, err)
	}
	defer stream.Close()

	backupPath, err := atomicfs.BackupAndReplaceFromReader(p.BinDst, stream.Reader, installBinPerm)
	if err != nil {
		return fmt.Errorf("%s install: запись бинаря: %w", p.Label, err)
	}

	if p.CheckConfig != nil {
		if err := p.CheckConfig(ctx); err != nil {
			log.L().Warn(p.Label+": конфиг невалиден, откат бинаря", "backup", backupPath, "err", err)
			if backupPath != "" {
				if restoreErr := atomicfs.RestoreBackup(backupPath, p.BinDst); restoreErr != nil {
					return fmt.Errorf("%s install: конфиг невалиден И откат не удался: %w (откат: %w)", p.Label, err, restoreErr)
				}
			}
			return fmt.Errorf("%s install: проверка конфига: %w", p.Label, err)
		}
	}

	log.L().Info(p.Label+" установлен", "path", p.BinDst)
	return nil
}

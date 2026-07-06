package singbox

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/elfcheck"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

const (
	// DefaultBinPath — путь установки бинаря sing-box.
	DefaultBinPath = "/opt/sbin/sing-box"
	// DefaultConfigDir — директория конфигурации.
	DefaultConfigDir = "/opt/etc/sign-craze"
	// DefaultCacheDir — постоянный каталог под кеш tarball'ов и
	// временную распаковку. Использует /opt (флешка, GB места), а не /tmp
	// (tmpfs ~50MB на Keenetic), чтобы избежать "no space left on device"
	// при распаковке ~12MB sing-box.
	DefaultCacheDir = "/opt/var/lib/sign-craze/cache"
)

// PrepareAndValidate распаковывает бинарь во временный файл и валидирует конфиг
// (sing-box check) ДО установки в финальный путь. Это защищает от ситуации
// "бинарь установлен, конфиг битый" (safety-fixes #10).
//
// Возвращает путь к временному бинарю (chmod 0755). Вызывающий обязан после
// успешной валидации сделать атомарный move temp → final binPath, либо удалить
// temp при отказе. Конфиг в configPath остаётся как есть (либо валиден, либо
// требует ручной коррекции/отката).
// workDir — родительская директория для temp-папки распаковки. Должна быть
// на постоянной FS с достаточным местом (см. DefaultCacheDir). Пустая строка =
// system tmp (TMPDIR/`/tmp`); подходит только для тестов на хосте.
func PrepareAndValidate(ctx context.Context, runner exectx.Runner, workDir, tarPath, configPath string, params ConfigParams) (tempBinPath string, err error) {
	tmpDir, err := os.MkdirTemp(workDir, "sign-craze-install-*")
	if err != nil {
		return "", fmt.Errorf("singbox prepare: tempdir: %w", err)
	}
	tempBin := filepath.Join(tmpDir, "sing-box")
	if err := extractBinaryToFile(tarPath, tempBin, 0o755); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("singbox prepare: распаковка: %w", err)
	}

	// Defense-in-depth: если бинарь динамически слинкован (PT_INTERP) и его
	// интерпретатор отсутствует в системе (glibc-сборка на musl/Entware) — ранняя
	// понятная ошибка вместо криптичного "fork/exec: no such file or directory"
	// из sing-box check (issue #3).
	if err := elfcheck.CheckInterpCompatibility(tempBin); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("singbox prepare: несовместимый бинарь: %w", err)
	}

	// WriteConfig пишет конфиг и валидирует через `tempBin check -c configPath`.
	if err := WriteConfig(ctx, runner, params, tempBin, configPath); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("singbox prepare: валидация конфига: %w", err)
	}

	return tempBin, nil
}

// binaryStream — потоковый ридер на содержимое бинаря из tarball.
// Caller обязан вызвать Close() для освобождения tar/gzip/file.
type binaryStream struct {
	Reader io.Reader
	closer func() error
}

func (b *binaryStream) Close() error { return b.closer() }

// openSingboxBinaryStream открывает tarball, ищет файл "sing-box" в любом
// подкаталоге, проверяет ELF-magic первых 4 байт и возвращает stream на
// полное содержимое (включая magic). Не буферизует бинарь в RAM —
// критично для роутеров с 128MB.
func openSingboxBinaryStream(tarPath string) (*binaryStream, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("открытие tarball: %w", err)
	}

	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("gzip reader: %w", err)
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			_ = gz.Close()
			_ = f.Close()
			return nil, fmt.Errorf("бинарь 'sing-box' не найден в архиве %s", tarPath)
		}
		if err != nil {
			_ = gz.Close()
			_ = f.Close()
			return nil, fmt.Errorf("чтение tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "sing-box" && !strings.HasSuffix(hdr.Name, "/sing-box") {
			continue
		}

		// ELF-magic проверяется без чтения всего бинаря: первые 4 байта
		// в буфер, остаток стримится через MultiReader.
		full, got, n, readErr := elfcheck.CheckAndRewind(tr)
		if readErr != nil {
			_ = gz.Close()
			_ = f.Close()
			return nil, fmt.Errorf("чтение ELF-magic: %w", readErr)
		}
		if !elfcheck.IsELF(got, n) {
			_ = gz.Close()
			_ = f.Close()
			return nil, fmt.Errorf("singbox install: %s/sing-box не ELF-бинарь (magic=%x)",
				filepath.Dir(hdr.Name), got[:n])
		}
		closer := func() error {
			_ = gz.Close()
			return f.Close()
		}
		return &binaryStream{Reader: full, closer: closer}, nil
	}
}

// extractBinaryToFile стримит бинарь sing-box из tarball напрямую в dstPath
// через atomicfs.WriteFileAtomicFromReader. Не загружает бинарь в RAM.
func extractBinaryToFile(tarPath, dstPath string, perm os.FileMode) error {
	stream, err := openSingboxBinaryStream(tarPath)
	if err != nil {
		return err
	}
	defer stream.Close()

	if err := atomicfs.WriteFileAtomicFromReader(dstPath, stream.Reader, perm); err != nil {
		return fmt.Errorf("запись бинаря: %w", err)
	}
	return nil
}

// checkConfig запускает `sing-box check -c configPath` для валидации конфига.
// Изолирован от parent ctx cancel — Ctrl+C пользователя не прерывает проверку
// (см. core.DeadlineCtx).
func checkConfig(ctx context.Context, runner exectx.Runner, binPath, configPath string) error {
	log.L().Info("sing-box check: проверка конфига (на slow MIPS CPU до 60s)")
	cctx, cancel := core.DeadlineCtx(ctx, core.CheckConfigTimeout)
	defer cancel()

	start := time.Now()
	res, err := runner.Run(cctx, binPath, "check", "-c", configPath)
	dur := time.Since(start)
	if err != nil {
		return core.CheckConfigError(cctx, "sing-box check", core.CheckConfigTimeout, dur, res, err)
	}
	log.L().Info("sing-box check: ok", "duration", dur.String())
	return nil
}

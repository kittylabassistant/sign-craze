package elfcheck

import (
	"bytes"
	"debug/elf"
	"fmt"
	"io"
	"os"
	"strings"
)

// Magic — первые 4 байта Linux ELF-бинаря (\x7f E L F).
var Magic = []byte{0x7f, 'E', 'L', 'F'}

// CheckAndRewind читает первые 4 байта из r, проверяет ELF-magic и возвращает
// новый io.Reader, который включает прочитанные байты (через io.MultiReader).
// Таким образом caller получает полный поток данных без потери начальных байт.
//
// Возвращает:
//   - (rewindedReader, gotBuf, nil) — чтение прошло успешно;
//     isELF = bytes.Equal(gotBuf[:n], Magic), где n = количество прочитанных байт.
//     Caller проверяет isELF самостоятельно и формирует ошибку со значением gotBuf
//     для диагностики.
//   - (nil, [4]byte{}, err) — при реальной ошибке чтения (кроме io.ErrUnexpectedEOF).
//
// Случай io.ErrUnexpectedEOF (n < 4) не является ошибкой функции: возвращает
// прочитанные байты и isELF = false, что позволяет caller корректно
// сформировать диагностическое сообщение.
func CheckAndRewind(r io.Reader) (full io.Reader, got [4]byte, n int, err error) {
	n, readErr := io.ReadFull(r, got[:])
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		// Реальная ошибка чтения (не EOF/UnexpectedEOF)
		return nil, [4]byte{}, 0, readErr
	}
	// Прочитанные байты возвращаем обратно через MultiReader (включая случай n < 4)
	full = io.MultiReader(bytes.NewReader(got[:n]), r)
	return full, got, n, nil
}

// IsELF проверяет, совпадают ли первые n байт buf с ELF-magic.
// Используется совместно с CheckAndRewind для формирования ошибки caller'ом.
func IsELF(buf [4]byte, n int) bool {
	if n < len(Magic) {
		return false
	}
	return bytes.Equal(buf[:n], Magic)
}

// maxInterpLen — потолок длины пути ELF-интерпретатора (PT_INTERP). Реальные пути
// компоновщика короче PATH_MAX; больший размер — аномалия (битый/вредоносный ELF),
// которую не анализируем, чтобы не аллоцировать большой буфер.
const maxInterpLen = 4096

// CheckInterpCompatibility открывает уже записанный на диск ELF-бинарь по path и
// проверяет, что его динамический компоновщик (PT_INTERP) присутствует в системе.
//
// Назначение — defense-in-depth: поймать бинарь, динамически слинкованный против
// libc, которой нет в окружении (glibc-сборка на musl/Entware), и выдать понятную
// ошибку ВМЕСТО криптичного "fork/exec: no such file or directory (exit -1)" на
// этапе запуска (issue #3).
//
// Поведение:
//   - elf.Open не смог распарсить файл (не ELF / повреждён / усечённый стаб) → nil.
//     ELF-magic уже проверен при распаковке (CheckAndRewind); полноценный парсинг
//     здесь best-effort — если не удался, валидацию делегируем последующему
//     `<core> check`. Проверка не падает на том, что не смогла проанализировать.
//   - PT_INTERP отсутствует (статический бинарь) → nil (совместим всегда).
//   - PT_INTERP есть, интерпретатор существует на FS → nil.
//   - PT_INTERP есть, интерпретатор отсутствует → описательная ошибка.
//
// debug/elf — stdlib, совместим с CGO_ENABLED=0.
func CheckInterpCompatibility(path string) error {
	f, err := elf.Open(path)
	if err != nil {
		// Не ELF / повреждён — не наша забота: magic проверен ранее, остальное
		// поймает `<core> check`.
		return nil
	}
	defer func() { _ = f.Close() }()

	for _, prog := range f.Progs {
		if prog.Type != elf.PT_INTERP {
			continue
		}
		// PT_INTERP найден → бинарь динамически слинкован. Читаем путь интерпретатора.
		if prog.Filesz == 0 || prog.Filesz > maxInterpLen {
			return nil // аномальный размер — не анализируем
		}
		buf := make([]byte, prog.Filesz)
		if _, rerr := io.ReadFull(prog.Open(), buf); rerr != nil {
			return nil // не смогли прочитать сегмент — best-effort, не блокируем
		}
		interp := strings.TrimRight(string(buf), "\x00")
		if interp == "" {
			return nil
		}
		if _, serr := os.Stat(interp); serr != nil {
			return fmt.Errorf(
				"бинарь %s динамически слинкован с интерпретатором %q, которого нет "+
					"в системе (musl/Entware-окружение несовместимо с glibc-сборкой); "+
					"нужен статический вариант (напр. *-musl.tar.gz)",
				path, interp)
		}
		return nil // интерпретатор присутствует → совместим
	}
	return nil // нет PT_INTERP → статический → совместим
}

package elfcheck

import (
	"bytes"
	"io"
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

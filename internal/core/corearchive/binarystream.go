package corearchive

import (
	"fmt"
	"io"

	"github.com/kittylabassistant/sign-craze/internal/elfcheck"
)

// BinaryStream — потоковый ридер на бинарь ядра, извлечённый из архива
// (zip у xray, plain gzip у mihomo). Caller обязан вызвать Close() для
// освобождения ресурсов (файловый дескриптор, zip/gzip reader), обычно
// через defer сразу после успешного открытия.
type BinaryStream struct {
	// Reader — поток полного содержимого бинаря, включая байты, прочитанные
	// при ELF-magic проверке (см. CheckELF) — ничего не теряется.
	Reader io.Reader
	closer func() error
}

// Close освобождает ресурсы, связанные со стримом.
func (b *BinaryStream) Close() error { return b.closer() }

// NewBinaryStream создаёт BinaryStream с заданным потоком содержимого и
// cleanup-функцией. closer вызывается ровно один раз, из Close().
func NewBinaryStream(r io.Reader, closer func() error) *BinaryStream {
	return &BinaryStream{Reader: r, closer: closer}
}

// CheckELF читает первые 4 байта r, проверяет ELF-magic (elfcheck.Magic) и
// возвращает reader на ПОЛНОЕ содержимое — прочитанные для проверки байты
// возвращаются обратно через io.MultiReader (elfcheck.CheckAndRewind),
// поток не теряет данные и не требует Seek.
//
// label используется только в тексте ошибки (путь файла внутри архива,
// либо описание источника вроде "содержимое gzip") — для диагностики
// caller'ом при отвергнутом/повреждённом бинаре.
//
// Не читает и не буферизует содержимое целиком — критично для роутеров с
// 128MB RAM: последующая запись на диск идёт потоково через
// atomicfs.WriteFileAtomicFromReader.
func CheckELF(r io.Reader, label string) (io.Reader, error) {
	full, got, n, err := elfcheck.CheckAndRewind(r)
	if err != nil {
		return nil, fmt.Errorf("чтение ELF-magic (%s): %w", label, err)
	}
	if !elfcheck.IsELF(got, n) {
		return nil, fmt.Errorf("%s не ELF-бинарь (magic=%x)", label, got[:n])
	}
	return full, nil
}

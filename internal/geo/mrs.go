package geo

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// MaxMRSSize — DoS-защита. Реальные .mrs файлы редко превышают 30MB.
const MaxMRSSize int64 = 128 * 1024 * 1024

// mrsMinSize — минимальный размер .mrs файла (header без тела).
const mrsMinSize = 24

// mrsMagic — первые 3 байта заголовка .mrs.
const mrsMagic = "MRS"

// mrsMaxVersion — sanity-порог версии формата.
const mrsMaxVersion = 1000

// mrsMaxCount — максимально допустимое число записей (DoS-защита).
const mrsMaxCount uint64 = 100_000_000

// MRSHeader — распарсенный заголовок .mrs файла.
type MRSHeader struct {
	Magic    string // первые 8 байт как строка (для diag)
	Version  uint32
	Behavior uint32 // 0=domain, 1=ipcidr, 2=classical
	Count    uint64 // число записей
}

// ValidateMRS читает заголовок .mrs файла по path и проверяет:
//   - размер ≥ 24 байта (минимум под заголовок)
//   - размер ≤ MaxMRSSize
//   - первые 3 байта = "MRS" (0x4D 0x52 0x53)
//   - Version > 0 и ≤ 1000
//   - Behavior ∈ {0, 1, 2}
//   - Count > 0 и ≤ 100_000_000
//
// Сжатое тело (zstd) не разбирается — полный парсинг выполняет mihomo при старте.
// Возвращает MRSHeader (для diag-вывода) и nil при успехе.
func ValidateMRS(path string) (MRSHeader, error) {
	// Проверка размера файла.
	info, err := os.Stat(path)
	if err != nil {
		return MRSHeader{}, fmt.Errorf("geo mrs: stat %s: %w", path, err)
	}

	fileSize := info.Size()
	if fileSize < mrsMinSize {
		return MRSHeader{}, fmt.Errorf("geo mrs: размер файла %d байт меньше минимального %d", fileSize, mrsMinSize)
	}
	if fileSize > MaxMRSSize {
		return MRSHeader{}, fmt.Errorf("geo mrs: размер файла %d байт превышает максимально допустимый %d", fileSize, MaxMRSSize)
	}

	// Читаем первые 24 байта заголовка.
	f, err := os.Open(path)
	if err != nil {
		return MRSHeader{}, fmt.Errorf("geo mrs: открытие %s: %w", path, err)
	}
	defer f.Close()

	var buf [mrsMinSize]byte
	if _, err := io.ReadFull(f, buf[:]); err != nil {
		return MRSHeader{}, fmt.Errorf("geo mrs: чтение заголовка %s: %w", path, err)
	}

	// Проверка magic-префикса (первые 3 байта = "MRS").
	if string(buf[0:3]) != mrsMagic {
		return MRSHeader{}, fmt.Errorf("geo mrs: неверный magic %q, ожидается %q", string(buf[0:3]), mrsMagic)
	}

	header := MRSHeader{
		Magic:    string(buf[0:8]),
		Version:  binary.LittleEndian.Uint32(buf[8:12]),
		Behavior: binary.LittleEndian.Uint32(buf[12:16]),
		Count:    binary.LittleEndian.Uint64(buf[16:24]),
	}

	// Sanity-проверки полей заголовка.
	if header.Version == 0 {
		return MRSHeader{}, fmt.Errorf("geo mrs: version = 0 недопустима")
	}
	if header.Version > mrsMaxVersion {
		return MRSHeader{}, fmt.Errorf("geo mrs: version %d превышает максимум %d", header.Version, mrsMaxVersion)
	}

	if header.Behavior > 2 {
		return MRSHeader{}, fmt.Errorf("geo mrs: недопустимый behavior %d (ожидается 0=domain, 1=ipcidr, 2=classical)", header.Behavior)
	}

	if header.Count == 0 {
		return MRSHeader{}, fmt.Errorf("geo mrs: count = 0 недопустим")
	}
	if header.Count > mrsMaxCount {
		return MRSHeader{}, fmt.Errorf("geo mrs: count %d превышает максимум %d", header.Count, mrsMaxCount)
	}

	return header, nil
}

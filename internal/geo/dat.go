package geo

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// MaxDATSize — максимальный разумный размер geoip.dat / geosite.dat файла.
// На 2026 актуальный geoip.dat ~5-8MB, geosite.dat ~7MB. Ставим 64MB
// как защиту от DoS (битый файл с гигантским length-prefix мог бы
// привести к OOM на 128MB Keenetic).
const MaxDATSize = 64 * 1024 * 1024

// ValidateDAT читает файл по path и проверяет что это валидный
// v2ray geoip.dat / geosite.dat (protobuf GeoIPList / GeoSiteList).
//
// Возвращает количество top-level entries и nil при OK.
// При ошибке: не-protobuf формат, превышение размера, обрезанный файл.
func ValidateDAT(path string) (entries int, err error) {
	_, err = parseDAT(path, false)
	if err != nil {
		return 0, err
	}
	codes, err := parseDAT(path, true)
	if err != nil {
		return 0, err
	}
	return len(codes), nil
}

// ParseDAT возвращает список country_code (или domain category для geosite).
// Полные CIDR/Domain данные не извлекаются — это дело xray.
//
// Используется в diag для отображения "geoip.dat: 254 стран".
func ParseDAT(path string) (codes []string, err error) {
	return parseDAT(path, true)
}

// parseDAT — внутренняя реализация: читает protobuf-stream файла.
// Если extractCodes=true — извлекает строковое первое поле каждого entry.
func parseDAT(path string, extractCodes bool) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("geo dat: stat %s: %w", path, err)
	}
	if info.Size() == 0 {
		return nil, fmt.Errorf("geo dat: файл пустой: %s", path)
	}
	if info.Size() > MaxDATSize {
		return nil, fmt.Errorf("geo dat: файл слишком большой (%d байт > %d): %s",
			info.Size(), MaxDATSize, path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("geo dat: открытие %s: %w", path, err)
	}
	defer f.Close()

	var codes []string
	offset := 0

	for {
		// Читаем tag top-level поля.
		tag, n, err := readVarint(f)
		if errors.Is(err, io.EOF) {
			// Конец файла — корректное завершение (если хоть одна запись была).
			break
		}
		if err != nil {
			return nil, fmt.Errorf("geo dat: чтение tag на смещении %d: %w", offset, err)
		}
		offset += n

		fieldNumber := tag >> 3
		wireType := uint8(tag & 7)

		// GeoIPList / GeoSiteList: top-level должны быть field_number=1, wire_type=2
		// (length-delimited embedded message GeoIP / GeoSite).
		if fieldNumber != 1 {
			return nil, fmt.Errorf("geo dat: неожиданный field_number=%d на смещении %d (ожидается 1)",
				fieldNumber, offset-n)
		}
		if wireType != 2 {
			return nil, fmt.Errorf("geo dat: неожиданный wire_type=%d на смещении %d (ожидается 2=length-delimited)",
				wireType, offset-n)
		}

		// Читаем содержимое entry (length-delimited).
		body, nn, err := readLengthDelimited(f)
		if err != nil {
			return nil, fmt.Errorf("geo dat: чтение entry на смещении %d: %w", offset, err)
		}
		offset += nn

		if extractCodes {
			code, err := extractFirstStringField(body)
			if err != nil {
				return nil, fmt.Errorf("geo dat: извлечение кода из entry на смещении %d: %w",
					offset-nn, err)
			}
			codes = append(codes, code)
		} else {
			codes = append(codes, "") // просто считаем записи
		}
	}

	if len(codes) == 0 {
		return nil, fmt.Errorf("geo dat: файл не содержит ни одной записи: %s", path)
	}

	return codes, nil
}

// extractFirstStringField читает из body (содержимое одного GeoIP/GeoSite entry)
// первое поле field_number=1, wire_type=2 и возвращает его как строку
// (country_code или domain category).
func extractFirstStringField(body []byte) (string, error) {
	r := &bytesReader{data: body}

	tag, _, err := readVarint(r)
	if err != nil {
		return "", fmt.Errorf("чтение tag поля entry: %w", err)
	}

	fieldNumber := tag >> 3
	wireType := uint8(tag & 7)

	// Первое поле entry должно быть field_number=1, wire_type=2 (string).
	if fieldNumber != 1 || wireType != 2 {
		return "", fmt.Errorf("первое поле entry: field=%d wire=%d, ожидается field=1 wire=2",
			fieldNumber, wireType)
	}

	strBody, _, err := readLengthDelimited(r)
	if err != nil {
		return "", fmt.Errorf("чтение строкового поля entry: %w", err)
	}

	return string(strBody), nil
}

// readVarint читает protobuf varint из r.
// Возвращает значение и число прочитанных байт.
// Защита от malformed varint: максимум 10 байт (uint64).
func readVarint(r io.Reader) (uint64, int, error) {
	var result uint64
	buf := make([]byte, 1)

	for i := 0; i < 10; i++ {
		_, err := io.ReadFull(r, buf)
		if err != nil {
			if i == 0 {
				return 0, 0, err // EOF на первом байте — нормальный конец стрима
			}
			return 0, i, fmt.Errorf("geo dat: обрыв varint на байте %d: %w", i, err)
		}
		b := buf[0]
		result |= uint64(b&0x7F) << (7 * uint(i))
		if b&0x80 == 0 {
			return result, i + 1, nil
		}
	}

	return 0, 10, fmt.Errorf("geo dat: varint длиннее 10 байт (malformed protobuf)")
}

// readLengthDelimited читает length-prefix (varint), затем читает len байт.
// Возвращает body и общее число прочитанных байт.
func readLengthDelimited(r io.Reader) ([]byte, int, error) {
	length, n, err := readVarint(r)
	if err != nil {
		return nil, n, fmt.Errorf("geo dat: чтение length-prefix: %w", err)
	}

	// Ограничение: один entry не может быть больше MaxDATSize.
	if length > MaxDATSize {
		return nil, n, fmt.Errorf("geo dat: length-prefix %d превышает MaxDATSize %d", length, MaxDATSize)
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, n, fmt.Errorf("geo dat: чтение %d байт body: %w", length, err)
	}

	return body, n + int(length), nil
}

// skipField пропускает значение поля заданного wire_type без декодирования.
// Используется при обходе полей внутри entry (кроме первого).
func skipField(r io.Reader, wireType uint8) (int, error) {
	switch wireType {
	case 0: // varint
		_, n, err := readVarint(r)
		return n, err
	case 1: // 64-bit
		buf := make([]byte, 8)
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, fmt.Errorf("geo dat: skip 64-bit field: %w", err)
		}
		return 8, nil
	case 2: // length-delimited
		_, n, err := readLengthDelimited(r)
		return n, err
	case 5: // 32-bit
		buf := make([]byte, 4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, fmt.Errorf("geo dat: skip 32-bit field: %w", err)
		}
		return 4, nil
	default:
		return 0, fmt.Errorf("geo dat: неизвестный wire_type=%d", wireType)
	}
}

// bytesReader — io.Reader поверх []byte (без аллокаций bufio).
type bytesReader struct {
	data []byte
	pos  int
}

func (b *bytesReader) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}

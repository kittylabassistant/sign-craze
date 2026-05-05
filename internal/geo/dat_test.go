package geo

import (
	"bytes"
	"os"
	"testing"
)

// buildEntry строит protobuf-encoded embedded message GeoIP/GeoSite entry
// с одним полем field_number=1, wire_type=2 (string = code).
func buildEntry(code string) []byte {
	var buf bytes.Buffer
	// tag: field_number=1, wire_type=2 → (1 << 3) | 2 = 0x0A
	writeVarint(&buf, 0x0A)
	// length-delimited строка
	writeVarint(&buf, uint64(len(code)))
	buf.WriteString(code)
	return buf.Bytes()
}

// buildDATFile строит минимальный валидный geoip.dat / geosite.dat
// с указанными кодами стран / категорий.
func buildDATFile(codes []string) []byte {
	var buf bytes.Buffer
	for _, code := range codes {
		entry := buildEntry(code)
		// top-level: field_number=1, wire_type=2 → 0x0A
		writeVarint(&buf, 0x0A)
		writeVarint(&buf, uint64(len(entry)))
		buf.Write(entry)
	}
	return buf.Bytes()
}

// writeVarint записывает protobuf varint в buf.
func writeVarint(buf *bytes.Buffer, v uint64) {
	for v >= 0x80 {
		buf.WriteByte(byte(v&0x7F) | 0x80)
		v >>= 7
	}
	buf.WriteByte(byte(v))
}

// writeTempFile создаёт временный файл с указанным содержимым.
func writeTempFile(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp("", "geo-dat-test-*.dat")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestValidateDAT_ValidMinimal(t *testing.T) {
	// Минимальный валидный geoip.dat: одна запись "RU".
	data := buildDATFile([]string{"RU"})
	path := writeTempFile(t, data)

	entries, err := ValidateDAT(path)
	if err != nil {
		t.Fatalf("ValidateDAT: неожиданная ошибка: %v", err)
	}
	if entries != 1 {
		t.Fatalf("ValidateDAT: ожидаем 1 запись, получили %d", entries)
	}
}

func TestParseDAT_ExtractsCountryCodes(t *testing.T) {
	// Файл с 3 entries: "RU", "US", "CN".
	want := []string{"RU", "US", "CN"}
	data := buildDATFile(want)
	path := writeTempFile(t, data)

	codes, err := ParseDAT(path)
	if err != nil {
		t.Fatalf("ParseDAT: неожиданная ошибка: %v", err)
	}
	if len(codes) != len(want) {
		t.Fatalf("ParseDAT: ожидаем %d кодов, получили %d: %v", len(want), len(codes), codes)
	}
	for i, c := range codes {
		if c != want[i] {
			t.Errorf("codes[%d]: ожидаем %q, получили %q", i, want[i], c)
		}
	}
}

func TestValidateDAT_RejectsEmpty(t *testing.T) {
	path := writeTempFile(t, []byte{})

	_, err := ValidateDAT(path)
	if err == nil {
		t.Fatal("ValidateDAT: ожидаем ошибку для пустого файла")
	}
}

func TestValidateDAT_RejectsTooLarge(t *testing.T) {
	// Создаём sparse-файл размером MaxDATSize+1 через Truncate.
	f, err := os.CreateTemp("", "geo-dat-large-*.dat")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	path := f.Name()
	t.Cleanup(func() { os.Remove(path) })

	// Записываем 1 байт, затем truncate до MaxDATSize+1.
	if _, err = f.Write([]byte{0x0A}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err = f.Truncate(MaxDATSize + 1); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	f.Close()

	_, err = ValidateDAT(path)
	if err == nil {
		t.Fatal("ValidateDAT: ожидаем ошибку для файла > MaxDATSize")
	}
}

func TestValidateDAT_RejectsMalformedVarint(t *testing.T) {
	// 10+ байт с continuation-bit → malformed varint.
	data := make([]byte, 12)
	for i := range data {
		data[i] = 0xFF
	}
	path := writeTempFile(t, data)

	_, err := ValidateDAT(path)
	if err == nil {
		t.Fatal("ValidateDAT: ожидаем ошибку для malformed varint")
	}
}

func TestValidateDAT_RejectsTruncated(t *testing.T) {
	// tag=0x0A (field1, wire2), length=100, только 50 байт body → обрыв.
	var buf bytes.Buffer
	writeVarint(&buf, 0x0A)     // top-level tag
	writeVarint(&buf, 100)      // length = 100 байт
	buf.Write(make([]byte, 50)) // только 50 байт body
	path := writeTempFile(t, buf.Bytes())

	_, err := ValidateDAT(path)
	if err == nil {
		t.Fatal("ValidateDAT: ожидаем ошибку для обрезанного файла")
	}
}

func TestValidateDAT_RejectsWrongTopField(t *testing.T) {
	// Top-level field_number=2 (не entry=1) → ошибка.
	var buf bytes.Buffer
	// field_number=2, wire_type=2 → (2 << 3) | 2 = 0x12
	writeVarint(&buf, 0x12)
	entry := buildEntry("RU")
	writeVarint(&buf, uint64(len(entry)))
	buf.Write(entry)
	path := writeTempFile(t, buf.Bytes())

	_, err := ValidateDAT(path)
	if err == nil {
		t.Fatal("ValidateDAT: ожидаем ошибку для неправильного top-level field_number")
	}
}

func TestReadVarint_MultiByte(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    uint64
		wantLen int
	}{
		{"single zero", []byte{0x00}, 0, 1},
		{"300 = 0xAC 0x02", []byte{0xAC, 0x02}, 300, 2},
		{"max uint32 = 0xFF*4 + 0x0F", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, 0xFFFFFFFF, 5},
		{"single byte 127", []byte{0x7F}, 127, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &bytesReader{data: tt.input}
			got, n, err := readVarint(r)
			if err != nil {
				t.Fatalf("readVarint: неожиданная ошибка: %v", err)
			}
			if got != tt.want {
				t.Errorf("значение: ожидаем %d, получили %d", tt.want, got)
			}
			if n != tt.wantLen {
				t.Errorf("длина: ожидаем %d байт, получили %d", tt.wantLen, n)
			}
		})
	}
}

func TestReadVarint_TooLong(t *testing.T) {
	// 11 байт с continuation-bit → ошибка.
	data := make([]byte, 11)
	for i := range data {
		data[i] = 0xFF
	}
	r := &bytesReader{data: data}
	_, _, err := readVarint(r)
	if err == nil {
		t.Fatal("readVarint: ожидаем ошибку для varint > 10 байт")
	}
}

func TestValidateDAT_MultipleEntries(t *testing.T) {
	// 254 записи — проверяем масштаб.
	codes := make([]string, 254)
	for i := range codes {
		codes[i] = "XX"
	}
	data := buildDATFile(codes)
	path := writeTempFile(t, data)

	entries, err := ValidateDAT(path)
	if err != nil {
		t.Fatalf("ValidateDAT: неожиданная ошибка: %v", err)
	}
	if entries != 254 {
		t.Fatalf("ValidateDAT: ожидаем 254 записи, получили %d", entries)
	}
}

func TestSkipField_AllWireTypes(t *testing.T) {
	tests := []struct {
		name     string
		wireType uint8
		data     []byte
		wantN    int
	}{
		{"varint 0", 0, []byte{0x00}, 1},
		{"varint 300", 0, []byte{0xAC, 0x02}, 2},
		{"64-bit", 1, make([]byte, 8), 8},
		{"length-delimited 3 bytes", 2, append([]byte{0x03}, []byte("RUS")...), 4},
		{"32-bit", 5, make([]byte, 4), 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &bytesReader{data: tt.data}
			n, err := skipField(r, tt.wireType)
			if err != nil {
				t.Fatalf("skipField: неожиданная ошибка: %v", err)
			}
			if n != tt.wantN {
				t.Errorf("прочитано байт: ожидаем %d, получили %d", tt.wantN, n)
			}
		})
	}
}

func TestSkipField_UnknownWireType(t *testing.T) {
	r := &bytesReader{data: []byte{0x00}}
	_, err := skipField(r, 3) // wire_type=3 не существует в protobuf
	if err == nil {
		t.Fatal("skipField: ожидаем ошибку для неизвестного wire_type")
	}
}

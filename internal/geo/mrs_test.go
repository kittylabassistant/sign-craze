package geo

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

// makeMRS создаёт временный .mrs файл с указанным заголовком и dummy-телом.
// magic должен быть ровно 8 байт. Возвращает путь к файлу.
func makeMRS(t *testing.T, magic []byte, version, behavior uint32, count uint64) string {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "*.mrs")
	if err != nil {
		t.Fatalf("makeMRS: CreateTemp: %v", err)
	}
	defer f.Close()

	// Заголовок: 8 байт magic + 4 version + 4 behavior + 8 count = 24 байта.
	var header [mrsMinSize]byte
	// copy дополняет нулями если magic короче 8 байт (array инициализирован нулями).
	copy(header[0:8], magic)
	binary.LittleEndian.PutUint32(header[8:12], version)
	binary.LittleEndian.PutUint32(header[12:16], behavior)
	binary.LittleEndian.PutUint64(header[16:24], count)

	if _, err := f.Write(header[:]); err != nil {
		t.Fatalf("makeMRS: write header: %v", err)
	}

	// Dummy-тело: 1KB нулей, чтобы размер был явно ≥ minSize.
	body := make([]byte, 1024)
	if _, err := f.Write(body); err != nil {
		t.Fatalf("makeMRS: write body: %v", err)
	}

	return f.Name()
}

// validMagic — корректный magic-префикс "MRS\x00\x00\x00\x00\x00".
var validMagic = []byte("MRS\x00\x00\x00\x00\x00")

func TestValidateMRS_ValidHeader(t *testing.T) {
	path := makeMRS(t, validMagic, 1, 0, 100)

	hdr, err := ValidateMRS(path)
	if err != nil {
		t.Fatalf("ожидался nil, получено: %v", err)
	}
	if hdr.Version != 1 {
		t.Errorf("Version: хотели 1, получили %d", hdr.Version)
	}
	if hdr.Count != 100 {
		t.Errorf("Count: хотели 100, получили %d", hdr.Count)
	}
	if hdr.Behavior != 0 {
		t.Errorf("Behavior: хотели 0, получили %d", hdr.Behavior)
	}
	if !strings.HasPrefix(hdr.Magic, "MRS") {
		t.Errorf("Magic: хотели префикс MRS, получили %q", hdr.Magic)
	}
}

func TestValidateMRS_RejectsBadMagic(t *testing.T) {
	path := makeMRS(t, []byte("FOO\x00\x00\x00\x00\x00"), 1, 0, 100)

	_, err := ValidateMRS(path)
	if err == nil {
		t.Fatal("ожидалась ошибка для неверного magic, получен nil")
	}
	if !strings.Contains(err.Error(), "magic") {
		t.Errorf("сообщение об ошибке должно содержать 'magic', получено: %q", err.Error())
	}
}

func TestValidateMRS_RejectsTooSmall(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.mrs")
	if err != nil {
		t.Fatal(err)
	}
	// Записываем только 10 байт — меньше mrsMinSize.
	_, _ = f.Write([]byte("MRS\x00\x00\x00\x00\x00\x00\x00"))
	f.Close()

	_, err = ValidateMRS(f.Name())
	if err == nil {
		t.Fatal("ожидалась ошибка для слишком маленького файла, получен nil")
	}
	if !strings.Contains(err.Error(), "размер") {
		t.Errorf("сообщение должно содержать 'размер', получено: %q", err.Error())
	}
}

func TestValidateMRS_RejectsTooLarge(t *testing.T) {
	// Создаём временный файл, имитируем большой размер через sparse file (truncate).
	f, err := os.CreateTemp(t.TempDir(), "*.mrs")
	if err != nil {
		t.Fatal(err)
	}

	// Пишем валидный header.
	var header [mrsMinSize]byte
	copy(header[0:8], validMagic)
	binary.LittleEndian.PutUint32(header[8:12], 1)
	binary.LittleEndian.PutUint32(header[12:16], 0)
	binary.LittleEndian.PutUint64(header[16:24], 100)
	_, _ = f.Write(header[:])

	// Расширяем файл до MaxMRSSize+1 через Truncate (sparse, без реальной записи).
	bigSize := MaxMRSSize + 1
	if err = f.Truncate(bigSize); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	f.Close()

	_, err = ValidateMRS(f.Name())
	if err == nil {
		t.Fatal("ожидалась ошибка для слишком большого файла, получен nil")
	}
	if !strings.Contains(err.Error(), "размер") {
		t.Errorf("сообщение должно содержать 'размер', получено: %q", err.Error())
	}
}

func TestValidateMRS_RejectsZeroVersion(t *testing.T) {
	path := makeMRS(t, validMagic, 0, 0, 100)

	_, err := ValidateMRS(path)
	if err == nil {
		t.Fatal("ожидалась ошибка для version=0, получен nil")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("сообщение должно содержать 'version', получено: %q", err.Error())
	}
}

func TestValidateMRS_RejectsHighVersion(t *testing.T) {
	path := makeMRS(t, validMagic, 10000, 0, 100)

	_, err := ValidateMRS(path)
	if err == nil {
		t.Fatal("ожидалась ошибка для version=10000, получен nil")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("сообщение должно содержать 'version', получено: %q", err.Error())
	}
}

func TestValidateMRS_RejectsInvalidBehavior(t *testing.T) {
	path := makeMRS(t, validMagic, 1, 99, 100)

	_, err := ValidateMRS(path)
	if err == nil {
		t.Fatal("ожидалась ошибка для behavior=99, получен nil")
	}
	if !strings.Contains(err.Error(), "behavior") {
		t.Errorf("сообщение должно содержать 'behavior', получено: %q", err.Error())
	}
}

func TestValidateMRS_RejectsZeroCount(t *testing.T) {
	path := makeMRS(t, validMagic, 1, 0, 0)

	_, err := ValidateMRS(path)
	if err == nil {
		t.Fatal("ожидалась ошибка для count=0, получен nil")
	}
	if !strings.Contains(err.Error(), "count") {
		t.Errorf("сообщение должно содержать 'count', получено: %q", err.Error())
	}
}

func TestValidateMRS_RejectsHugeCount(t *testing.T) {
	path := makeMRS(t, validMagic, 1, 0, 200_000_000)

	_, err := ValidateMRS(path)
	if err == nil {
		t.Fatal("ожидалась ошибка для count=200_000_000, получен nil")
	}
	if !strings.Contains(err.Error(), "count") {
		t.Errorf("сообщение должно содержать 'count', получено: %q", err.Error())
	}
}

func TestValidateMRS_AcceptsAllBehaviors(t *testing.T) {
	behaviors := []struct {
		value uint32
		name  string
	}{
		{0, "domain"},
		{1, "ipcidr"},
		{2, "classical"},
	}

	for _, b := range behaviors {
		b := b
		t.Run(b.name, func(t *testing.T) {
			path := makeMRS(t, validMagic, 1, b.value, 50)
			hdr, err := ValidateMRS(path)
			if err != nil {
				t.Fatalf("behavior=%d (%s): ожидался nil, получено: %v", b.value, b.name, err)
			}
			if hdr.Behavior != b.value {
				t.Errorf("Behavior: хотели %d, получили %d", b.value, hdr.Behavior)
			}
		})
	}
}

func TestValidateMRS_RejectsNonExistentFile(t *testing.T) {
	_, err := ValidateMRS("/tmp/nonexistent_sign_craze_test.mrs")
	if err == nil {
		t.Fatal("ожидалась ошибка для несуществующего файла, получен nil")
	}
}

func TestValidateMRS_AcceptsMaxValidVersion(t *testing.T) {
	path := makeMRS(t, validMagic, mrsMaxVersion, 0, 1)
	_, err := ValidateMRS(path)
	if err != nil {
		t.Fatalf("version=%d должна быть допустима, получено: %v", mrsMaxVersion, err)
	}
}

func TestValidateMRS_AcceptsMaxValidCount(t *testing.T) {
	path := makeMRS(t, validMagic, 1, 0, mrsMaxCount)
	_, err := ValidateMRS(path)
	if err != nil {
		t.Fatalf("count=%d должен быть допустим, получено: %v", mrsMaxCount, err)
	}
}

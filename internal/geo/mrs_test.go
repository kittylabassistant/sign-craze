package geo

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// buildRawMRSHeader собирает "сырой" (нес-жатый) заголовок .mrs заданного
// размера mrsHeaderSize: magic+version+behavior+reserved(нули)+count(LE).
func buildRawMRSHeader(t *testing.T, version, behavior byte, count uint64) []byte {
	t.Helper()
	buf := make([]byte, mrsHeaderSize)
	copy(buf[0:3], mrsMagic)
	buf[3] = version
	buf[4] = behavior
	// buf[5:12] остаются нулями — совпадает с наблюдаемыми реальными образцами.
	binary.LittleEndian.PutUint64(buf[12:20], count)
	return buf
}

func TestValidateMRSHeader_ValidRawHeader_Domain(t *testing.T) {
	// Значения из реального образца geosite/youtube.mrs (behavior=domain).
	header := buildRawMRSHeader(t, 1, mrsBehaviorDomain, 178)
	if err := ValidateMRSHeader(bytes.NewReader(header), MaxMRSSize); err != nil {
		t.Fatalf("ValidateMRSHeader(валидный domain-заголовок) = %v, ожидался nil", err)
	}
}

func TestValidateMRSHeader_ValidRawHeader_IPCIDR(t *testing.T) {
	// Значения из реального образца geoip/ru.mrs (behavior=ipcidr).
	header := buildRawMRSHeader(t, 1, mrsBehaviorIPCIDR, 47457)
	if err := ValidateMRSHeader(bytes.NewReader(header), MaxMRSSize); err != nil {
		t.Fatalf("ValidateMRSHeader(валидный ipcidr-заголовок) = %v, ожидался nil", err)
	}
}

func TestValidateMRSHeader_ValidRawHeaderWithTrailingPayload(t *testing.T) {
	header := buildRawMRSHeader(t, 1, mrsBehaviorDomain, 28)
	// Добавляем "payload" succinct-trie — валидатор должен его пропустить
	// (skip), не пытаясь разобрать.
	payload := []byte("succinct-trie-payload-not-parsed")
	full := make([]byte, 0, len(header)+len(payload))
	full = append(full, header...)
	full = append(full, payload...)
	if err := ValidateMRSHeader(bytes.NewReader(full), MaxMRSSize); err != nil {
		t.Fatalf("ValidateMRSHeader(заголовок+payload) = %v, ожидался nil", err)
	}
}

func TestValidateMRSHeader_InvalidMagic(t *testing.T) {
	bad := []byte("XYZ1restofjunkdata......")
	if err := ValidateMRSHeader(bytes.NewReader(bad), MaxMRSSize); err == nil {
		t.Fatal("ожидалась ошибка для неверного magic")
	}
}

func TestValidateMRSHeader_UnknownVersion(t *testing.T) {
	header := buildRawMRSHeader(t, 99, mrsBehaviorDomain, 1)
	if err := ValidateMRSHeader(bytes.NewReader(header), MaxMRSSize); err == nil {
		t.Fatal("ожидалась ошибка для неизвестной версии")
	}
}

func TestValidateMRSHeader_UnknownBehavior(t *testing.T) {
	header := buildRawMRSHeader(t, 1, 99, 1)
	if err := ValidateMRSHeader(bytes.NewReader(header), MaxMRSSize); err == nil {
		t.Fatal("ожидалась ошибка для неизвестного behavior")
	}
}

func TestValidateMRSHeader_CountTooLarge(t *testing.T) {
	header := buildRawMRSHeader(t, 1, mrsBehaviorDomain, maxSaneMRSCount+1)
	if err := ValidateMRSHeader(bytes.NewReader(header), MaxMRSSize); err == nil {
		t.Fatal("ожидалась ошибка для неразумно большого count")
	}
}

func TestValidateMRSHeader_TooShort(t *testing.T) {
	if err := ValidateMRSHeader(bytes.NewReader([]byte{0x4d, 0x52}), MaxMRSSize); err == nil {
		t.Fatal("ожидалась ошибка для потока короче 4 байт")
	}
}

func TestValidateMRSHeader_HeaderTruncatedAfterMagic(t *testing.T) {
	// "MRS"+version, но без behavior/reserved/count.
	if err := ValidateMRSHeader(bytes.NewReader([]byte{'M', 'R', 'S', 1}), MaxMRSSize); err == nil {
		t.Fatal("ожидалась ошибка для заголовка, обрезанного после version")
	}
}

func TestValidateMRSHeader_Oversize(t *testing.T) {
	header := buildRawMRSHeader(t, 1, mrsBehaviorDomain, 1)
	padding := bytes.Repeat([]byte{0x00}, 100)
	full := make([]byte, 0, len(header)+len(padding))
	full = append(full, header...)
	full = append(full, padding...)
	if err := ValidateMRSHeader(bytes.NewReader(full), int64(len(header)+10)); err == nil {
		t.Fatal("ожидалась ошибка превышения лимита размера")
	}
}

func TestValidateMRSHeader_DefaultsMaxSizeWhenNonPositive(t *testing.T) {
	header := buildRawMRSHeader(t, 1, mrsBehaviorDomain, 1)
	if err := ValidateMRSHeader(bytes.NewReader(header), 0); err != nil {
		t.Errorf("ValidateMRSHeader(maxSize=0) = %v, ожидался nil (дефолт на MaxMRSSize)", err)
	}
}

// TestValidateMRSHeader_ZstdContainerAccepted — реальные .mrs, как их качает
// mihomo, целиком обёрнуты в zstd-фрейм (см. комментарий пакета). Без
// zstd-декодера (stdlib его не имеет, новые зависимости запрещены) заглянуть
// внутрь нельзя — валидатор обязан распознать контейнер и НЕ отвергать
// легитимный файл только из-за отсутствия видимого "MRS" на первых байтах.
func TestValidateMRSHeader_ZstdContainerAccepted(t *testing.T) {
	fake := append([]byte{0x28, 0xb5, 0x2f, 0xfd}, []byte("compressed-payload-not-inspected")...)
	if err := ValidateMRSHeader(bytes.NewReader(fake), MaxMRSSize); err != nil {
		t.Fatalf("ValidateMRSHeader(zstd-контейнер) = %v, ожидался nil", err)
	}
}

func TestValidateMRSHeader_ZstdContainerOversize(t *testing.T) {
	fake := append([]byte{0x28, 0xb5, 0x2f, 0xfd}, bytes.Repeat([]byte{0x00}, 100)...)
	if err := ValidateMRSHeader(bytes.NewReader(fake), 10); err == nil {
		t.Fatal("ожидалась ошибка превышения лимита размера для zstd-контейнера")
	}
}

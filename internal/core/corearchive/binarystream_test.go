package corearchive

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/elfcheck"
)

// TestNewBinaryStream_CloseInvokesCloser проверяет, что Close() вызывает
// переданную closer-функцию ровно один раз и пробрасывает её ошибку.
func TestNewBinaryStream_CloseInvokesCloser(t *testing.T) {
	calls := 0
	bs := NewBinaryStream(strings.NewReader("x"), func() error {
		calls++
		return nil
	})
	if err := bs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if calls != 1 {
		t.Errorf("closer вызван %d раз(а), ожидался 1", calls)
	}
}

// TestNewBinaryStream_ClosePropagatesError — ошибка closer'а должна дойти
// до вызывающего кода без изменений.
func TestNewBinaryStream_ClosePropagatesError(t *testing.T) {
	sentinel := errors.New("close failed")
	bs := NewBinaryStream(strings.NewReader("x"), func() error {
		return sentinel
	})
	if err := bs.Close(); !errors.Is(err, sentinel) {
		t.Errorf("Close() = %v, want %v", err, sentinel)
	}
}

// TestCheckELF_ValidMagicPassesThroughFullContent — успешная проверка не
// теряет прочитанные для magic-проверки байты: caller должен получить весь
// поток целиком (см. elfcheck.CheckAndRewind).
func TestCheckELF_ValidMagicPassesThroughFullContent(t *testing.T) {
	body := append(append([]byte{}, elfcheck.Magic...), []byte("rest-of-binary")...)

	r, err := CheckELF(bytes.NewReader(body), "test-label")
	if err != nil {
		t.Fatalf("CheckELF: %v", err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("содержимое отличается:\ngot:  %x\nwant: %x", got, body)
	}
}

// TestCheckELF_RejectsNonELF — содержимое без ELF-magic отвергается, ошибка
// содержит и метку "не ELF" (проверяется вызывающим кодом xray/mihomo), и
// переданный label (диагностика).
func TestCheckELF_RejectsNonELF(t *testing.T) {
	_, err := CheckELF(bytes.NewReader([]byte("MZ-not-elf-content")), "test-label")
	if err == nil {
		t.Fatal("ожидалась ошибка для не-ELF содержимого")
	}
	if !strings.Contains(err.Error(), "не ELF") {
		t.Errorf("ошибка должна упоминать 'не ELF': %v", err)
	}
	if !strings.Contains(err.Error(), "test-label") {
		t.Errorf("ошибка должна содержать label %q: %v", "test-label", err)
	}
}

// TestCheckELF_ShortReadRejected — содержимое короче 4 байт (усечённый файл)
// не проходит magic-проверку и не паникует.
func TestCheckELF_ShortReadRejected(t *testing.T) {
	_, err := CheckELF(bytes.NewReader([]byte{0x7f, 'E'}), "short")
	if err == nil {
		t.Fatal("ожидалась ошибка для содержимого короче ELF-magic")
	}
}

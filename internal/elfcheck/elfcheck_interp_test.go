package elfcheck

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// elfIdent64 формирует e_ident для ELF64 little-endian.
func elfIdent64() [16]byte {
	var id [16]byte
	copy(id[:], []byte{
		0x7f, 'E', 'L', 'F',
		byte(elf.ELFCLASS64),
		byte(elf.ELFDATA2LSB),
		byte(elf.EV_CURRENT),
		0, // OSABI = System V
	})
	return id
}

// makeMinimalStaticELF64 — валидный ELF64 LE без program headers (статический).
func makeMinimalStaticELF64() []byte {
	hdr := elf.Header64{
		Ident:     elfIdent64(),
		Type:      uint16(elf.ET_EXEC),
		Machine:   uint16(elf.EM_AARCH64),
		Version:   uint32(elf.EV_CURRENT),
		Ehsize:    64,
		Phentsize: 56,
		Phnum:     0,
		Shentsize: 64,
		Shnum:     0,
	}
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, hdr)
	return buf.Bytes()
}

// makeMinimalDynamicELF64 — валидный ELF64 LE с одним PT_INTERP, указывающим на
// interpPath. Layout: ELF header (64) + Prog64 (56) + interp-строка (null-term).
func makeMinimalDynamicELF64(interpPath string) []byte {
	interp := append([]byte(interpPath), 0)
	const phoff = 64
	const interpOff = phoff + 56 // 120

	hdr := elf.Header64{
		Ident:     elfIdent64(),
		Type:      uint16(elf.ET_DYN),
		Machine:   uint16(elf.EM_AARCH64),
		Version:   uint32(elf.EV_CURRENT),
		Phoff:     phoff,
		Ehsize:    64,
		Phentsize: 56,
		Phnum:     1,
		Shentsize: 64,
		Shnum:     0,
	}
	ph := elf.Prog64{
		Type:   uint32(elf.PT_INTERP),
		Flags:  uint32(elf.PF_R),
		Off:    interpOff,
		Filesz: uint64(len(interp)),
		Memsz:  uint64(len(interp)),
		Align:  1,
	}
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, hdr)
	_ = binary.Write(&buf, binary.LittleEndian, ph)
	buf.Write(interp)
	return buf.Bytes()
}

func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bin")
	if err := os.WriteFile(p, data, 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return p
}

// Статический бинарь (нет PT_INTERP) → совместим всегда.
func TestCheckInterpCompatibility_Static(t *testing.T) {
	if err := CheckInterpCompatibility(writeTemp(t, makeMinimalStaticELF64())); err != nil {
		t.Errorf("статический ELF: ожидался nil, получено %v", err)
	}
}

// Динамический бинарь, интерпретатор существует на FS → совместим.
func TestCheckInterpCompatibility_DynamicInterpreterExists(t *testing.T) {
	dir := t.TempDir()
	interp := filepath.Join(dir, "ld-fake.so")
	if err := os.WriteFile(interp, []byte{0}, 0o755); err != nil {
		t.Fatalf("WriteFile interp: %v", err)
	}
	p := filepath.Join(dir, "dyn")
	if err := os.WriteFile(p, makeMinimalDynamicELF64(interp), 0o755); err != nil {
		t.Fatalf("WriteFile bin: %v", err)
	}
	if err := CheckInterpCompatibility(p); err != nil {
		t.Errorf("динамический ELF с существующим интерпретатором: ожидался nil, получено %v", err)
	}
}

// Динамический бинарь, интерпретатор отсутствует → понятная ошибка (issue #3).
func TestCheckInterpCompatibility_DynamicInterpreterMissing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent-ld.so") // гарантированно не существует
	p := writeTemp(t, makeMinimalDynamicELF64(missing))
	err := CheckInterpCompatibility(p)
	if err == nil {
		t.Fatal("ожидалась ошибка для отсутствующего интерпретатора")
	}
	if !strings.Contains(err.Error(), "динамически слинкован") {
		t.Errorf("сообщение = %q, ожидалось содержащее 'динамически слинкован'", err.Error())
	}
}

// Неполный ELF-стаб (magic + мусор) не парсится elf.Open → best-effort пропуск (nil).
func TestCheckInterpCompatibility_NotELF(t *testing.T) {
	p := writeTemp(t, append([]byte{0x7f, 'E', 'L', 'F'}, []byte("fake-binary")...))
	if err := CheckInterpCompatibility(p); err != nil {
		t.Errorf("неполный ELF-стаб: ожидался nil (best-effort), получено %v", err)
	}
}

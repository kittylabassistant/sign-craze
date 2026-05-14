package naiveproxy

import (
	"context"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestDownload_MIPSBigEndian_Rejected(t *testing.T) {
	_, err := Download(context.Background(), types.ArchMIPS, t.TempDir())
	if err == nil {
		t.Fatal("ожидалась ошибка про неподдержку BE MIPS")
	}
	// Проверяем, что упало ДО сетевых запросов — ошибка содержит ключевую фразу.
	if got := err.Error(); !contains(got, "big-endian") && !contains(got, "mips") {
		t.Errorf("ошибка %q не упоминает mips/big-endian", got)
	}
}

func TestDownload_PatternMapping(t *testing.T) {
	expect := map[types.Arch]string{
		types.ArchARM64:  "linux-arm64.tar.xz",
		types.ArchARM7:   "linux-arm.tar.xz",
		types.ArchMIPSLE: "linux-mipsel.tar.xz",
		types.ArchAMD64:  "linux-x64.tar.xz",
	}
	for arch, want := range expect {
		got, ok := naiveAssetPattern[arch]
		if !ok {
			t.Errorf("arch=%s: паттерн отсутствует", arch)
			continue
		}
		if got != want {
			t.Errorf("arch=%s: pattern=%q, want %q", arch, got, want)
		}
	}
	if _, ok := naiveAssetPattern[types.ArchMIPS]; ok {
		t.Error("naiveAssetPattern содержит ArchMIPS — должен быть исключён")
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

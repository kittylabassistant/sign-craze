package mihomo

import (
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestMatchAsset_Linux_перебирает_фактический_список_assets — фильтр
// должен принять default-вариант для каждой архитектуры и отвергнуть
// все микроарх/go-version/format-варианты. Список взят из реального
// `gh api repos/MetaCubeX/mihomo/releases/latest`.
func TestMatchAsset_перебирает_фактический_список_assets(t *testing.T) {
	all := []string{
		"mihomo-linux-amd64-v1-go120-v1.19.24.gz",
		"mihomo-linux-amd64-v1-go123-v1.19.24.gz",
		"mihomo-linux-amd64-v1-v1.19.24.gz",
		"mihomo-linux-amd64-v1.19.24.gz", // ← arm-default amd64
		"mihomo-linux-amd64-v2-v1.19.24.gz",
		"mihomo-linux-amd64-v3-v1.19.24.gz",
		"mihomo-linux-amd64-compatible-v1.19.24.gz",
		"mihomo-linux-amd64-v1-v1.19.24.deb",
		"mihomo-linux-amd64-v1.19.24.pkg.tar.zst",
		"mihomo-linux-arm64-v1.19.24.gz", // ← arm64-default
		"mihomo-linux-arm64-go120-v1.19.24.gz",
		"mihomo-linux-arm64-go124-v1.19.24.gz",
		"mihomo-linux-armv5-v1.19.24.gz",
		"mihomo-linux-armv6-v1.19.24.gz",
		"mihomo-linux-armv7-v1.19.24.gz",          // ← arm7
		"mihomo-linux-mips-softfloat-v1.19.24.gz", // ← mips
		"mihomo-linux-mips-hardfloat-v1.19.24.gz",
		"mihomo-linux-mipsle-softfloat-v1.19.24.gz", // ← mipsle
		"mihomo-linux-mipsle-hardfloat-v1.19.24.gz",
		"mihomo-linux-mips64-v1.19.24.gz",
	}

	expect := map[types.Arch]string{
		types.ArchAMD64:  "mihomo-linux-amd64-v1.19.24.gz",
		types.ArchARM64:  "mihomo-linux-arm64-v1.19.24.gz",
		types.ArchARM7:   "mihomo-linux-armv7-v1.19.24.gz",
		types.ArchMIPSLE: "mihomo-linux-mipsle-softfloat-v1.19.24.gz",
		types.ArchMIPS:   "mihomo-linux-mips-softfloat-v1.19.24.gz",
	}

	for arch, want := range expect {
		t.Run(string(arch), func(t *testing.T) {
			fn := matchAsset(arch)
			var matches []string
			for _, name := range all {
				if fn(types.Asset{Name: name}) {
					matches = append(matches, name)
				}
			}
			if len(matches) != 1 {
				t.Fatalf("ожидалось ровно 1 совпадение, получено %d: %v", len(matches), matches)
			}
			if matches[0] != want {
				t.Errorf("matches[0] = %q, ожидалось %q", matches[0], want)
			}
		})
	}
}

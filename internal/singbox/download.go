package singbox

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// assetMatchers — упорядоченные matcher'ы release-ассета sing-box по архитектуре.
// Первый совпавший выигрывает: предпочитается СТАТИЧЕСКИЙ вариант (musl), базовый —
// fallback.
//
// Почему musl первым: SagerNet начиная с ~v1.13.x публикует для arm64 и amd64
// базовый ассет (linux-arm64.tar.gz) с CGO/libcronet.so — динамически слинкован
// с glibc (интерпретатор /lib/ld-linux-aarch64.so.1). Entware/Keenetic работает
// на musl без этого интерпретатора → exec падает с ENOENT ("fork/exec: no such
// file or directory"). Вариант *-musl.tar.gz статически слинкован и совместим
// с любым окружением (см. issue #3).
//
// Почему base как fallback:
//   - mips (big-endian): SagerNet НЕ публикует musl-вариант; базовый исторически
//     статичен — единственный matcher.
//   - Устойчивость: если upstream переименует/уберёт musl-ассет, download выживет
//     через base, а elfcheck.CheckInterpCompatibility затем выдаст понятную ошибку.
//
// Коллизий подстрок нет: "linux-arm64.tar.gz" НЕ входит в "linux-arm64-musl.tar.gz"
// (после arm64 идёт "-musl"); "linux-mips-softfloat.tar.gz" НЕ входит в
// "linux-mipsle-softfloat.tar.gz".
var assetMatchers = map[types.Arch][]func(types.Asset) bool{
	types.ArchARM64: {
		ghrelease.MatchByContains("linux-arm64-musl.tar.gz"),
		ghrelease.MatchByContains("linux-arm64.tar.gz"),
	},
	types.ArchARM7:   {ghrelease.MatchByContains("linux-armv7.tar.gz")},
	types.ArchMIPSLE: {ghrelease.MatchByContains("linux-mipsle-softfloat.tar.gz")},
	types.ArchMIPS:   {ghrelease.MatchByContains("linux-mips-softfloat.tar.gz")},
	types.ArchAMD64: {
		ghrelease.MatchByContains("linux-amd64-musl.tar.gz"),
		ghrelease.MatchByContains("linux-amd64.tar.gz"),
	},
}

// Download скачивает актуальный tarball sing-box для указанной архитектуры в директорию dst.
// При совпадении ETag повторная загрузка не производится.
func Download(ctx context.Context, arch types.Arch, dstDir string) (core.DownloadResult, error) {
	if err := arch.Validate(); err != nil {
		return core.DownloadResult{}, err
	}

	matchers, ok := assetMatchers[arch]
	if !ok {
		return core.DownloadResult{}, fmt.Errorf("singbox download: нет матчеров для архитектуры %q", arch)
	}

	res, err := ghrelease.New().Fetch(ctx, ghrelease.FetchOptions{
		Owner:         "SagerNet",
		Repo:          "sing-box",
		AssetMatchers: matchers,
		DstDir:        dstDir,
		// SagerNet sing-box историчесĸи НЕ публикует <asset>.sha256 в релизах
		// (проверено на v1.13.11 — gh api repos/SagerNet/sing-box/releases/tags/...
		// не содержит .sha256 / SHA256SUMS / signature файлов).
		// AllowMissingSHA=true: integrity-проверка пропускается с WARN.
		// Защита от truncated download остаётся через manifest Size check
		// в ghrelease.downloadFromMirror (resp.Size != asset.Size → ошибка).
		// От targeted MITM не защищает — но sing-box качается через GitHub
		// HTTPS, для RCE-attack нужно компрометировать SagerNet/GitHub TLS.
		VerifySHA:       true,
		AllowMissingSHA: true,
	})
	if err != nil {
		return core.DownloadResult{}, fmt.Errorf("singbox download: %w", err)
	}

	return core.DownloadResult{
		Downloaded: res.Downloaded,
		Version:    res.Version,
		Path:       res.Path,
	}, nil
}

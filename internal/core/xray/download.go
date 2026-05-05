package xray

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// XRayMIPSVersion — pinned тег sign-craze custom-build xray для mips/mipsle.
//
// Upstream XTLS/Xray-core собирается с Go 1.23+, runtime которого использует
// futex-операции, отсутствующие в старых ядрах Keenetic (3.4–4.x). Падение
// `runtime.futexwakeup ... returned -89` (ENOSYS) делает официальные
// MIPS-бинари непригодными.
//
// Workflow `.github/workflows/build-xray-mips.yml` собирает xray из
// XTLS/Xray-core с принудительным toolchain Go 1.24 + GOMIPS=softfloat и
// публикует результат в release с tag-ом ниже. Значение синхронизируется
// вручную при выкатке нового xray.
//
// Pinned tag — v25.12.8: последний stable перед xray v26.x bump'нул
// gvisor до версии, требующей Go 1.25.5+. v25.12.8 даёт всё ключевое:
// PQ-VLESS (mlkem768x25519plus), VLESS Vision UDP443, XHTTP modes
// stream-up/stream-one/packet-up — и собирается с Go 1.24.x, что
// подтверждено совместимым sing-box (v1.13.11 → Go 1.24.7) на Keenetic.
const XRayMIPSVersion = "v25.12.8"

// XRayMIPSReleaseRepo — репозиторий, в котором лежат custom MIPS-сборки xray.
const (
	XRayMIPSReleaseOwner = "kittylabassistant"
	XRayMIPSReleaseRepo  = "sign-craze-xray"
)

// assetPattern — подстрока в имени asset на GitHub releases XTLS/Xray-core,
// идентифицирующая нужную архитектуру.
//
// Соответствие проверено на релизах XTLS/Xray-core (см. `gh api
// repos/XTLS/Xray-core/releases/latest`):
//
//	arm64  → Xray-linux-arm64-v8a.zip
//	arm7   → Xray-linux-arm32-v7a.zip
//	amd64  → Xray-linux-64.zip
//
// MIPS архитектуры ARE намеренно отсутствуют: для них качаются собственные
// сборки (см. mipsAssetPattern + XRayMIPSVersion).
var assetPattern = map[types.Arch]string{
	types.ArchARM64: "linux-arm64-v8a.zip",
	types.ArchARM7:  "linux-arm32-v7a.zip",
	types.ArchAMD64: "linux-64.zip",
}

// mipsAssetPattern — подстрока для MIPS asset'ов в собственном release
// `xray-mips-<XRayMIPSVersion>`. Совпадает с upstream именованием,
// чтобы install.go (zip-распаковщик) работал без изменений.
var mipsAssetPattern = map[types.Arch]string{
	types.ArchMIPSLE: "linux-mips32le.zip",
	types.ArchMIPS:   "linux-mips32.zip",
}

// DownloadResult описывает результат вызова Download.
type DownloadResult struct {
	Downloaded bool   // false если файл актуален (ETag совпал)
	Version    string // тег релиза, например "v1.8.4"
	Path       string // путь к скачанному zip-файлу
}

// Download скачивает актуальный zip-архив xray для указанной архитектуры.
// При совпадении ETag повторная загрузка не производится.
//
// Источник:
//   - arm64/arm7/amd64 — последний релиз XTLS/Xray-core (upstream).
//   - mips/mipsle      — pinned-tag в kittylabassistant/sign-craze (Go 1.22-сборка
//     для совместимости с ядрами Keenetic).
//
// Возвращает путь к zip — распаковка делается отдельно через Install
// (см. install.go).
func Download(ctx context.Context, arch types.Arch, dstDir string) (DownloadResult, error) {
	if err := arch.Validate(); err != nil {
		return DownloadResult{}, err
	}

	if pattern, ok := mipsAssetPattern[arch]; ok {
		res, err := ghrelease.New().Fetch(ctx, ghrelease.FetchOptions{
			Owner:      XRayMIPSReleaseOwner,
			Repo:       XRayMIPSReleaseRepo,
			Tag:        "xray-mips-" + XRayMIPSVersion,
			AssetMatch: ghrelease.MatchByContains(pattern),
			DstDir:     dstDir,
			VerifySHA:  true,
		})
		if err != nil {
			return DownloadResult{}, fmt.Errorf("xray download (mips, custom build): %w", err)
		}
		return DownloadResult{
			Downloaded: res.Downloaded,
			Version:    res.Version,
			Path:       res.Path,
		}, nil
	}

	pattern, ok := assetPattern[arch]
	if !ok {
		return DownloadResult{}, fmt.Errorf("xray download: нет паттерна для архитектуры %q", arch)
	}

	res, err := ghrelease.New().Fetch(ctx, ghrelease.FetchOptions{
		Owner:      "XTLS",
		Repo:       "Xray-core",
		AssetMatch: ghrelease.MatchByContains(pattern),
		DstDir:     dstDir,
	})
	if err != nil {
		return DownloadResult{}, fmt.Errorf("xray download: %w", err)
	}

	return DownloadResult{
		Downloaded: res.Downloaded,
		Version:    res.Version,
		Path:       res.Path,
	}, nil
}

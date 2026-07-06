package cli

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/core"
)

// installGeoDATAssets не возвращает ошибку (по контракту — выполнение
// с Warn при неудаче, установка ядра не должна фейлиться), поэтому тесты
// проверяют побочный эффект (вызвана ли geoDownloadDATFn) и отсутствие паники.

func TestInstallGeoDATAssets_SkipsNonDATCores(t *testing.T) {
	for _, name := range []string{"sing-box", "mihomo"} {
		t.Run(name, func(t *testing.T) {
			called := false
			orig := geoDownloadDATFn
			geoDownloadDATFn = func(ctx context.Context, cacheDir, dstDir string) (int, error) {
				called = true
				return 0, nil
			}
			defer func() { geoDownloadDATFn = orig }()

			installGeoDATAssets(context.Background(), core.MustGet(name))
			if called {
				t.Errorf("%s: GeoFormat()!=GeoDAT, geo.DownloadDAT не должен вызываться", name)
			}
		})
	}
}

func TestInstallGeoDATAssets_CallsForXrayWithCorrectPaths(t *testing.T) {
	var gotCacheDir, gotDstDir string
	called := false
	orig := geoDownloadDATFn
	geoDownloadDATFn = func(ctx context.Context, cacheDir, dstDir string) (int, error) {
		called = true
		gotCacheDir, gotDstDir = cacheDir, dstDir
		return 2, nil
	}
	defer func() { geoDownloadDATFn = orig }()

	c := core.MustGet("xray")
	installGeoDATAssets(context.Background(), c)

	if !called {
		t.Fatal("xray (GeoDAT): geo.DownloadDAT должен вызываться")
	}
	if gotCacheDir != c.CacheDir() {
		t.Errorf("cacheDir = %q, ожидался %q", gotCacheDir, c.CacheDir())
	}
	wantDst := filepath.Join(c.ConfigDir(), "assets")
	if gotDstDir != wantDst {
		t.Errorf("dstDir = %q, ожидался %q", gotDstDir, wantDst)
	}
}

// TestInstallGeoDATAssets_ErrorDoesNotPanicOrPropagate — ошибка download
// поглощается (Warn-лог), не паникует и не имеет возвращаемого значения для
// проверки — сам факт успешного возврата уже подтверждает контракт
// "не фейлит установку".
func TestInstallGeoDATAssets_ErrorDoesNotPanicOrPropagate(t *testing.T) {
	orig := geoDownloadDATFn
	geoDownloadDATFn = func(ctx context.Context, cacheDir, dstDir string) (int, error) {
		return 0, errors.New("сеть недоступна")
	}
	defer func() { geoDownloadDATFn = orig }()

	installGeoDATAssets(context.Background(), core.MustGet("xray"))
}

package cli

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/core"
)

// Ядра sing-box/xray/mihomo зарегистрированы через blank-импорты cores.go —
// доступны в любом тесте пакета cli без ручной регистрации фейков.

func TestCoreFromArgsOrActive_Default(t *testing.T) {
	c, err := coreFromArgsOrActive(nil)
	if err != nil {
		t.Fatalf("coreFromArgsOrActive(nil): %v", err)
	}
	if c.Name() != "sing-box" {
		t.Errorf("Name() = %q, ожидалось sing-box (нет state.json → DefaultCore fallback)", c.Name())
	}
}

func TestCoreFromArgsOrActive_ExplicitCore(t *testing.T) {
	c, err := coreFromArgsOrActive([]string{"--core", "xray"})
	if err != nil {
		t.Fatalf("coreFromArgsOrActive: %v", err)
	}
	if c.Name() != "xray" {
		t.Errorf("Name() = %q, ожидалось xray", c.Name())
	}
}

func TestCoreFromArgsOrActive_ExplicitCore_NotFirstArg(t *testing.T) {
	// Флаг --core должен находиться независимо от позиции в args.
	c, err := coreFromArgsOrActive([]string{"--some-other-flag", "--core", "mihomo"})
	if err != nil {
		t.Fatalf("coreFromArgsOrActive: %v", err)
	}
	if c.Name() != "mihomo" {
		t.Errorf("Name() = %q, ожидалось mihomo", c.Name())
	}
}

func TestCoreFromArgsOrActive_UnknownCore(t *testing.T) {
	_, err := coreFromArgsOrActive([]string{"--core", "bogus-core"})
	if err == nil {
		t.Fatal("ожидалась ошибка для незарегистрированного ядра")
	}
}

func TestCoreFromArgsOrActive_MissingValue(t *testing.T) {
	_, err := coreFromArgsOrActive([]string{"--core"})
	if err == nil {
		t.Fatal("ожидалась ошибка при отсутствии значения после --core")
	}
}

func TestUpdateGeoForCore_MihomoNoOp(t *testing.T) {
	called := false
	orig := geoDownloadDATFn
	geoDownloadDATFn = func(ctx context.Context, cacheDir, dstDir string) (int, error) {
		called = true
		return 0, nil
	}
	defer func() { geoDownloadDATFn = orig }()

	c := core.MustGet("mihomo")
	if err := updateGeoForCore(context.Background(), c); err != nil {
		t.Fatalf("updateGeoForCore(mihomo) = %v, ожидался nil", err)
	}
	if called {
		t.Error("mihomo (GeoMRS): geo.DownloadDAT не должен вызываться")
	}
}

func TestUpdateGeoForCore_XrayCallsDownloadDATWithCorrectPaths(t *testing.T) {
	var gotCacheDir, gotDstDir string
	orig := geoDownloadDATFn
	geoDownloadDATFn = func(ctx context.Context, cacheDir, dstDir string) (int, error) {
		gotCacheDir, gotDstDir = cacheDir, dstDir
		return 2, nil
	}
	defer func() { geoDownloadDATFn = orig }()

	c := core.MustGet("xray")
	if err := updateGeoForCore(context.Background(), c); err != nil {
		t.Fatalf("updateGeoForCore(xray) = %v, ожидался nil", err)
	}
	if gotCacheDir != c.CacheDir() {
		t.Errorf("cacheDir = %q, ожидался %q", gotCacheDir, c.CacheDir())
	}
	wantDst := filepath.Join(c.ConfigDir(), "assets")
	if gotDstDir != wantDst {
		t.Errorf("dstDir = %q, ожидался %q (должен совпадать с GeoAssetsDir из xray/coreadapter.go)", gotDstDir, wantDst)
	}
}

func TestUpdateGeoForCore_XrayPropagatesDownloadError(t *testing.T) {
	wantErr := errors.New("сеть недоступна")
	orig := geoDownloadDATFn
	geoDownloadDATFn = func(ctx context.Context, cacheDir, dstDir string) (int, error) {
		return 0, wantErr
	}
	defer func() { geoDownloadDATFn = orig }()

	err := updateGeoForCore(context.Background(), core.MustGet("xray"))
	if !errors.Is(err, wantErr) {
		t.Fatalf("updateGeoForCore(xray) с ошибкой download = %v, ожидалась обёртка над %v", err, wantErr)
	}
}

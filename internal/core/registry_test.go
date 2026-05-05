package core

import (
	"context"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// fakeCore — минимальная реализация Core для тестов registry.
type fakeCore struct{ name string }

func (f *fakeCore) Name() string                                                     { return f.name }
func (f *fakeCore) BinaryPath() string                                               { return "/tmp/" + f.name }
func (f *fakeCore) ConfigDir() string                                                { return "/tmp/" + f.name + "-cfg" }
func (f *fakeCore) CacheDir() string                                                 { return "/tmp/" + f.name + "-cache" }
func (f *fakeCore) ConfigPath() string                                               { return "/tmp/" + f.name + "/config" }
func (f *fakeCore) PIDPath() string                                                  { return "/tmp/" + f.name + ".pid" }
func (f *fakeCore) ConfigFormat() ConfigFormat                                       { return ConfigJSON }
func (f *fakeCore) GeoFormat() GeoFormat                                             { return GeoSRS }
func (f *fakeCore) NewLifecycle() service.Lifecycle                                  { return nil }
func (f *fakeCore) CheckConfig(ctx context.Context, r exectx.Runner, p string) error { return nil }
func (f *fakeCore) Download(ctx context.Context, a types.Arch, dst string) (DownloadResult, error) {
	return DownloadResult{Path: "/tmp/" + f.name + ".bin"}, nil
}
func (f *fakeCore) Install(ctx context.Context, r exectx.Runner, p string) error { return nil }
func (f *fakeCore) BinaryVersion(ctx context.Context, r exectx.Runner) (string, error) {
	return "0.0.0", nil
}
func (f *fakeCore) ParseVersion(line string) (string, error) { return line, nil }
func (f *fakeCore) RenderConfig(_ types.CoreRenderParams) ([]byte, error) {
	return []byte("fake"), nil
}

func TestRegisterAndGet(t *testing.T) {
	resetForTest()
	defer resetForTest()

	Register(&fakeCore{name: "alpha"})
	Register(&fakeCore{name: "beta"})

	c, err := Get("alpha")
	if err != nil {
		t.Fatalf("Get(alpha): %v", err)
	}
	if c.Name() != "alpha" {
		t.Errorf("Name() = %q, ожидалось alpha", c.Name())
	}

	got := Names()
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("Names() = %v, ожидалось [alpha beta]", got)
	}
}

// Get с пустым именем должен возвращать sing-box (default для legacy state.json).
func TestGet_EmptyNameDefaultsToSingBox(t *testing.T) {
	resetForTest()
	defer resetForTest()

	Register(&fakeCore{name: "sing-box"})
	c, err := Get("")
	if err != nil {
		t.Fatalf("Get(\"\"): %v", err)
	}
	if c.Name() != "sing-box" {
		t.Errorf("Name() = %q, ожидалось sing-box", c.Name())
	}
}

func TestGet_UnknownReturnsError(t *testing.T) {
	resetForTest()
	defer resetForTest()

	Register(&fakeCore{name: "sing-box"})
	_, err := Get("xray")
	if err == nil {
		t.Fatal("ожидалась ошибка для незарегистрированного xray")
	}
	if !strings.Contains(err.Error(), "не зарегистрировано") {
		t.Errorf("сообщение об ошибке должно описывать причину: %v", err)
	}
}

func TestRegister_PanicsOnDuplicate(t *testing.T) {
	resetForTest()
	defer resetForTest()

	Register(&fakeCore{name: "dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("ожидалась паника при дублирующей регистрации")
		}
	}()
	Register(&fakeCore{name: "dup"})
}

func TestRegister_PanicsOnNilOrEmpty(t *testing.T) {
	resetForTest()
	defer resetForTest()

	t.Run("nil", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("ожидалась паника при nil Core")
			}
		}()
		Register(nil)
	})

	t.Run("empty name", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("ожидалась паника при пустом имени")
			}
		}()
		Register(&fakeCore{name: ""})
	})
}

package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestLoad_FileMissingReturnsDefault(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Mode != types.ModeProxy {
		t.Errorf("Mode = %q, ожидалось proxy", s.Mode)
	}
	if len(s.Outbounds) != 1 || s.Outbounds[0].Type != "direct" {
		t.Errorf("ожидался stub direct outbound, получено %+v", s.Outbounds)
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := &State{
		Mode: types.ModeHybrid,
		Outbounds: []types.Outbound{
			{Tag: "proxy", Type: "socks", Server: "1.2.3.4", Port: 1080},
		},
		Ports:       []uint16{80, 443},
		Excludes:    []string{"192.168.1.0/24"},
		DPIEnabled:  true,
		DPIStrategy: "preset:default",
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Mode != want.Mode {
		t.Errorf("Mode = %q, ожидалось %q", got.Mode, want.Mode)
	}
	if len(got.Ports) != 2 || got.Ports[0] != 80 {
		t.Errorf("Ports = %v, ожидалось [80 443]", got.Ports)
	}
	if !got.DPIEnabled {
		t.Error("DPIEnabled должен быть true")
	}
}

func TestSave_Permissions0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Save(path, Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("права %o, ожидалось 0600 (содержит креды outbound)", mode)
	}
}

func TestSave_NilStateReturnsError(t *testing.T) {
	err := Save(filepath.Join(t.TempDir(), "x.json"), nil)
	if err == nil {
		t.Fatal("ожидалась ошибка при nil State")
	}
}

func TestLoad_NormalisesNilSlices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"mode":"proxy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Outbounds == nil || s.Ports == nil || s.Excludes == nil {
		t.Errorf("ожидались инициализированные пустые слайсы, получено %+v", s)
	}
}

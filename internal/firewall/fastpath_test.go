package firewall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDisableFastPath_SavesAndDisables(t *testing.T) {
	dir := t.TempDir()
	natPath := filepath.Join(dir, "fastnat")
	routePath := filepath.Join(dir, "fastroute")
	statePath := filepath.Join(dir, "subdir", "state.json")
	if err := os.WriteFile(natPath, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(routePath, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := disableFastPathAt(natPath, routePath, statePath); err != nil {
		t.Fatalf("disable: %v", err)
	}

	if v, _ := readSysctl(natPath); v != "0" {
		t.Errorf("nat sysctl: got %q, want 0", v)
	}
	if v, _ := readSysctl(routePath); v != "0" {
		t.Errorf("route sysctl: got %q, want 0", v)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("state file: %v", err)
	}
	var st fastPathState
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if st.FastNAT != "1" || st.FastRoute != "1" {
		t.Errorf("state mismatch: %+v", st)
	}
}

func TestDisableFastPath_PreservesOriginalOnRepeatedCall(t *testing.T) {
	dir := t.TempDir()
	natPath := filepath.Join(dir, "fastnat")
	routePath := filepath.Join(dir, "fastroute")
	statePath := filepath.Join(dir, "state.json")
	_ = os.WriteFile(natPath, []byte("1"), 0o644)
	_ = os.WriteFile(routePath, []byte("1"), 0o644)

	if err := disableFastPathAt(natPath, routePath, statePath); err != nil {
		t.Fatal(err)
	}
	if err := disableFastPathAt(natPath, routePath, statePath); err != nil {
		t.Fatalf("повторный disable: %v", err)
	}

	data, _ := os.ReadFile(statePath)
	var st fastPathState
	_ = json.Unmarshal(data, &st)
	if st.FastNAT != "1" || st.FastRoute != "1" {
		t.Errorf("после повторного disable state затёрт нулями: %+v", st)
	}
}

func TestDisableFastPath_NoopOnMissingSysctl(t *testing.T) {
	dir := t.TempDir()
	natPath := filepath.Join(dir, "missing-fastnat")
	routePath := filepath.Join(dir, "missing-fastroute")
	statePath := filepath.Join(dir, "state.json")

	if err := disableFastPathAt(natPath, routePath, statePath); err != nil {
		t.Fatalf("ожидался no-op, получено: %v", err)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("state-файл не должен создаваться на не-Keenetic системе")
	}
}

func TestRestoreFastPath_RestoresAndCleansState(t *testing.T) {
	dir := t.TempDir()
	natPath := filepath.Join(dir, "fastnat")
	routePath := filepath.Join(dir, "fastroute")
	statePath := filepath.Join(dir, "state.json")
	_ = os.WriteFile(natPath, []byte("0"), 0o644)
	_ = os.WriteFile(routePath, []byte("0"), 0o644)
	st := fastPathState{FastNAT: "1", FastRoute: "1"}
	data, _ := json.Marshal(st)
	_ = os.WriteFile(statePath, data, 0o644)

	if err := restoreFastPathAt(natPath, routePath, statePath); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if v, _ := readSysctl(natPath); v != "1" {
		t.Errorf("nat sysctl: got %q, want 1", v)
	}
	if v, _ := readSysctl(routePath); v != "1" {
		t.Errorf("route sysctl: got %q, want 1", v)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("state-файл должен быть удалён после restore")
	}
}

func TestRestoreFastPath_NoopWithoutStateFile(t *testing.T) {
	dir := t.TempDir()
	natPath := filepath.Join(dir, "fastnat")
	routePath := filepath.Join(dir, "fastroute")
	statePath := filepath.Join(dir, "state.json")
	_ = os.WriteFile(natPath, []byte("1"), 0o644)
	_ = os.WriteFile(routePath, []byte("1"), 0o644)

	if err := restoreFastPathAt(natPath, routePath, statePath); err != nil {
		t.Fatalf("ожидался no-op без state-файла: %v", err)
	}
	if v, _ := readSysctl(natPath); v != "1" {
		t.Errorf("nat sysctl изменён: %q", v)
	}
}

func TestWriteSysctl_RejectsNonNumeric(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x")
	_ = os.WriteFile(path, []byte("0"), 0o644)
	if err := writeSysctl(path, "bad"); err == nil {
		t.Error("ожидалась ошибка на не-числовое значение")
	}
}

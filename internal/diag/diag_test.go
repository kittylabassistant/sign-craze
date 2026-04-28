package diag

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRun_AnyFail(t *testing.T) {
	checks := []Check{
		func(_ context.Context) Result { return Result{Name: "ok", Status: PASS} },
		func(_ context.Context) Result { return Result{Name: "fail", Status: FAIL} },
	}
	results := Run(context.Background(), checks)
	if len(results) != 2 {
		t.Fatalf("len = %d", len(results))
	}
	if !AnyFail(results) {
		t.Error("AnyFail должен быть true")
	}
}

func TestRun_PanicCaught(t *testing.T) {
	checks := []Check{
		func(_ context.Context) Result { panic("boom") },
	}
	results := Run(context.Background(), checks)
	if len(results) != 1 || results[0].Status != FAIL {
		t.Errorf("ожидался FAIL после panic, получено %+v", results)
	}
}

func TestCheckBinaryExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "bin")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := checkBinaryExecutable("test", exe)(context.Background())
	if r.Status != PASS {
		t.Errorf("Status = %s, ожидался PASS", r.Status)
	}

	r = checkBinaryExecutable("test", filepath.Join(dir, "missing"))(context.Background())
	if r.Status != FAIL {
		t.Errorf("Status = %s для отсутствующего бинаря, ожидался FAIL", r.Status)
	}

	notExe := filepath.Join(dir, "notexe")
	_ = os.WriteFile(notExe, []byte("data"), 0o644)
	r = checkBinaryExecutable("test", notExe)(context.Background())
	if r.Status != FAIL {
		t.Errorf("Status = %s для не-исполняемого, ожидался FAIL", r.Status)
	}
}

func TestCheckGeoFiles(t *testing.T) {
	dir := t.TempDir()
	r := checkGeoFiles(dir, 7)(context.Background())
	if r.Status != WARN {
		t.Errorf("пустая директория: Status = %s, ожидался WARN", r.Status)
	}

	if err := os.WriteFile(filepath.Join(dir, "geosite.srs"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r = checkGeoFiles(dir, 7)(context.Background())
	if r.Status != PASS {
		t.Errorf("свежий файл: Status = %s, ожидался PASS", r.Status)
	}
}

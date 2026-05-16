package diag

import (
	"bytes"
	"context"
	"encoding/json"
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

// TestResultJSON_ValidEncoding проверяет, что []Result корректно кодируется в JSON
// и содержит ожидаемые поля name/status/detail (имитирует --diag --json).
func TestResultJSON_ValidEncoding(t *testing.T) {
	results := []Result{
		{Name: "binary-self", Status: PASS, Detail: "/opt/sbin/sign-craze"},
		{Name: "config-valid", Status: FAIL, Detail: "конфиг отсутствует"},
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(results); err != nil {
		t.Fatalf("json.Encode: %v", err)
	}

	raw := buf.Bytes()
	if !json.Valid(raw) {
		t.Fatalf("невалидный JSON: %s", raw)
	}

	// Проверяем schema: хотя бы одна запись с полями name/status/detail
	var decoded []map[string]string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(decoded) == 0 {
		t.Fatal("пустой массив в JSON")
	}
	first := decoded[0]
	for _, field := range []string{"name", "status", "detail"} {
		if _, ok := first[field]; !ok {
			t.Errorf("поле %q отсутствует в JSON-записи: %v", field, first)
		}
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

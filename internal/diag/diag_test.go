package diag

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
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

// ===== checkRuleSetURLs (инцидент 2026-05-12, tasks/lessons.md) =====

// writeRoutingJSON — хелпер: сериализует RoutingConfig в файл для теста.
func writeRoutingJSON(t *testing.T, path string, cfg types.RoutingConfig) {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal routing config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("запись %s: %v", path, err)
	}
}

// TestCheckRuleSetURLs_NoRoutingFile — routing.json отсутствует → PASS-skip.
// diag не должен требовать routing UI: не у всех пользователей он настроен.
func TestCheckRuleSetURLs_NoRoutingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-routing.json")
	r := checkRuleSetURLs(path)(context.Background())
	if r.Status != PASS {
		t.Errorf("routing.json отсутствует: Status = %s, ожидался PASS, detail=%s", r.Status, r.Detail)
	}
}

// TestCheckRuleSetURLs_EmptyRuleSets — routing.json есть, но rule_sets пуст → PASS-skip.
func TestCheckRuleSetURLs_EmptyRuleSets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")
	writeRoutingJSON(t, path, types.RoutingConfig{Version: 1})

	r := checkRuleSetURLs(path)(context.Background())
	if r.Status != PASS {
		t.Errorf("rule_sets пуст: Status = %s, ожидался PASS, detail=%s", r.Status, r.Detail)
	}
}

// TestCheckRuleSetURLs_404_WARN — rule_set с 404 URL → WARN (не FAIL: diag не
// должен считать это аварией сервиса — единичный проблемный rule_set не
// критичнее, чем, например, остановленный nfqws2).
func TestCheckRuleSetURLs_404_WARN(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "routing.json")
	writeRoutingJSON(t, path, types.RoutingConfig{
		Version: 1,
		RuleSets: []types.RuleSetRef{
			{Tag: "ru-sites", Type: "remote", Format: "binary", URL: srv.URL + "/geosite-ru.srs"},
		},
	})

	r := checkRuleSetURLs(path)(context.Background())
	if r.Status != WARN {
		t.Errorf("404: Status = %s, ожидался WARN, detail=%s", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "ru-sites") {
		t.Errorf("detail не содержит тег проблемного rule_set: %s", r.Detail)
	}
}

// TestCheckRuleSetURLs_FormatMismatch_WARN — JSON source вместо compiled SRS
// под .srs URL (ровно кейс инцидента 2026-05-12) → WARN.
func TestCheckRuleSetURLs_FormatMismatch_WARN(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version": 2, "rules": []}`))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "routing.json")
	writeRoutingJSON(t, path, types.RoutingConfig{
		Version: 1,
		RuleSets: []types.RuleSetRef{
			{Tag: "ru-sites", Type: "remote", Format: "binary", URL: srv.URL + "/geosite-category-ru.srs"},
		},
	})

	r := checkRuleSetURLs(path)(context.Background())
	if r.Status != WARN {
		t.Errorf("format mismatch: Status = %s, ожидался WARN, detail=%s", r.Status, r.Detail)
	}
}

// TestCheckRuleSetURLs_Unreachable_WARNSkipped — сеть недоступна → WARN с
// текстом "пропущено: сеть недоступна" (не FAIL — offline-роутер это не
// ошибка конфигурации, diag не должен об этом кричать как об аварии).
func TestCheckRuleSetURLs_Unreachable_WARNSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL + "/geoip-ru.srs"
	srv.Close()

	path := filepath.Join(t.TempDir(), "routing.json")
	writeRoutingJSON(t, path, types.RoutingConfig{
		Version: 1,
		RuleSets: []types.RuleSetRef{
			{Tag: "geoip-ru", Type: "remote", URL: url},
		},
	})

	r := checkRuleSetURLs(path)(context.Background())
	if r.Status != WARN {
		t.Errorf("сеть недоступна: Status = %s, ожидался WARN, detail=%s", r.Status, r.Detail)
	}
	if !strings.Contains(r.Detail, "пропущено: сеть недоступна") {
		t.Errorf("detail = %q, ожидалась подстрока «пропущено: сеть недоступна»", r.Detail)
	}
}

// TestCheckRuleSetURLs_AllOK_PASS — все rule_set URL доступны и формат
// совпадает → PASS.
func TestCheckRuleSetURLs_AllOK_PASS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(append([]byte("SRS"), make([]byte, 16)...))
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "routing.json")
	writeRoutingJSON(t, path, types.RoutingConfig{
		Version: 1,
		RuleSets: []types.RuleSetRef{
			{Tag: "geoip-ru", Type: "remote", Format: "binary", URL: srv.URL + "/geoip-ru.srs"},
		},
	})

	r := checkRuleSetURLs(path)(context.Background())
	if r.Status != PASS {
		t.Errorf("все ок: Status = %s, ожидался PASS, detail=%s", r.Status, r.Detail)
	}
}

// TestCheckRuleSetURLs_NoURLRuleSets_PASS — rule_sets заданы, но все
// type=local/inline (без URL) → PASS-skip, без сетевых обращений.
func TestCheckRuleSetURLs_NoURLRuleSets_PASS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")
	writeRoutingJSON(t, path, types.RoutingConfig{
		Version: 1,
		RuleSets: []types.RuleSetRef{
			{Tag: "local-set", Type: "local", Path: "/opt/etc/sign-craze/local.srs"},
		},
	})

	r := checkRuleSetURLs(path)(context.Background())
	if r.Status != PASS {
		t.Errorf("только local rule_set: Status = %s, ожидался PASS, detail=%s", r.Status, r.Detail)
	}
}

package routing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// TestCheckRuleSetURL_EmptyURL — пустой URL (типичен для type=local/inline) —
// проверка пропускается, сетевых обращений нет.
func TestCheckRuleSetURL_EmptyURL(t *testing.T) {
	res := CheckRuleSetURL(context.Background(), nil, types.RuleSetRef{Tag: "t", Type: "local"})
	if !res.OK || res.Mismatch || res.Unreachable {
		t.Errorf("пустой URL: ожидался OK без Mismatch/Unreachable, получено %+v", res)
	}
}

// TestCheckRuleSetURL_404 — сервер отвечает 404 → Mismatch. Регрессия
// инцидента 2026-05-12 (tasks/lessons.md): sing-box падал с "unexpected
// status: 404 Not Found" на старте.
func TestCheckRuleSetURL_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	ref := types.RuleSetRef{Tag: "ru-sites", Type: "remote", Format: "binary", URL: srv.URL + "/geosite-ru.srs"}
	res := CheckRuleSetURL(context.Background(), srv.Client(), ref)
	if !res.Mismatch || res.OK || res.Unreachable {
		t.Errorf("404: ожидался Mismatch, получено %+v", res)
	}
}

// TestCheckRuleSetURL_5xx — сервер отвечает 500 → Mismatch.
func TestCheckRuleSetURL_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ref := types.RuleSetRef{Tag: "t", Type: "remote", URL: srv.URL + "/x.srs"}
	res := CheckRuleSetURL(context.Background(), srv.Client(), ref)
	if !res.Mismatch || res.OK {
		t.Errorf("500: ожидался Mismatch, получено %+v", res)
	}
}

// TestCheckRuleSetURL_JSONInsteadOfSRS_Mismatch — ровно кейс инцидента
// 2026-05-12: .srs URL с format=binary, но тело — JSON source
// ({"version": 2, "rules": [...]}), а не compiled binary. Должен быть Mismatch.
func TestCheckRuleSetURL_JSONInsteadOfSRS_Mismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version": 2, "rules": [{"domain_suffix": [".ru"]}]}`))
	}))
	defer srv.Close()

	ref := types.RuleSetRef{Tag: "ru-sites", Type: "remote", Format: "binary", URL: srv.URL + "/geosite-category-ru.srs"}
	res := CheckRuleSetURL(context.Background(), srv.Client(), ref)
	if !res.Mismatch || res.OK {
		t.Errorf("JSON под .srs: ожидался Mismatch, получено %+v", res)
	}
}

// TestCheckRuleSetURL_SRSMagicOK — корректный compiled rule-set (сигнатура
// "SRS") проходит проверку. client=nil → используется http.DefaultClient
// (бьёт в локальный httptest-сервер, не во внешнюю сеть — безопасно).
func TestCheckRuleSetURL_SRSMagicOK(t *testing.T) {
	body := append([]byte("SRS"), make([]byte, 32)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	ref := types.RuleSetRef{Tag: "geoip-ru", Type: "remote", Format: "binary", URL: srv.URL + "/geoip-ru.srs"}
	res := CheckRuleSetURL(context.Background(), nil, ref)
	if !res.OK || res.Mismatch || res.Unreachable {
		t.Errorf("валидный SRS: ожидался OK, получено %+v", res)
	}
}

// TestCheckRuleSetURL_SourceFormatOK — JSON source rule-set (format=source)
// с корректным телом, начинающимся с '{'.
func TestCheckRuleSetURL_SourceFormatOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version": 2, "rules": []}`))
	}))
	defer srv.Close()

	ref := types.RuleSetRef{Tag: "t", Type: "remote", Format: "source", URL: srv.URL + "/x.json"}
	res := CheckRuleSetURL(context.Background(), srv.Client(), ref)
	if !res.OK || res.Mismatch {
		t.Errorf("валидный source JSON: ожидался OK, получено %+v", res)
	}
}

// TestCheckRuleSetURL_SourceFormatMismatch — format=source, но тело не JSON
// (не начинается с '{').
func TestCheckRuleSetURL_SourceFormatMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("SRS\x00\x00binarygarbage"))
	}))
	defer srv.Close()

	ref := types.RuleSetRef{Tag: "t", Type: "remote", Format: "source", URL: srv.URL + "/x.json"}
	res := CheckRuleSetURL(context.Background(), srv.Client(), ref)
	if !res.Mismatch || res.OK {
		t.Errorf("binary тело под format=source: ожидался Mismatch, получено %+v", res)
	}
}

// TestCheckRuleSetURL_UnknownFormat_Skipped — нераспознанный формат
// (напр. mihomo .mrs, format пуст) → magic-сверка сознательно пропускается,
// OK=true независимо от содержимого. "Не усложняй" — поддерживаем только
// srs/json кейсы sing-box.
func TestCheckRuleSetURL_UnknownFormat_Skipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("MRS1garbage"))
	}))
	defer srv.Close()

	ref := types.RuleSetRef{Tag: "t", Type: "remote", URL: srv.URL + "/x.mrs"}
	res := CheckRuleSetURL(context.Background(), srv.Client(), ref)
	if !res.OK || res.Mismatch {
		t.Errorf("неизвестный формат: ожидался OK (skip), получено %+v", res)
	}
}

// TestCheckRuleSetURL_HEADNotSupported_FallbackToGET — сервер отвечает 405 на
// HEAD → CheckRuleSetURL обязан упасть на GET Range и всё равно корректно
// распознать формат за один дополнительный запрос.
func TestCheckRuleSetURL_HEADNotSupported_FallbackToGET(t *testing.T) {
	body := append([]byte("SRS"), make([]byte, 32)...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	ref := types.RuleSetRef{Tag: "t", Type: "remote", Format: "binary", URL: srv.URL + "/x.srs"}
	res := CheckRuleSetURL(context.Background(), srv.Client(), ref)
	if !res.OK || res.Mismatch || res.Unreachable {
		t.Errorf("HEAD 405 + fallback GET: ожидался OK, получено %+v", res)
	}
}

// TestCheckRuleSetURL_Unreachable — сервер остановлен → сетевая ошибка,
// отдельная категория Unreachable (не Mismatch): offline-роутер — это не
// ошибка конфигурации rule_set.
func TestCheckRuleSetURL_Unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL + "/x.srs"
	srv.Close() // порт освобождён, соединения теперь обрываются

	ref := types.RuleSetRef{Tag: "t", Type: "remote", URL: url}
	res := CheckRuleSetURL(context.Background(), http.DefaultClient, ref)
	if !res.Unreachable || res.Mismatch || res.OK {
		t.Errorf("сервер недоступен: ожидался Unreachable, получено %+v", res)
	}
}

// TestCheckRuleSetURL_DoesNotReadWholeBody — сервер отдаёт длинное тело;
// проверка не должна требовать его целиком (используется io.LimitReader).
// Тест проверяет, что функция не виснет и не падает по памяти на большом теле.
func TestCheckRuleSetURL_DoesNotReadWholeBody(t *testing.T) {
	big := append([]byte("SRS"), make([]byte, 4*1024*1024)...) // 4MB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	ref := types.RuleSetRef{Tag: "t", Type: "remote", Format: "binary", URL: srv.URL + "/big.srs"}
	res := CheckRuleSetURL(context.Background(), srv.Client(), ref)
	if !res.OK {
		t.Errorf("большое тело с валидной сигнатурой: ожидался OK, получено %+v", res)
	}
}

package web

import (
	"net/http/httptest"
	"testing"
)

// TestWSUpgrade_NoSecKey — отсутствие Sec-WebSocket-Key должно давать 400.
func TestWSUpgrade_NoSecKey(t *testing.T) {
	req := httptest.NewRequest("GET", "/traffic", nil)
	rec := httptest.NewRecorder()

	conn, err := wsUpgrade(rec, req)
	if err == nil {
		t.Fatal("ожидалась ошибка для запроса без Sec-WebSocket-Key")
	}
	if conn != nil {
		t.Error("conn должен быть nil при ошибке")
	}
	if rec.Code != 400 {
		t.Errorf("ожидался 400, получен %d", rec.Code)
	}
}

// TestWSUpgrade_HijackerNotSupported — httptest.ResponseRecorder не реализует
// http.Hijacker, поэтому wsUpgrade должен вернуть 500.
func TestWSUpgrade_HijackerNotSupported(t *testing.T) {
	req := httptest.NewRequest("GET", "/traffic", nil)
	req.Header.Set("Sec-Websocket-Key", "test-key-here")
	rec := httptest.NewRecorder()

	_, err := wsUpgrade(rec, req)
	if err == nil {
		t.Fatal("ожидалась ошибка для ResponseWriter без Hijacker")
	}
}

// TestMakeWSHeader_РазмерыКадров — длина payload отражается в заголовке
// согласно RFC 6455 §5.2.
func TestMakeWSHeader_РазмерыКадров(t *testing.T) {
	tests := []struct {
		n       int
		wantLen int
	}{
		{0, 2},        // короткий: 2 байта (FIN+opcode, mask+len)
		{125, 2},      // граница 7-bit length
		{126, 4},      // 16-bit extended length: 4 байта
		{65535, 4},    // граница 16-bit
		{65536, 10},   // 64-bit extended length: 10 байт
		{1000000, 10}, // 1MB → 64-bit
	}
	for _, tt := range tests {
		got := makeWSHeader(tt.n)
		if len(got) != tt.wantLen {
			t.Errorf("makeWSHeader(%d) len=%d, ожидалось %d", tt.n, len(got), tt.wantLen)
		}
		if got[0] != 0x81 {
			t.Errorf("makeWSHeader(%d)[0] = %x, ожидалось 0x81 (FIN+text)", tt.n, got[0])
		}
	}
}

// TestWSAcceptKey_RFC6455 — известное значение из RFC 6455 §1.3.
// "dGhlIHNhbXBsZSBub25jZQ==" + GUID → "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
func TestWSAcceptKey_RFC6455(t *testing.T) {
	got := wsAcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Errorf("wsAcceptKey = %q, ожидалось %q (RFC 6455 §1.3)", got, want)
	}
}

package geo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// validDATFixture — минимальная валидная микро-фикстура: одна top-level
// запись field=1/wiretype=2 (tag=0x0a), длина=5, payload="hello".
func validDATFixture() []byte {
	return []byte{0x0a, 0x05, 'h', 'e', 'l', 'l', 'o'}
}

func TestValidateDAT_ValidMicroFixture(t *testing.T) {
	if err := ValidateDAT(bytes.NewReader(validDATFixture()), MaxDATSize); err != nil {
		t.Fatalf("ValidateDAT(валидная фикстура) = %v, ожидался nil", err)
	}
}

func TestValidateDAT_MultipleEntries(t *testing.T) {
	var buf bytes.Buffer
	buf.Write([]byte{0x0a, 0x03, 'f', 'o', 'o'}) // entry 0: field1/wt2, len3
	buf.Write([]byte{0x12, 0x03, 'b', 'a', 'r'}) // entry 1: field2/wt2, len3 (форвард-совместимость: другой field number)
	if err := ValidateDAT(&buf, MaxDATSize); err != nil {
		t.Fatalf("ValidateDAT(2 записи) = %v, ожидался nil", err)
	}
}

func TestValidateDAT_EmptyFile(t *testing.T) {
	err := ValidateDAT(bytes.NewReader(nil), MaxDATSize)
	if err == nil {
		t.Fatal("ожидалась ошибка для пустого файла")
	}
}

// TestValidateDAT_BrokenVarint — varint с битом продолжения, обрывающийся
// на EOF до терминирующего байта.
func TestValidateDAT_BrokenVarint(t *testing.T) {
	err := ValidateDAT(bytes.NewReader([]byte{0x80}), MaxDATSize)
	if err == nil {
		t.Fatal("ожидалась ошибка для оборванного varint")
	}
}

// TestValidateDAT_VarintTooLong — 10 байт подряд с битом продолжения:
// protobuf ограничивает varint 10 байтами для 64-битного значения.
func TestValidateDAT_VarintTooLong(t *testing.T) {
	junk := bytes.Repeat([]byte{0x80}, 10)
	err := ValidateDAT(bytes.NewReader(junk), MaxDATSize)
	if err == nil {
		t.Fatal("ожидалась ошибка для varint длиннее 10 байт")
	}
}

// TestValidateDAT_LengthExceedsLimit — заявленная length-delimited длина
// превышает переданный maxSize (проверка срабатывает до попытки чтения payload).
func TestValidateDAT_LengthExceedsLimit(t *testing.T) {
	// tag=field1/wt2, length=1000 (varint: 0xe8 0x07).
	fixture := []byte{0x0a, 0xe8, 0x07}
	err := ValidateDAT(bytes.NewReader(fixture), 50)
	if err == nil {
		t.Fatal("ожидалась ошибка: длина записи превышает maxSize")
	}
}

// TestValidateDAT_TotalSizeExceedsLimit — ни одна отдельная запись не
// превышает maxSize, но их сумма превышает бюджет.
func TestValidateDAT_TotalSizeExceedsLimit(t *testing.T) {
	entry := []byte{0x0a, 0x05, 'h', 'e', 'l', 'l', 'o'} // 7 байт
	var buf bytes.Buffer
	for i := 0; i < 3; i++ {
		buf.Write(entry)
	}
	// 3*7=21 байт суммарно, лимит меньше суммы.
	err := ValidateDAT(bytes.NewReader(buf.Bytes()), 15)
	if err == nil {
		t.Fatal("ожидалась ошибка: суммарный размер превышает maxSize")
	}
}

// TestValidateDAT_UnsupportedWireType — wiretype=3 (start group, deprecated
// в proto3, v2fly GeoSiteList/GeoIPList его не используют).
func TestValidateDAT_UnsupportedWireType(t *testing.T) {
	tag := byte((1 << 3) | 3) // field=1, wiretype=3
	err := ValidateDAT(bytes.NewReader([]byte{tag}), MaxDATSize)
	if err == nil {
		t.Fatal("ожидалась ошибка для неподдерживаемого wiretype")
	}
}

// TestValidateDAT_RealSampleTruncated — первые 32 байта РЕАЛЬНОГО geosite.dat
// (Loyalsoldier/v2ray-rules-dat, релиз 202607052256; байты сняты вручную через
// `curl -r 0-31` при подготовке этой реализации). Первая запись честно
// заявляет длину 247 байт, но в 32-байтном префиксе после tag+length остаётся
// лишь 28 байт — типичная картина оборванной/усечённой загрузки. Валидатор
// должен распознать это как ошибку, а не молча принять частичный payload.
func TestValidateDAT_RealSampleTruncated(t *testing.T) {
	realPrefix := []byte{
		0x0a, 0xf7, 0x01, 0x0a, 0x04, 0x45, 0x53, 0x45, 0x54, 0x12, 0x3d, 0x08,
		0x03, 0x12, 0x39, 0x65, 0x73, 0x65, 0x74, 0x2d, 0x70, 0x72, 0x6f, 0x64,
		0x2d, 0x63, 0x61, 0x34, 0x38, 0x36, 0x34, 0x38,
	}
	err := ValidateDAT(bytes.NewReader(realPrefix), MaxDATSize)
	if err == nil {
		t.Fatal("ожидалась ошибка для усечённого реального образца geosite.dat")
	}
}

// TestValidateDAT_DefaultsMaxSizeWhenNonPositive проверяет, что maxSize<=0
// переключается на MaxDATSize, а не превращает первый же байт в ошибку.
func TestValidateDAT_DefaultsMaxSizeWhenNonPositive(t *testing.T) {
	if err := ValidateDAT(bytes.NewReader(validDATFixture()), 0); err != nil {
		t.Errorf("ValidateDAT(maxSize=0) = %v, ожидался nil (дефолт на MaxDATSize)", err)
	}
	if err := ValidateDAT(bytes.NewReader(validDATFixture()), -1); err != nil {
		t.Errorf("ValidateDAT(maxSize=-1) = %v, ожидался nil (дефолт на MaxDATSize)", err)
	}
}

// --- DownloadDAT: httptest-сервер с fake-ассетами (тот же паттерн, что
// internal/core/xray/download_test.go: подмена ghrelease.APIBaseURL). ---

func sha256sumLine(name string, content []byte) []byte {
	sum := sha256.Sum256(content)
	return []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")
}

// setupDATTestServer поднимает fake GitHub API: releases/latest отдаёт
// перечень assets из files с BrowserDownloadURL, указывающим на этот же
// сервер; каждый файл раздаётся по своему пути.
func setupDATTestServer(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server

	mux.HandleFunc(fmt.Sprintf("/repos/%s/%s/releases/latest", datRepoOwner, datRepoRepo),
		func(w http.ResponseWriter, _ *http.Request) {
			rel := types.Release{TagName: "202607052256"}
			for name, data := range files {
				rel.Assets = append(rel.Assets, types.Asset{
					Name:               name,
					BrowserDownloadURL: srv.URL + "/download/" + name,
					Size:               int64(len(data)),
				})
			}
			_ = json.NewEncoder(w).Encode(rel)
		})
	for name, data := range files {
		data := data // захват переменной цикла
		mux.HandleFunc("/download/"+name, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(data)
		})
	}

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func withFakeGitHubAPI(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := ghrelease.APIBaseURL
	ghrelease.APIBaseURL = srv.URL
	t.Cleanup(func() { ghrelease.APIBaseURL = orig })
}

func TestDownloadDAT_Success(t *testing.T) {
	geosite := validDATFixture()
	geoip := []byte{0x0a, 0x03, 'f', 'o', 'o'}

	files := map[string][]byte{
		"geosite.dat":           geosite,
		"geosite.dat.sha256sum": sha256sumLine("geosite.dat", geosite),
		"geoip.dat":             geoip,
		"geoip.dat.sha256sum":   sha256sumLine("geoip.dat", geoip),
	}
	withFakeGitHubAPI(t, setupDATTestServer(t, files))

	cacheDir := t.TempDir()
	dstDir := t.TempDir()

	updated, err := DownloadDAT(context.Background(), cacheDir, dstDir)
	if err != nil {
		t.Fatalf("DownloadDAT: %v", err)
	}
	if updated != 2 {
		t.Errorf("updated = %d, ожидалось 2", updated)
	}

	gotSite, err := os.ReadFile(filepath.Join(dstDir, "geosite.dat"))
	if err != nil {
		t.Fatalf("чтение geosite.dat: %v", err)
	}
	if !bytes.Equal(gotSite, geosite) {
		t.Error("содержимое geosite.dat не совпадает с ожидаемым")
	}
	gotIP, err := os.ReadFile(filepath.Join(dstDir, "geoip.dat"))
	if err != nil {
		t.Fatalf("чтение geoip.dat: %v", err)
	}
	if !bytes.Equal(gotIP, geoip) {
		t.Error("содержимое geoip.dat не совпадает с ожидаемым")
	}
}

func TestDownloadDAT_Idempotent(t *testing.T) {
	geosite := validDATFixture()
	geoip := []byte{0x0a, 0x03, 'f', 'o', 'o'}
	files := map[string][]byte{
		"geosite.dat":           geosite,
		"geosite.dat.sha256sum": sha256sumLine("geosite.dat", geosite),
		"geoip.dat":             geoip,
		"geoip.dat.sha256sum":   sha256sumLine("geoip.dat", geoip),
	}
	withFakeGitHubAPI(t, setupDATTestServer(t, files))

	cacheDir := t.TempDir()
	dstDir := t.TempDir()

	if _, err := DownloadDAT(context.Background(), cacheDir, dstDir); err != nil {
		t.Fatalf("первый DownloadDAT: %v", err)
	}
	updated, err := DownloadDAT(context.Background(), cacheDir, dstDir)
	if err != nil {
		t.Fatalf("второй DownloadDAT: %v", err)
	}
	if updated != 0 {
		t.Errorf("второй вызов: updated = %d, ожидалось 0 (файлы уже актуальны)", updated)
	}
}

func TestDownloadDAT_SHA256Mismatch(t *testing.T) {
	geosite := validDATFixture()
	geoip := []byte{0x0a, 0x03, 'f', 'o', 'o'}
	files := map[string][]byte{
		"geosite.dat": geosite,
		// Заведомо неверная контрольная сумма.
		"geosite.dat.sha256sum": []byte("0000000000000000000000000000000000000000000000000000000000000000  geosite.dat\n"),
		"geoip.dat":             geoip,
		"geoip.dat.sha256sum":   sha256sumLine("geoip.dat", geoip),
	}
	withFakeGitHubAPI(t, setupDATTestServer(t, files))

	cacheDir := t.TempDir()
	dstDir := t.TempDir()

	_, err := DownloadDAT(context.Background(), cacheDir, dstDir)
	if err == nil {
		t.Fatal("ожидалась ошибка при несовпадении SHA256")
	}
	if _, statErr := os.Stat(filepath.Join(dstDir, "geosite.dat")); statErr == nil {
		t.Error("geosite.dat не должен был быть записан при несовпадении SHA256")
	}
}

// TestDownloadDAT_StructurallyInvalid — контрольная сумма совпадает (файл не
// повреждён в передаче), но содержимое не является валидным .dat —
// защита ValidateDAT ДО замены production-файла должна сработать.
func TestDownloadDAT_StructurallyInvalid(t *testing.T) {
	geosite := []byte{0x80} // оборванный varint — структурно невалиден
	geoip := []byte{0x0a, 0x03, 'f', 'o', 'o'}
	files := map[string][]byte{
		"geosite.dat":           geosite,
		"geosite.dat.sha256sum": sha256sumLine("geosite.dat", geosite),
		"geoip.dat":             geoip,
		"geoip.dat.sha256sum":   sha256sumLine("geoip.dat", geoip),
	}
	withFakeGitHubAPI(t, setupDATTestServer(t, files))

	cacheDir := t.TempDir()
	dstDir := t.TempDir()

	_, err := DownloadDAT(context.Background(), cacheDir, dstDir)
	if err == nil {
		t.Fatal("ожидалась ошибка структурной проверки для невалидного geosite.dat")
	}
	if _, statErr := os.Stat(filepath.Join(dstDir, "geosite.dat")); statErr == nil {
		t.Error("geosite.dat не должен был быть записан при провале структурной проверки")
	}
}

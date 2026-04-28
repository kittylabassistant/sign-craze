package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestUpdate_Success(t *testing.T) {
	binData := []byte("new-sign-craze-binary")
	hash := sha256.Sum256(binData)
	wantSum := hex.EncodeToString(hash[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			rel := types.Release{
				TagName: "v0.2.0",
				Assets: []types.Asset{
					{Name: "sign-craze-arm64", BrowserDownloadURL: "http://" + r.Host + "/dl/sign-craze-arm64"},
					{Name: "sign-craze-arm64.sha256", BrowserDownloadURL: "http://" + r.Host + "/dl/sign-craze-arm64.sha256"},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = w.Write([]byte(wantSum + "  sign-craze-arm64\n"))
		default:
			_, _ = w.Write(binData)
		}
	}))
	defer srv.Close()

	old := ghrelease.APIBaseURL
	ghrelease.APIBaseURL = srv.URL
	defer func() { ghrelease.APIBaseURL = old }()

	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "sign-craze")

	version, err := Update(context.Background(), nil, Options{
		Owner:   "kittylabassistant",
		Repo:    "sign-craze",
		Arch:    types.ArchARM64,
		BinPath: binPath,
		DstDir:  tmp,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if version != "v0.2.0" {
		t.Errorf("version = %q, ожидалось v0.2.0", version)
	}

	got, _ := os.ReadFile(binPath)
	if string(got) != string(binData) {
		t.Errorf("бинарь не заменён: %q", got)
	}
	info, _ := os.Stat(binPath)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("права = %o, ожидалось 0755", info.Mode().Perm())
	}
}

func TestUpdate_SHA256Mismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			rel := types.Release{
				TagName: "v0.2.0",
				Assets: []types.Asset{
					{Name: "sign-craze-arm64", BrowserDownloadURL: "http://" + r.Host + "/dl/x"},
					{Name: "sign-craze-arm64.sha256", BrowserDownloadURL: "http://" + r.Host + "/dl/x.sha256"},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = w.Write([]byte("deadbeef  sign-craze-arm64\n"))
		default:
			_, _ = w.Write([]byte("payload"))
		}
	}))
	defer srv.Close()

	old := ghrelease.APIBaseURL
	ghrelease.APIBaseURL = srv.URL
	defer func() { ghrelease.APIBaseURL = old }()

	tmp := t.TempDir()
	_, err := Update(context.Background(), nil, Options{
		Arch:    types.ArchARM64,
		BinPath: filepath.Join(tmp, "sign-craze"),
		DstDir:  tmp,
	})
	if err == nil {
		t.Fatal("ожидалась ошибка несовпадения SHA256")
	}
}

func TestUpdate_UnsupportedArch(t *testing.T) {
	_, err := Update(context.Background(), nil, Options{Arch: "riscv"})
	if err == nil {
		t.Fatal("ожидалась ошибка для неподдерживаемой архитектуры")
	}
}

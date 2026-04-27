package singbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// githubReleasesURL — var для подмены в тестах через httptest.Server.
var githubReleasesURL = "https://api.github.com/repos/SagerNet/sing-box/releases/latest"

const (
	downloadTimeout = 10 * time.Minute
	connectTimeout  = 30 * time.Second
)

// assetPattern возвращает подстроку, по которой определяется нужный asset в релизе.
var assetPattern = map[types.Arch]string{
	types.ArchARM64:  "linux-arm64.tar.gz",
	types.ArchARM7:   "linux-armv7.tar.gz",
	types.ArchMIPSLE: "linux-mipsle-softfloat.tar.gz",
	types.ArchMIPS:   "linux-mips-softfloat.tar.gz",
	types.ArchAMD64:  "linux-amd64.tar.gz",
}

var httpClient = &http.Client{
	Timeout: downloadTimeout,
	Transport: &http.Transport{
		ResponseHeaderTimeout: connectTimeout,
	},
}

// DownloadResult описывает результат вызова Download.
type DownloadResult struct {
	Downloaded bool   // false если файл актуален (ETag совпал)
	Version    string // тег релиза, например "v1.10.0"
	Path       string // путь к скачанному файлу
}

// Download скачивает актуальный tarball sing-box для указанной архитектуры в директорию dst.
// При совпадении ETag повторная загрузка не производится.
// Возвращает DownloadResult с флагом Downloaded=false если файл не изменился.
func Download(ctx context.Context, arch types.Arch, dstDir string) (DownloadResult, error) {
	if err := arch.Validate(); err != nil {
		return DownloadResult{}, err
	}

	pattern, ok := assetPattern[arch]
	if !ok {
		return DownloadResult{}, fmt.Errorf("singbox download: нет паттерна для архитектуры %q", arch)
	}

	log.L().Info("проверка последнего релиза sing-box", "arch", arch)

	release, err := fetchLatestRelease(ctx)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("singbox download: получение метаданных релиза: %w", err)
	}

	asset := findAsset(release.Assets, pattern)
	if asset == nil {
		return DownloadResult{}, fmt.Errorf("singbox download: asset для %q не найден в релизе %s", arch, release.TagName)
	}

	dstFile := filepath.Join(dstDir, asset.Name)
	etagFile := dstFile + ".etag"

	savedETag := readETag(etagFile)

	log.L().Info("загрузка sing-box", "version", release.TagName, "asset", asset.Name)

	downloaded, newETag, err := downloadFile(ctx, asset.BrowserDownloadURL, dstFile, savedETag)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("singbox download: загрузка %s: %w", asset.Name, err)
	}

	if downloaded && newETag != "" {
		_ = os.WriteFile(etagFile, []byte(newETag), 0o644)
	}

	return DownloadResult{
		Downloaded: downloaded,
		Version:    release.TagName,
		Path:       dstFile,
	}, nil
}

// fetchLatestRelease запрашивает метаданные последнего релиза sing-box через GitHub API.
func fetchLatestRelease(ctx context.Context) (*types.Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sign-craze")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API вернул %d", resp.StatusCode)
	}

	var release types.Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("декодирование JSON: %w", err)
	}
	return &release, nil
}

// findAsset ищет asset, имя которого содержит pattern.
func findAsset(assets []types.Asset, pattern string) *types.Asset {
	for i := range assets {
		if strings.Contains(assets[i].Name, pattern) {
			return &assets[i]
		}
	}
	return nil
}

// downloadFile скачивает url в dstFile, используя ETag для пропуска загрузки при отсутствии изменений.
// Возвращает (скачан, новый ETag, ошибка).
func downloadFile(ctx context.Context, url, dstFile, savedETag string) (bool, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, "", err
	}
	req.Header.Set("User-Agent", "sign-craze")
	if savedETag != "" {
		req.Header.Set("If-None-Match", savedETag)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		log.L().Info("файл не изменился (ETag совпал)", "url", url)
		return false, savedETag, nil
	}

	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("HTTP %d для %s", resp.StatusCode, url)
	}

	if err := os.MkdirAll(filepath.Dir(dstFile), 0o755); err != nil {
		return false, "", fmt.Errorf("mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dstFile), ".dl-*")
	if err != nil {
		return false, "", fmt.Errorf("создание temp-файла: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil {
		_ = tmp.Close()
		return false, "", fmt.Errorf("чтение тела ответа: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, "", fmt.Errorf("fsync: %w", err)
	}
	_ = tmp.Close()

	checksum := hex.EncodeToString(h.Sum(nil))
	log.L().Debug("загрузка завершена", "sha256", checksum)

	if err := atomicfs.WriteFileAtomic(dstFile, mustReadFile(tmpName), 0o644); err != nil {
		return false, "", fmt.Errorf("атомарная запись: %w", err)
	}

	return true, resp.Header.Get("ETag"), nil
}

func readETag(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func mustReadFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("mustReadFile: %v", err))
	}
	return data
}

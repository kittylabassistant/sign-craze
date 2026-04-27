package dpi

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

// nfqws2ReleasesURL — var для подмены в тестах через httptest.Server.
var nfqws2ReleasesURL = "https://api.github.com/repos/bol-van/nfqws2-keenetic/releases/latest"

const (
	downloadTimeout = 10 * time.Minute
	connectTimeout  = 30 * time.Second
)

// nfqws2AssetPattern — суффикс asset для каждой архитектуры в релизе nfqws2-keenetic.
var nfqws2AssetPattern = map[types.Arch]string{
	types.ArchARM64:  "aarch64.tar.gz",
	types.ArchARM7:   "armv7l.tar.gz",
	types.ArchMIPSLE: "mipsel.tar.gz",
	types.ArchMIPS:   "mips.tar.gz",
	types.ArchAMD64:  "x86_64.tar.gz",
}

var dpiHTTPClient = &http.Client{
	Timeout: downloadTimeout,
	Transport: &http.Transport{
		ResponseHeaderTimeout: connectTimeout,
	},
}

// DownloadResult описывает результат вызова Download.
type DownloadResult struct {
	Downloaded bool   // false если файл актуален (ETag совпал)
	Version    string // тег релиза
	Path       string // путь к скачанному tarball
}

// Download скачивает актуальный tarball nfqws2-keenetic для указанной архитектуры в dstDir.
// При совпадении ETag повторная загрузка не производится.
func Download(ctx context.Context, arch types.Arch, dstDir string) (DownloadResult, error) {
	if err := arch.Validate(); err != nil {
		return DownloadResult{}, err
	}

	pattern, ok := nfqws2AssetPattern[arch]
	if !ok {
		return DownloadResult{}, fmt.Errorf("dpi download: нет паттерна для архитектуры %q", arch)
	}

	log.L().Info("проверка последнего релиза nfqws2-keenetic", "arch", arch)

	release, err := fetchRelease(ctx)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("dpi download: получение метаданных релиза: %w", err)
	}

	asset := findAsset(release.Assets, pattern)
	if asset == nil {
		return DownloadResult{}, fmt.Errorf("dpi download: asset для %q не найден в релизе %s", arch, release.TagName)
	}

	dstFile := filepath.Join(dstDir, asset.Name)
	etagFile := dstFile + ".etag"

	savedETag := readETag(etagFile)
	log.L().Info("загрузка nfqws2-keenetic", "version", release.TagName, "asset", asset.Name)

	downloaded, newETag, err := downloadFile(ctx, asset.BrowserDownloadURL, dstFile, savedETag)
	if err != nil {
		return DownloadResult{}, fmt.Errorf("dpi download: загрузка %s: %w", asset.Name, err)
	}

	if downloaded && newETag != "" {
		if err := os.WriteFile(etagFile, []byte(newETag), 0o644); err != nil {
			log.L().Warn("не удалось сохранить ETag", "file", etagFile, "err", err)
		}
	}

	return DownloadResult{
		Downloaded: downloaded,
		Version:    release.TagName,
		Path:       dstFile,
	}, nil
}

func fetchRelease(ctx context.Context) (*types.Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nfqws2ReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sign-craze")

	resp, err := dpiHTTPClient.Do(req)
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

func findAsset(assets []types.Asset, pattern string) *types.Asset {
	for i := range assets {
		if strings.Contains(assets[i].Name, pattern) {
			return &assets[i]
		}
	}
	return nil
}

func downloadFile(ctx context.Context, url, dstFile, savedETag string) (bool, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, "", err
	}
	req.Header.Set("User-Agent", "sign-craze")
	if savedETag != "" {
		req.Header.Set("If-None-Match", savedETag)
	}

	resp, err := dpiHTTPClient.Do(req)
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

	log.L().Debug("загрузка завершена", "sha256", hex.EncodeToString(h.Sum(nil)))

	data, err := os.ReadFile(tmpName)
	if err != nil {
		return false, "", fmt.Errorf("чтение tmp-файла: %w", err)
	}
	if err := atomicfs.WriteFileAtomic(dstFile, data, 0o644); err != nil {
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

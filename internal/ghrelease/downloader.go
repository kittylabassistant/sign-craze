package ghrelease

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

const (
	defaultDownloadTimeout = 10 * time.Minute
	defaultConnectTimeout  = 30 * time.Second
	userAgent              = "sign-craze"
)

// APIBaseURL — базовый URL GitHub API. Перезаписывается в тестах через httptest.Server.
var APIBaseURL = "https://api.github.com"

// Downloader выполняет загрузки asset'ов из GitHub Releases.
type Downloader struct {
	HTTPClient *http.Client
}

// New создаёт Downloader со стандартным HTTP-клиентом.
func New() *Downloader {
	return &Downloader{
		HTTPClient: &http.Client{
			Timeout: defaultDownloadTimeout,
			Transport: &http.Transport{
				ResponseHeaderTimeout: defaultConnectTimeout,
			},
		},
	}
}

// FetchOptions описывает параметры загрузки.
type FetchOptions struct {
	Owner      string                 // владелец репозитория (например "SagerNet")
	Repo       string                 // имя репозитория (например "sing-box")
	AssetMatch func(types.Asset) bool // выбор нужного asset из релиза
	DstDir     string                 // директория для записи
	VerifySHA  bool                   // если true — ищет рядом asset с суффиксом .sha256 и сверяет
}

// FetchResult описывает результат загрузки.
type FetchResult struct {
	Downloaded bool   // false если ETag совпал и файл не перезаписан
	Version    string // тег релиза (например "v1.10.0")
	Path       string // путь к скачанному файлу
}

// Fetch скачивает asset из последнего релиза репозитория.
// При совпадении ETag повторная загрузка пропускается.
func (d *Downloader) Fetch(ctx context.Context, opts FetchOptions) (FetchResult, error) {
	if opts.Owner == "" || opts.Repo == "" {
		return FetchResult{}, fmt.Errorf("ghrelease: пустой Owner/Repo")
	}
	if opts.AssetMatch == nil {
		return FetchResult{}, fmt.Errorf("ghrelease: AssetMatch не задан")
	}

	release, err := d.LatestRelease(ctx, opts.Owner, opts.Repo)
	if err != nil {
		return FetchResult{}, fmt.Errorf("ghrelease: метаданные релиза %s/%s: %w", opts.Owner, opts.Repo, err)
	}

	asset := matchAsset(release.Assets, opts.AssetMatch)
	if asset == nil {
		return FetchResult{}, fmt.Errorf("ghrelease: asset не найден в релизе %s/%s %s", opts.Owner, opts.Repo, release.TagName)
	}

	dstFile := filepath.Join(opts.DstDir, asset.Name)
	etagFile := dstFile + ".etag"
	savedETag := readETag(etagFile)

	log.L().Info("ghrelease: загрузка", "repo", opts.Owner+"/"+opts.Repo, "version", release.TagName, "asset", asset.Name)

	downloaded, newETag, sum, err := d.downloadAsset(ctx, asset.BrowserDownloadURL, dstFile, savedETag)
	if err != nil {
		return FetchResult{}, fmt.Errorf("ghrelease: загрузка %s: %w", asset.Name, err)
	}

	if downloaded && newETag != "" {
		if err := os.WriteFile(etagFile, []byte(newETag), 0o644); err != nil {
			log.L().Warn("ghrelease: не удалось сохранить ETag", "file", etagFile, "err", err)
		}
	}

	// Проверка размера: GitHub manifest указывает Size, скачанный файл должен
	// совпадать. Truncated-загрузка (порванный канал, MITM-обрыв) ловится здесь
	// до того, как файл попадёт в extractBinary.
	if downloaded && asset.Size > 0 {
		info, statErr := os.Stat(dstFile)
		if statErr == nil && info.Size() != asset.Size {
			_ = os.Remove(dstFile)
			return FetchResult{}, fmt.Errorf("ghrelease: размер %s = %d не совпадает с manifest %d (загрузка прервана?)",
				asset.Name, info.Size(), asset.Size)
		}
	}

	if downloaded && opts.VerifySHA {
		if vErr := d.verifySHA256(ctx, release.Assets, asset.Name, sum); vErr != nil {
			_ = os.Remove(dstFile)
			return FetchResult{}, fmt.Errorf("ghrelease: проверка SHA256 %s: %w", asset.Name, vErr)
		}
	}

	return FetchResult{
		Downloaded: downloaded,
		Version:    release.TagName,
		Path:       dstFile,
	}, nil
}

// LatestRelease возвращает метаданные последнего релиза репозитория.
func (d *Downloader) LatestRelease(ctx context.Context, owner, repo string) (*types.Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", APIBaseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := d.HTTPClient.Do(req)
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

// downloadAsset скачивает url в dstFile с поддержкой ETag.
// Возвращает (downloaded, newETag, sha256-hex).
func (d *Downloader) downloadAsset(ctx context.Context, url, dstFile, savedETag string) (bool, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, "", "", err
	}
	req.Header.Set("User-Agent", userAgent)
	if savedETag != "" {
		req.Header.Set("If-None-Match", savedETag)
	}

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return false, "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		log.L().Info("ghrelease: файл не изменился (ETag совпал)", "url", url)
		return false, savedETag, "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, "", "", fmt.Errorf("HTTP %d для %s", resp.StatusCode, url)
	}

	if mkErr := os.MkdirAll(filepath.Dir(dstFile), 0o755); mkErr != nil {
		return false, "", "", fmt.Errorf("mkdir: %w", mkErr)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dstFile), ".dl-*")
	if err != nil {
		return false, "", "", fmt.Errorf("создание temp-файла: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	h := sha256.New()
	if _, cpErr := io.Copy(io.MultiWriter(tmp, h), resp.Body); cpErr != nil {
		_ = tmp.Close()
		return false, "", "", fmt.Errorf("чтение тела ответа: %w", cpErr)
	}
	if syncErr := tmp.Sync(); syncErr != nil {
		_ = tmp.Close()
		return false, "", "", fmt.Errorf("fsync: %w", syncErr)
	}
	_ = tmp.Close()

	checksum := hex.EncodeToString(h.Sum(nil))
	log.L().Debug("ghrelease: загрузка завершена", "sha256", checksum)

	data, err := os.ReadFile(tmpName)
	if err != nil {
		return false, "", "", fmt.Errorf("чтение tmp-файла: %w", err)
	}
	if wErr := atomicfs.WriteFileAtomic(dstFile, data, 0o644); wErr != nil {
		return false, "", "", fmt.Errorf("атомарная запись: %w", wErr)
	}

	return true, resp.Header.Get("ETag"), checksum, nil
}

// verifySHA256 ищет в релизе asset с именем "<assetName>.sha256" и сверяет хэш.
func (d *Downloader) verifySHA256(ctx context.Context, assets []types.Asset, assetName, gotSum string) error {
	wantName := assetName + ".sha256"
	var sumAsset *types.Asset
	for i := range assets {
		if assets[i].Name == wantName {
			sumAsset = &assets[i]
			break
		}
	}
	if sumAsset == nil {
		return fmt.Errorf("asset %q отсутствует в релизе", wantName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumAsset.BrowserDownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d для %s", resp.StatusCode, sumAsset.BrowserDownloadURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	wantSum := strings.Fields(strings.TrimSpace(string(body)))
	if len(wantSum) == 0 {
		return fmt.Errorf("пустой sha256-файл")
	}
	if !strings.EqualFold(wantSum[0], gotSum) {
		return fmt.Errorf("несовпадение sha256: ожидалось %s, получено %s", wantSum[0], gotSum)
	}
	return nil
}

// MatchByContains возвращает AssetMatch, выбирающий первый asset, имя которого содержит substr.
func MatchByContains(substr string) func(types.Asset) bool {
	return func(a types.Asset) bool {
		return strings.Contains(a.Name, substr)
	}
}

func matchAsset(assets []types.Asset, fn func(types.Asset) bool) *types.Asset {
	for i := range assets {
		if fn(assets[i]) {
			return &assets[i]
		}
	}
	return nil
}

func readETag(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

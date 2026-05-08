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

	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

const (
	defaultDownloadTimeout = 10 * time.Minute
	// defaultConnectTimeout — потолок ожидания первого байта response header.
	// 15s — достаточно для медленного TLS handshake к api.github.com, но
	// быстро триггерит fallback на gh-proxy/jsdelivr если GitHub блокируется
	// провайдером (типичная ситуация в РФ, где sing-box не качается напрямую).
	defaultConnectTimeout = 15 * time.Second
	// userAgent — curl-like UA. GitHub anonymous-downloads throttles
	// нестандартные User-Agents в low-priority bucket (наблюдалось ~3 минуты
	// на 16MB файл против ~2s через curl). Использование curl-like UA
	// возвращает обычную полосу.
	userAgent = "curl/8.12.1"
	// progressInterval — частота прогресс-логов при загрузке asset'а.
	progressInterval = 5 * time.Second
)

// APIBaseURL — базовый URL GitHub API. Перезаписывается в тестах через httptest.Server.
var APIBaseURL = "https://api.github.com"

// Downloader выполняет загрузки asset'ов из GitHub Releases.
type Downloader struct {
	HTTPClient *http.Client

	// Mirrors — порядок попыток при сбое (network error / HTTP 5xx).
	// nil → DefaultMirrors (direct → gh-proxy.com → jsdelivr). Тесты
	// переопределяют для проверки fallback-логики; production — оставляет nil.
	Mirrors []URLRewriter
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
	Tag        string                 // конкретный tag релиза; пусто — latest
	AssetMatch func(types.Asset) bool // выбор нужного asset из релиза
	DstDir     string                 // директория для записи
	VerifySHA  bool                   // если true — ищет рядом asset с суффиксом .sha256 и сверяет
	// AllowMissingSHA — при VerifySHA=true и отсутствии `<asset>.sha256` в
	// релизе НЕ возвращать ошибку, а только записать WARN. Использовать
	// исключительно для upstream-репозиториев, которые исторически не
	// публикуют .sha256 (например nfqws2-keenetic). Для собственных и
	// крупных upstream (SagerNet/sing-box, MetaCubeX/mihomo) оставлять false.
	AllowMissingSHA bool
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

	var release *types.Release
	var err error
	if opts.Tag != "" {
		release, err = d.releaseByTag(ctx, opts.Owner, opts.Repo, opts.Tag)
	} else {
		release, err = d.latestRelease(ctx, opts.Owner, opts.Repo)
	}
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
		if vErr := d.verifySHA256(ctx, release.Assets, asset.Name, sum, opts.AllowMissingSHA); vErr != nil {
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

// latestRelease возвращает метаданные последнего релиза репозитория.
func (d *Downloader) latestRelease(ctx context.Context, owner, repo string) (*types.Release, error) {
	return d.fetchReleaseMeta(ctx, fmt.Sprintf("%s/repos/%s/%s/releases/latest", APIBaseURL, owner, repo))
}

// releaseByTag возвращает метаданные релиза по конкретному тегу.
// Используется когда требуется pinned-версия (например, собственные сборки
// xray для MIPS лежат под отдельным tag-ом и не являются "latest").
func (d *Downloader) releaseByTag(ctx context.Context, owner, repo, tag string) (*types.Release, error) {
	return d.fetchReleaseMeta(ctx, fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", APIBaseURL, owner, repo, tag))
}

func (d *Downloader) fetchReleaseMeta(ctx context.Context, url string) (*types.Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := d.doWithFallback(req)
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

	resp, err := d.doWithFallback(req)
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

	// Имя tmp-файла рядом с dstFile, чтобы rename был на одной FS.
	tmp, err := os.CreateTemp(filepath.Dir(dstFile), ".dl-*")
	if err != nil {
		return false, "", "", fmt.Errorf("создание temp-файла: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	defer func() {
		if tmpName != "" {
			cleanup()
		}
	}()

	totalSize := resp.ContentLength
	pr := newProgressReader(resp.Body, totalSize)

	h := sha256.New()
	if _, cpErr := io.Copy(io.MultiWriter(tmp, h), pr); cpErr != nil {
		_ = tmp.Close()
		return false, "", "", fmt.Errorf("чтение тела ответа: %w", cpErr)
	}
	if syncErr := tmp.Sync(); syncErr != nil {
		_ = tmp.Close()
		return false, "", "", fmt.Errorf("fsync: %w", syncErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		return false, "", "", fmt.Errorf("закрытие tmp-файла: %w", closeErr)
	}

	checksum := hex.EncodeToString(h.Sum(nil))
	log.L().Debug("ghrelease: загрузка завершена", "sha256", checksum, "bytes", pr.read)

	// Атомарный rename вместо ReadFile+WriteFileAtomic: tmp на той же FS,
	// rename атомарен в POSIX. Раньше был лишний 16MB-аллок в RAM, что
	// проблема на 128MB-роутерах.
	if rnErr := os.Rename(tmpName, dstFile); rnErr != nil {
		return false, "", "", fmt.Errorf("атомарный rename %s → %s: %w", tmpName, dstFile, rnErr)
	}
	tmpName = "" // успех — не удалять

	return true, resp.Header.Get("ETag"), checksum, nil
}

// progressReader логирует прогресс загрузки каждые progressInterval.
// Без него юзер не видит идёт ли загрузка или висит — критично на slow links
// где загрузка 16MB занимает 30-60s даже на хорошем канале.
type progressReader struct {
	r        io.Reader
	total    int64 // -1 если ContentLength неизвестен
	read     int64
	lastTick time.Time
	started  time.Time
}

func newProgressReader(r io.Reader, total int64) *progressReader {
	now := time.Now()
	return &progressReader{r: r, total: total, lastTick: now, started: now}
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.read += int64(n)
	if time.Since(p.lastTick) >= progressInterval {
		p.tick()
		p.lastTick = time.Now()
	}
	return n, err
}

func (p *progressReader) tick() {
	elapsed := time.Since(p.started).Seconds()
	if elapsed < 0.001 {
		return
	}
	speed := float64(p.read) / elapsed / 1024 // KB/s
	if p.total > 0 {
		pct := float64(p.read) * 100 / float64(p.total)
		log.L().Info("ghrelease: прогресс",
			"загружено_МБ", fmt.Sprintf("%.1f", float64(p.read)/(1024*1024)),
			"всего_МБ", fmt.Sprintf("%.1f", float64(p.total)/(1024*1024)),
			"процент", fmt.Sprintf("%.0f", pct),
			"скорость_КБ/с", fmt.Sprintf("%.0f", speed),
		)
	} else {
		log.L().Info("ghrelease: прогресс",
			"загружено_МБ", fmt.Sprintf("%.1f", float64(p.read)/(1024*1024)),
			"скорость_КБ/с", fmt.Sprintf("%.0f", speed),
		)
	}
}

// verifySHA256 ищет в релизе asset с именем "<assetName>.sha256" и сверяет хэш.
// Если allowMissing=true и .sha256 не найден — WARN-лог + nil (защита для
// upstream без культуры публикации checksums).
func (d *Downloader) verifySHA256(ctx context.Context, assets []types.Asset, assetName, gotSum string, allowMissing bool) error {
	wantName := assetName + ".sha256"
	var sumAsset *types.Asset
	for i := range assets {
		if assets[i].Name == wantName {
			sumAsset = &assets[i]
			break
		}
	}
	if sumAsset == nil {
		if allowMissing {
			log.L().Warn("ghrelease: .sha256 отсутствует в релизе, integrity-проверка пропущена",
				"asset", assetName)
			return nil
		}
		return fmt.Errorf("asset %q отсутствует в релизе", wantName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumAsset.BrowserDownloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := d.doWithFallback(req)
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

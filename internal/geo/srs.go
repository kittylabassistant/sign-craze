package geo

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
	"time"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

const (
	// DefaultGeoDir — директория хранения .srs файлов на роутере.
	DefaultGeoDir = "/opt/var/lib/sign-craze/geo/"

	// MaxGeoFileSize — глобальный «hard cap» на размер одного скачиваемого .srs,
	// применяется когда manifest.Size неизвестен (=0). Для известных файлов
	// authoritative — manifest.Size + sizeTolerance.
	//
	// 200MB покрывает крупные geoip-базы (полный maxmind с ASN ~30-50MB,
	// объединённые .srs до ~100MB) с запасом, при этом блокирует OOM от
	// malicious mirror, отдающего безграничный поток на 128MB-роутер.
	MaxGeoFileSize int64 = 200 * 1024 * 1024

	// sizeTolerance — допустимое превышение manifest.Size (zip-padding,
	// header-разница). 10% + 1KB.
	sizeTolerance = 1 * 1024
)

// effectiveLimit вычисляет ограничение на скачивание. Если manifest.Size
// задан — используем его + 10% + 1KB как authoritative; иначе — глобальный cap.
func effectiveLimit(manifestSize int64) int64 {
	if manifestSize <= 0 {
		return MaxGeoFileSize
	}
	limit := manifestSize + manifestSize/10 + sizeTolerance
	if limit > MaxGeoFileSize {
		return MaxGeoFileSize
	}
	return limit
}

// manifestURLVar и downloadBaseURLVar — var для подмены в тестах через httptest.Server.
var (
	manifestURLVar     = "https://github.com/kittylabassistant/sign-craze-dat/releases/latest/download/manifest.json"
	downloadBaseURLVar = "https://github.com/kittylabassistant/sign-craze-dat/releases/latest/download/"
)

var geoHTTPClient = &http.Client{
	Timeout: 10 * time.Minute,
	Transport: &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
	},
}

// ManifestEntry описывает один файл в manifest.json.
type ManifestEntry struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest — корневая структура manifest.json из репозитория sign-craze-dat.
type Manifest struct {
	Version   string          `json:"version"`
	UpdatedAt string          `json:"updated_at"`
	Files     []ManifestEntry `json:"files"`
}

// FetchManifest скачивает manifest.json из релиза sign-craze-dat и парсит его.
func FetchManifest(ctx context.Context) (Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURLVar, nil)
	if err != nil {
		return Manifest{}, fmt.Errorf("geo: формирование запроса manifest: %w", err)
	}
	req.Header.Set("User-Agent", "sign-craze")

	resp, err := geoHTTPClient.Do(req)
	if err != nil {
		return Manifest{}, fmt.Errorf("geo: запрос manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("geo: manifest HTTP %d", resp.StatusCode)
	}

	var m Manifest
	if decErr := json.NewDecoder(resp.Body).Decode(&m); decErr != nil {
		return Manifest{}, fmt.Errorf("geo: декодирование manifest: %w", decErr)
	}
	return m, nil
}

// Update скачивает только те .srs файлы из needed, чей локальный SHA256 не совпадает с манифестом.
// Атомарно записывает файлы в geoDir. Возвращает количество обновлённых файлов.
func Update(ctx context.Context, needed []string, geoDir string) (int, error) {
	if err := os.MkdirAll(geoDir, 0o755); err != nil {
		return 0, fmt.Errorf("geo: создание директории %s: %w", geoDir, err)
	}

	m, err := FetchManifest(ctx)
	if err != nil {
		return 0, err
	}

	// Строим индекс manifest по имени файла.
	index := make(map[string]ManifestEntry, len(m.Files))
	for _, e := range m.Files {
		index[e.Name] = e
	}

	updated := 0
	for _, name := range needed {
		entry, ok := index[name]
		if !ok {
			return updated, fmt.Errorf("geo: файл %q отсутствует в манифесте (версия %s)", name, m.Version)
		}

		localPath := filepath.Join(geoDir, name)
		if localSHA256(localPath) == entry.SHA256 {
			log.L().Debug("geo: файл актуален, пропускаем", "name", name)
			continue
		}

		log.L().Info("geo: скачиваем файл", "name", name, "version", m.Version, "size", entry.Size)
		gotHash, err := streamDownloadAndWrite(ctx, entry.Name, localPath, effectiveLimit(entry.Size))
		if err != nil {
			return updated, fmt.Errorf("geo: загрузка %s: %w", name, err)
		}
		if gotHash != entry.SHA256 {
			_ = os.Remove(localPath)
			return updated, fmt.Errorf("geo: SHA256 не совпадает для %s: ожидался %s, получен %s",
				name, entry.SHA256, gotHash)
		}

		log.L().Info("geo: файл обновлён", "name", name, "sha256", gotHash[:12]+"...")
		updated++
	}

	return updated, nil
}

// streamDownloadAndWrite скачивает .srs потоково в localPath, считая SHA256
// через io.MultiWriter — без буферизации в RAM (safety-fixes #14).
// Возвращает hex SHA256 успешно записанного файла.
//
// limit — максимально допустимый размер ответа (effectiveLimit для известных
// файлов или MaxGeoFileSize для прямых вызовов). Защита от malicious mirror:
// Content-Length > limit отвергается до записи; chunked-стрим режется
// LimitReader+countingReader.
func streamDownloadAndWrite(ctx context.Context, name, localPath string, limit int64) (string, error) {
	url := downloadBaseURLVar + name
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sign-craze")

	resp, err := geoHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d для %s", resp.StatusCode, url)
	}

	if resp.ContentLength > limit {
		return "", fmt.Errorf("geo: размер %s = %d байт превышает лимит %d", name, resp.ContentLength, limit)
	}

	// LimitReader на (limit+1): если прочитано == limit+1 — поток превысил лимит.
	limited := io.LimitReader(resp.Body, limit+1)

	hasher := sha256.New()
	tee := io.TeeReader(limited, hasher)
	cw := &countingReader{r: tee}
	if err := atomicfs.WriteFileAtomicFromReader(localPath, cw, 0o644); err != nil {
		return "", err
	}
	if cw.n > limit {
		_ = os.Remove(localPath)
		return "", fmt.Errorf("geo: %s превысил лимит %d байт (chunked-стрим)", name, limit)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// countingReader подсчитывает прочитанные байты — для проверки лимита поверх
// LimitReader (LimitReader сам по себе не сообщает «достиг лимита»).
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// localSHA256 вычисляет SHA256 файла по пути потоково (без загрузки в RAM).
// Возвращает пустую строку если файл не существует или не читается.
// Использует io.Copy для поддержки .srs файлов 30-100MB на роутере с 128MB RAM.
func localSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

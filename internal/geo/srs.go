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

	// MaxGeoFileSize — лимит на размер одного скачиваемого .srs.
	// 128MB-роутер не должен ловить OOM от malicious mirror, отдающего
	// бесконечный поток или Content-Length 10GB. 50MB — больше любого
	// реалистичного geo-файла (geoip-cn ~10MB, geosite ~5MB).
	MaxGeoFileSize int64 = 50 * 1024 * 1024
)

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

		log.L().Info("geo: скачиваем файл", "name", name, "version", m.Version)
		gotHash, err := streamDownloadAndWrite(ctx, entry.Name, localPath)
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
// Защита от malicious mirror: до начала записи отбрасывается ответ с
// Content-Length > MaxGeoFileSize; chunked-стрим обрезается LimitReader, и
// если поток превысил лимит — возвращается error.
func streamDownloadAndWrite(ctx context.Context, name, localPath string) (string, error) {
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

	if resp.ContentLength > MaxGeoFileSize {
		return "", fmt.Errorf("geo: размер %s = %d байт превышает лимит %d", name, resp.ContentLength, MaxGeoFileSize)
	}

	// LimitReader на (MaxGeoFileSize+1): если read == limit+1 — поток превысил лимит.
	limited := io.LimitReader(resp.Body, MaxGeoFileSize+1)

	hasher := sha256.New()
	tee := io.TeeReader(limited, hasher)
	cw := &countingReader{r: tee}
	if err := atomicfs.WriteFileAtomicFromReader(localPath, cw, 0o644); err != nil {
		return "", err
	}
	if cw.n > MaxGeoFileSize {
		_ = os.Remove(localPath)
		return "", fmt.Errorf("geo: %s превысил лимит %d байт (chunked-стрим)", name, MaxGeoFileSize)
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

// localSHA256 вычисляет SHA256 файла по пути. Возвращает пустую строку если файл не существует.
func localSHA256(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return hashBytes(data)
}

func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

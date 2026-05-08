package ghrelease

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/log"
)

// URLRewriter трансформирует прямой GitHub URL в URL зеркала.
// Возвращает пустую строку если URL не подходит для конкретного зеркала
// (например jsdelivr не поддерживает release-assets) — вызывающий пропускает.
type URLRewriter func(directURL string) string

// directRewriter возвращает URL без изменений (первая попытка — direct).
func directRewriter(u string) string { return u }

// ghProxyRewriter переписывает любой github.com/api.github.com/raw
// URL через https://gh-proxy.com/<full-direct-url>. Прокси сохраняет
// HTTPS, заголовки и тело. Не подходит для не-GH хостов.
func ghProxyRewriter(u string) string {
	switch {
	case strings.HasPrefix(u, "https://github.com/"),
		strings.HasPrefix(u, "https://api.github.com/"),
		strings.HasPrefix(u, "https://raw.githubusercontent.com/"),
		strings.HasPrefix(u, "https://objects.githubusercontent.com/"),
		strings.HasPrefix(u, "https://codeload.github.com/"):
		return "https://gh-proxy.com/" + u
	}
	return ""
}

// jsdelivrRewriter переписывает raw.githubusercontent.com URL в
// cdn.jsdelivr.net/gh/<owner>/<repo>@<ref>/<path>. Не подходит для
// release-assets (cdn.jsdelivr.net их не хостит) и api.github.com —
// для них возвращает "".
func jsdelivrRewriter(u string) string {
	const rawPrefix = "https://raw.githubusercontent.com/"
	if !strings.HasPrefix(u, rawPrefix) {
		return ""
	}
	rest := strings.TrimPrefix(u, rawPrefix)
	parts := strings.SplitN(rest, "/", 4)
	if len(parts) < 4 {
		return ""
	}
	owner, repo, ref, path := parts[0], parts[1], parts[2], parts[3]
	return fmt.Sprintf("https://cdn.jsdelivr.net/gh/%s/%s@%s/%s", owner, repo, ref, path)
}

// DefaultMirrors — порядок попыток при сбое: direct → gh-proxy → jsdelivr.
// Меняется в тестах (см. mirrors_test.go) и в Downloader.Mirrors для
// кастомных setup'ов.
var DefaultMirrors = []URLRewriter{
	directRewriter,
	ghProxyRewriter,
	jsdelivrRewriter,
}

// doWithFallback пытается выполнить req по очереди через каждое из зеркал
// (d.Mirrors или DefaultMirrors если nil). Возвращает первый ответ с
// status < 500 (включая 304 Not Modified — это успех при If-None-Match).
// При network error / 5xx — пробует следующее зеркало.
//
// Не использовать для запросов с req.Body != nil: тело может быть
// прочитано только один раз, retry на следующее зеркало упадёт.
// Текущие callers (releaseMeta, asset download, sha256 verify) — все GET
// без body, ограничение приемлемо.
func (d *Downloader) doWithFallback(req *http.Request) (*http.Response, error) {
	mirrors := d.Mirrors
	if mirrors == nil {
		mirrors = DefaultMirrors
	}
	if req.Body != nil {
		// Не должно случиться в текущем коде; защита от регрессии.
		return nil, fmt.Errorf("ghrelease: doWithFallback не поддерживает запросы с body")
	}

	originalURL := req.URL.String()
	var lastErr error
	for i, rewrite := range mirrors {
		mirrorURL := rewrite(originalURL)
		if mirrorURL == "" {
			continue // зеркало не поддерживает данный URL
		}
		mirrorReq, err := cloneRequestWithURL(req, mirrorURL)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := d.HTTPClient.Do(mirrorReq)
		if err != nil {
			lastErr = fmt.Errorf("mirror[%d] %s: %w", i, hostOf(mirrorURL), err)
			log.L().Warn("ghrelease: mirror недоступен, пробуем следующий",
				"mirror", hostOf(mirrorURL), "err", err)
			continue
		}
		// 5xx — server error, пробуем дальше; 4xx — реальная ошибка ресурса
		// (404 = asset не найден, 401 = rate-limit), без retry.
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("mirror[%d] %s вернул HTTP %d", i, hostOf(mirrorURL), resp.StatusCode)
			log.L().Warn("ghrelease: mirror вернул 5xx, пробуем следующий",
				"mirror", hostOf(mirrorURL), "status", resp.StatusCode)
			continue
		}
		if i > 0 {
			log.L().Info("ghrelease: использовано mirror-зеркало",
				"mirror", hostOf(mirrorURL), "url", originalURL)
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("ghrelease: все зеркала вернули неподдерживаемый URL: %s", originalURL)
	}
	return nil, lastErr
}

func cloneRequestWithURL(req *http.Request, newURL string) (*http.Request, error) {
	out, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	for k, v := range req.Header {
		out.Header[k] = append([]string(nil), v...)
	}
	return out, nil
}

func hostOf(u string) string {
	// Дешёвый парсинг hostname без net/url — берём кусок между "://" и следующим "/".
	rest := strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}

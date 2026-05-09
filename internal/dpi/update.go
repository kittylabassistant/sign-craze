package dpi

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/log"
)

// UpdateReport — итог одного цикла обновления hostlist.
type UpdateReport struct {
	Timestamp     time.Time
	TotalHosts    int
	SourcesOK     int
	SourcesFailed int
	Errors        []string
}

// updateHTTPTimeout — потолок одного download-запроса. На MIPS softfloat
// HTTPS handshake к GitHub занимает 1.5–4s даже с прогретыми ассетами,
// поэтому таймаут — 30s, не дефолтные 5s. Меньше — спорадические ENOTUDP/i/o
// timeout на slow-link cellular IPv6.
const updateHTTPTimeout = 30 * time.Second

// updateMaxBodySize — потолок размера одного hostlist'а. Реалистичные
// upstream-листы — десятки KB; 2 MB защита от мусорных URL и DDoS.
const updateMaxBodySize = 2 * 1024 * 1024

// UpdateHostlist скачивает hostlist'ы из urls, объединяет с extraTargets
// (state.DPITargets — пользовательские локальные домены), убирает дубликаты,
// сортирует и атомарно записывает в dst.
//
// Поведение при ошибках: failure любого url НЕ прерывает обновление — собираем
// что доступно. Если все источники упали И extraTargets пуст — возвращаем
// ошибку (нечего записывать). При частичном успехе пишем то что есть и
// пишем WARN в лог с перечнем фейлов.
//
// Не блокирует на kthreads/pure-go: net/http использует netpoll, контекст
// прерывает зависшие соединения. Caller обеспечивает разумный timeout через ctx.
func UpdateHostlist(ctx context.Context, urls, extraTargets []string, dst string) (UpdateReport, error) {
	report := UpdateReport{Timestamp: time.Now()}
	if dst == "" {
		return report, fmt.Errorf("dpi update: пустой dst")
	}

	client := &http.Client{
		Timeout: updateHTTPTimeout,
		Transport: &http.Transport{
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			DisableKeepAlives:     true,
			ResponseHeaderTimeout: updateHTTPTimeout / 2,
		},
	}

	hosts := make(map[string]struct{}, 1024)
	for _, t := range extraTargets {
		t = strings.TrimSpace(t)
		if t != "" {
			hosts[strings.ToLower(t)] = struct{}{}
		}
	}

	for _, raw := range urls {
		url := strings.TrimSpace(raw)
		if url == "" {
			continue
		}
		fetched, err := fetchHostlist(ctx, client, url)
		if err != nil {
			report.SourcesFailed++
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", url, err))
			log.L().Warn("dpi update: источник недоступен", "url", url, "err", err)
			continue
		}
		report.SourcesOK++
		for _, h := range fetched {
			hosts[h] = struct{}{}
		}
	}

	if len(hosts) == 0 {
		return report, fmt.Errorf("dpi update: пустой результат (источников OK: %d, failed: %d)", report.SourcesOK, report.SourcesFailed)
	}

	sorted := make([]string, 0, len(hosts))
	for h := range hosts {
		sorted = append(sorted, h)
	}
	sort.Strings(sorted)

	if err := WriteHostlist(dst, sorted); err != nil {
		return report, fmt.Errorf("dpi update: запись %s: %w", dst, err)
	}

	report.TotalHosts = len(sorted)
	log.L().Info("dpi update: hostlist обновлён",
		"path", dst,
		"hosts", report.TotalHosts,
		"sources_ok", report.SourcesOK,
		"sources_failed", report.SourcesFailed,
	)
	return report, nil
}

// fetchHostlist скачивает один URL и парсит как hostlist. Каждая строка =
// один домен. Комментарии (#…) и пустые строки игнорируются. Whitespace
// triммируется. Опциональные префиксы вида "0.0.0.0 " (Pi-hole-style) и
// "||" (Adblock-style) тоже срезаются: тогда тот же лист подходит и нам
// и hosts-блокировщикам.
func fetchHostlist(ctx context.Context, client *http.Client, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	req.Header.Set("User-Agent", "sign-craze/dpi-update")
	req.Header.Set("Accept", "text/plain, */*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, updateMaxBodySize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if int64(len(body)) > updateMaxBodySize {
		return nil, fmt.Errorf("body > %d байт", updateMaxBodySize)
	}

	hosts := make([]string, 0, 256)
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 64*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if h := parseHostLine(line); h != "" {
			hosts = append(hosts, h)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return hosts, nil
}

// parseHostLine извлекает хост из одной строки hostlist-формата.
// Поддержанные форматы:
//
//	example.com
//	0.0.0.0 example.com
//	127.0.0.1 example.com  # comment
//	||example.com^
//
// Возвращает lowercase-хост без префиксов/суффиксов, "" для невалидной строки.
func parseHostLine(line string) string {
	if i := strings.IndexByte(line, '#'); i >= 0 {
		line = line[:i]
	}
	if i := strings.IndexByte(line, '!'); i == 0 {
		return "" // Adblock comment
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	// Adblock: ||domain^ или ||domain
	if strings.HasPrefix(line, "||") {
		line = line[2:]
		if i := strings.IndexAny(line, "^/$"); i >= 0 {
			line = line[:i]
		}
	}
	// Pi-hole/hosts: "0.0.0.0 domain" или "127.0.0.1 domain"
	if fields := strings.Fields(line); len(fields) >= 2 {
		first := fields[0]
		if first == "0.0.0.0" || first == "127.0.0.1" || first == "::" {
			line = fields[1]
		} else {
			line = fields[0]
		}
	}
	line = strings.TrimSpace(line)
	line = strings.ToLower(line)
	if line == "" || !looksLikeHostname(line) {
		return ""
	}
	return line
}

// looksLikeHostname — простой sanity-check: содержит хотя бы одну точку
// и состоит из допустимых для DNS символов (буквы, цифры, дефис, точка).
// Без этого в hostlist может попасть мусор (HTML-куски, error pages),
// который nfqws2 примет за валидный hostname pattern и desync ничего не делает.
func looksLikeHostname(s string) bool {
	if !strings.ContainsRune(s, '.') {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '.' || r == '_':
		default:
			return false
		}
	}
	return true
}

// ShouldUpdate возвращает true если последний update был более чем
// intervalHours часов назад. lastUpdate — RFC3339 строка из state.DPILastUpdate.
// Пустая строка/некорректный формат → true (давно/никогда не обновлялся).
// intervalHours <= 0 → false (auto-update выключен).
func ShouldUpdate(lastUpdate string, intervalHours int) bool {
	if intervalHours <= 0 {
		return false
	}
	if lastUpdate == "" {
		return true
	}
	ts, err := time.Parse(time.RFC3339, lastUpdate)
	if err != nil {
		return true
	}
	return time.Since(ts) >= time.Duration(intervalHours)*time.Hour
}

// DefaultUpdateURLs — рекомендованный стартовый набор источников.
// bol-van/zapret — апстрим nfqws/zapret (MIT), files обновляются эпизодически.
// flowseal/zapret-discord-youtube — community-листы для актуальных DPI-блоков
// Discord+YouTube в РФ, обновляются регулярно.
// Эти URL показываются юзеру в --help/install как пример; реальная подписка —
// через `--dpi-update-urls <urls>`.
var DefaultUpdateURLs = []string{
	"https://raw.githubusercontent.com/bol-van/zapret/master/ipset/zapret-hosts-user.txt.example",
	"https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-youtube.txt",
	"https://raw.githubusercontent.com/Flowseal/zapret-discord-youtube/main/lists/list-discord.txt",
}

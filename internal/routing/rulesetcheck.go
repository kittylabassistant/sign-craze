package routing

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// srsMagic — сигнатура компилированного sing-box rule-set (бинарный .srs):
// первые 3 байта файла — ASCII "SRS" (0x53 0x52 0x53).
//
// Инцидент 2026-05-12 (tasks/lessons.md): rule_set был объявлен как
// format=binary/.srs, но фактическое содержимое оказалось JSON source
// ({"version": 2, "rules": [...]}) — sing-box падал на старте с
// "invalid sing-box rule-set file". Эта проверка ловит именно такой mismatch
// до сохранения конфига.
var srsMagic = []byte("SRS")

// magicProbeBytes — сколько байт тела читаем для сверки сигнатуры формата.
// С запасом относительно "SRS" (3 байта) и "{" (1 байт); rule-set файлы
// весят от сотен КБ до десятков МБ — целиком их читать нельзя.
const magicProbeBytes = 16

// ruleSetCheckUserAgent — User-Agent для HEAD/GET-запросов проверки (тот же
// стиль, что и в internal/dpi/update.go: "sign-craze/<компонент>").
const ruleSetCheckUserAgent = "sign-craze/rule-set-check"

// Result — результат проверки одного rule_set.URL.
type Result struct {
	// OK — true, если URL прошёл проверку: сервер ответил без 404/5xx и (если
	// формат распознан по ref.Format/расширению) magic-байты тела совпадают с
	// ожидаемыми. Также true, если формат не распознан — тогда сверка
	// содержимого сознательно пропускается.
	OK bool

	// Mismatch — true при подтверждённой проблеме: HTTP 404/5xx, либо
	// magic-байты тела не совпадают с ожидаемым форматом (например, JSON
	// source под .srs-URL — ровно кейс инцидента 2026-05-12).
	Mismatch bool

	// Unreachable — true при сетевой ошибке (DNS, timeout, connection
	// refused и т.п.). Намеренно отдельная категория от Mismatch: недоступность
	// сети (offline-роутер) — не ошибка конфигурации rule_set, вызывающий код
	// не должен трактовать её как повод блокировать сохранение/запуск.
	Unreachable bool

	// Detail — человекочитаемое описание результата на русском (для diag/web).
	Detail string
}

// CheckRuleSetURL проверяет rule_set.URL: доступность (HTTP HEAD, fallback на
// GET с Range: bytes=0-N, если HEAD не поддержан сервером — 405/501) и
// соответствие формата тела ожидаемому (ref.Format с fallback на расширение
// URL): "binary"/".srs" → ожидаем сигнатуру "SRS", "source"/".json" → ожидаем
// тело, начинающееся с '{'. Остальные форматы (например, mihomo .mrs) по
// содержимому не проверяются — намеренно, чтобы не блокировать форматы,
// которые sign-craze не умеет валидировать (OK=true, сверка пропущена).
//
// Тело не загружается целиком: читаются только первые magicProbeBytes байт
// через io.LimitReader — rule-set файлы могут весить десятки МБ.
//
// client может быть nil (используется http.DefaultClient). Таймаут не
// устанавливается функцией — это ответственность ctx: diag использует общий
// бюджет на все rule_set сразу, web — короткий таймаут на одну проверку при
// добавлении через UI.
func CheckRuleSetURL(ctx context.Context, client *http.Client, ref types.RuleSetRef) Result {
	if ref.URL == "" {
		return Result{OK: true, Detail: "url не задан — проверка пропущена"}
	}
	if client == nil {
		client = http.DefaultClient
	}

	status, body, err := probeStatusAndBody(ctx, client, ref.URL)
	if err != nil {
		return Result{Unreachable: true, Detail: fmt.Sprintf("сеть недоступна: %v", err)}
	}
	if detail, bad := statusProblem(status); bad {
		return Result{Mismatch: true, Detail: detail}
	}

	if body == nil {
		// HEAD не отдаёт тело — отдельным запросом читаем первые байты для magic-сверки.
		body, err = rangeGetBytes(ctx, client, ref.URL)
		if err != nil {
			return Result{Unreachable: true, Detail: fmt.Sprintf("сеть недоступна при чтении содержимого: %v", err)}
		}
	}

	return matchMagic(ref, body)
}

// probeStatusAndBody выполняет HEAD-запрос. Если сервер не поддерживает HEAD
// (405 Method Not Allowed / 501 Not Implemented) — делает fallback на GET с
// Range и возвращает сразу и статус, и прочитанное тело (экономим второй
// запрос). При успешном HEAD тело всегда nil — вызывающий код запрашивает его
// отдельно через rangeGetBytes.
func probeStatusAndBody(ctx context.Context, client *http.Client, url string) (status int, body []byte, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("HEAD %s: %w", url, err)
	}
	req.Header.Set("User-Agent", ruleSetCheckUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("HEAD %s: %w", url, err)
	}
	resp.Body.Close() // HEAD: тела в ответе нет, закрываем сразу

	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotImplemented {
		return resp.StatusCode, nil, nil
	}

	b, getStatus, getErr := rangeGet(ctx, client, url)
	if getErr != nil {
		return 0, nil, getErr
	}
	return getStatus, b, nil
}

// rangeGetBytes — GET с Range, статус отбрасывается (используется только
// когда статус уже подтверждён предыдущим успешным HEAD).
func rangeGetBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	b, _, err := rangeGet(ctx, client, url)
	return b, err
}

// rangeGet выполняет GET с заголовком Range: bytes=0-<magicProbeBytes-1> и
// читает не больше magicProbeBytes байт тела через io.LimitReader —
// независимо от того, уважает ли сервер Range (некоторые отдают файл целиком,
// это нормально: лишнее просто не будет прочитано).
func rangeGet(ctx context.Context, client *http.Client, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("GET %s: %w", url, err)
	}
	req.Header.Set("User-Agent", ruleSetCheckUserAgent)
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", magicProbeBytes-1))

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, magicProbeBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("чтение тела %s: %w", url, err)
	}
	return data, resp.StatusCode, nil
}

// statusProblem подтверждённой проблемой считает только 404 и 5xx — это то,
// что реально ломает sing-box на старте (инцидент 2026-05-12: "unexpected
// status: 404 Not Found"). Остальные коды (включая прочие 4xx) не блокируют
// последующую magic-сверку.
func statusProblem(status int) (detail string, bad bool) {
	switch {
	case status == http.StatusNotFound:
		return "HTTP 404 Not Found", true
	case status >= http.StatusInternalServerError:
		return fmt.Sprintf("сервер вернул HTTP %d", status), true
	default:
		return "", false
	}
}

// expectedMagicFormat возвращает "srs" | "source" | "" (формат не распознан)
// по ref.Format с fallback на расширение URL. ref.Format имеет приоритет:
// это явное намерение (как rule_set фактически объявлен), расширение —
// эвристика на случай, если Format не задан.
func expectedMagicFormat(ref types.RuleSetRef) string {
	switch ref.Format {
	case "binary":
		return "srs"
	case "source":
		return "source"
	}
	switch {
	case strings.HasSuffix(ref.URL, ".srs"):
		return "srs"
	case strings.HasSuffix(ref.URL, ".json"):
		return "source"
	default:
		return ""
	}
}

// matchMagic сверяет первые байты тела с ожидаемой сигнатурой формата.
func matchMagic(ref types.RuleSetRef, body []byte) Result {
	switch expectedMagicFormat(ref) {
	case "srs":
		if bytes.HasPrefix(body, srsMagic) {
			return Result{OK: true, Detail: "OK: compiled rule-set (сигнатура SRS)"}
		}
		return Result{Mismatch: true, Detail: fmt.Sprintf(
			"ожидался compiled rule-set (сигнатура %q), получено %q — похоже на JSON source под .srs расширением",
			srsMagic, previewBytes(body))}
	case "source":
		if len(body) > 0 && body[0] == '{' {
			return Result{OK: true, Detail: "OK: JSON source rule-set"}
		}
		return Result{Mismatch: true, Detail: fmt.Sprintf(
			"ожидался JSON source (тело начинается с '{'), получено %q", previewBytes(body))}
	default:
		return Result{OK: true, Detail: "формат не распознан (не .srs/.json) — проверка сигнатуры пропущена"}
	}
}

// previewBytes возвращает укороченный фрагмент тела для сообщений об ошибке
// (безопасно для непечатаемых байт — используется с verb %q на вызывающей стороне).
func previewBytes(body []byte) []byte {
	const n = 8
	if len(body) > n {
		return body[:n]
	}
	return body
}

package ndm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL — стандартный endpoint RCI на роутере Keenetic.
const DefaultBaseURL = "http://127.0.0.1:79/rci"

// DefaultTimeout — таймаут одной HTTP-операции с RCI.
const DefaultTimeout = 10 * time.Second

// ErrNotFound возвращается, если запрашиваемый объект (policy, interface)
// отсутствует в RCI.
var ErrNotFound = errors.New("ndm: объект не найден")

// Client — HTTP-клиент Keenetic RCI.
type Client struct {
	baseURL string
	hc      *http.Client
}

// NewClient создаёт клиент RCI с дефолтными baseURL и таймаутом.
func NewClient() *Client {
	return newClientWithBase(DefaultBaseURL, DefaultTimeout)
}

// newClientWithBase позволяет указать кастомный baseURL (для тестов через httptest).
// Таймаут применяется per-request, контекст вызывающего может быть короче.
func newClientWithBase(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		hc:      &http.Client{Timeout: timeout},
	}
}

// rciStatusEntry — одна запись status[] в ответе RCI.
type rciStatusEntry struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Ident   string `json:"ident"`
	Message string `json:"message"`
}

// Get выполняет GET <baseURL><path> и парсит JSON-ответ в out.
// Path должен начинаться со слеша, например "/show/ip/policy".
func (c *Client) Get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("ndm: GET %s: %w", path, err)
	}
	body, err := c.do(req)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("ndm: парсинг GET %s: %w", path, err)
	}
	return nil
}

// PostJSON отправляет POST с телом в виде map[string]any (формат RCI: вложенные объекты).
// Возвращает сырой ответ для пост-обработки вызывающим (RCI кладёт status[]
// в разных местах в зависимости от endpoint'а).
func (c *Client) PostJSON(ctx context.Context, path string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ndm: marshal POST %s: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ndm: POST %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

// do выполняет HTTP-запрос и возвращает тело ответа. Ошибки ниже-протокольного
// уровня (5xx) маппятся в ошибку Go; 4xx возвращают тело и nil — пусть
// вызывающий решает, является ли это нормой (например, "policy not found").
func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ndm: %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ndm: чтение ответа %s: %w", req.URL.Path, err)
	}
	if resp.StatusCode >= 500 {
		return body, fmt.Errorf("ndm: %s %s вернул %d: %s", req.Method, req.URL.Path, resp.StatusCode, string(body))
	}
	return body, nil
}

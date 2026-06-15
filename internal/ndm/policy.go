package ndm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// PolicyInfo — runtime-state IP Policy из /rci/show/ip/policy.
//
// Mark и Table4/Table6 присваиваются Keenetic'ом автоматически при создании
// policy. Mark инкрементальный (XKeen=ffffaaa, sign-craze=ffffaab, ...).
type PolicyInfo struct {
	Name        string   // ключ в JSON, e.g. "sign-craze"
	Description string   // user-facing description, тот же что Name по конвенции sign-craze
	Mark        uint32   // парсится из hex-строки "ffffaab" → 0x0FFFFAAB
	Table4      int      // routing table IPv4
	Table6      int      // routing table IPv6
	Permit      []string // список разрешённых WAN-интерфейсов (только enabled=true)
	Multipath   bool
}

// rawPolicyShow — формат ответа /rci/show/ip/policy.
type rawPolicyShow struct {
	Description string `json:"description"`
	Mark        string `json:"mark"` // hex-строка без 0x
	Table4      int    `json:"table4"`
	Table6      int    `json:"table6"`
}

// rawPolicyRC — формат ответа /rci/show/rc/ip/policy.
type rawPolicyRC struct {
	Description string `json:"description"`
	Permit      []struct {
		Enabled   bool   `json:"enabled"`
		No        bool   `json:"no"`
		Interface string `json:"interface"`
	} `json:"permit"`
	Multipath bool `json:"multipath"`
}

// GetPolicy ищет policy по ключу (не по description) в /rci/show/ip/policy.
// Если policy ещё не получила mark от Keenetic (асинхронное присвоение после
// create), возвращает ErrPolicyMarkPending. Вызывающему стоит повторить позже.
func (c *Client) GetPolicy(ctx context.Context, name string) (*PolicyInfo, error) {
	var showMap map[string]rawPolicyShow
	if err := c.Get(ctx, "/show/ip/policy", &showMap); err != nil {
		return nil, err
	}
	raw, ok := showMap[name]
	if !ok {
		return nil, ErrNotFound
	}
	mark, err := parseMarkHex(raw.Mark)
	if err != nil {
		return nil, fmt.Errorf("ndm: некорректный mark %q для policy %q: %w", raw.Mark, name, err)
	}

	// Permit и multipath — в running-config endpoint'е.
	var rcMap map[string]rawPolicyRC
	if err := c.Get(ctx, "/show/rc/ip/policy", &rcMap); err != nil {
		return nil, err
	}
	rc, ok := rcMap[name]
	if !ok {
		return nil, fmt.Errorf("ndm: policy %q есть в /show, но отсутствует в /show/rc", name)
	}

	permit := make([]string, 0, len(rc.Permit))
	for _, p := range rc.Permit {
		if p.Enabled && !p.No {
			permit = append(permit, p.Interface)
		}
	}

	return &PolicyInfo{
		Name:        name,
		Description: raw.Description,
		Mark:        mark,
		Table4:      raw.Table4,
		Table6:      raw.Table6,
		Permit:      permit,
		Multipath:   rc.Multipath,
	}, nil
}

// parseMarkHex парсит hex-строку Keenetic-mark (без 0x): "ffffaab" → 0x0FFFFAAB.
// Пустая строка возвращает ErrPolicyMarkPending — Keenetic ещё не присвоил mark.
func parseMarkHex(s string) (uint32, error) {
	if s == "" {
		return 0, ErrPolicyMarkPending
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}

// ErrPolicyMarkPending — Keenetic ещё не присвоил mark policy
// (асинхронное присвоение после create, обычно <5s).
var ErrPolicyMarkPending = errors.New("ndm: policy mark ещё не присвоен Keenetic'ом")

// CreatePolicy создаёт IP-policy в Keenetic через RCI POST.
//
// Тело запроса (формат подтверждён эмпирически):
//
//	{"ip":{"policy":{"<name>":{"description":"<desc>","permit":{"interface":"<iface>"}}}}}
//
// После создания Keenetic асинхронно присваивает mark/table — вызывающий
// должен дождаться через waitForMark.
func (c *Client) CreatePolicy(ctx context.Context, name, description, wanIface string) error {
	if name == "" || wanIface == "" {
		return fmt.Errorf("ndm: CreatePolicy: name и wanIface не могут быть пустыми")
	}
	payload := map[string]any{
		"ip": map[string]any{
			"policy": map[string]any{
				name: map[string]any{
					"description": description,
					"permit": map[string]any{
						"interface": wanIface,
					},
				},
			},
		},
	}
	body, err := c.PostJSON(ctx, "/", payload)
	if err != nil {
		return err
	}
	if err := checkPolicyCreateStatus(body, name); err != nil {
		return err
	}
	return nil
}

// checkPolicyCreateStatus парсит вложенный status[] из ответа создания policy.
// RCI возвращает структуру вида
// {"ip":{"policy":{"<name>":{"status":[...], "description":{"status":[...]}, "permit":{"status":[...]}}}}}.
// Любой "status":"error" внутри — ошибка.
func checkPolicyCreateStatus(body []byte, name string) error {
	var resp struct {
		IP struct {
			Policy map[string]json.RawMessage `json:"policy"`
		} `json:"ip"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("ndm: парсинг ответа CreatePolicy: %w (body: %s)", err, string(body))
	}
	raw, ok := resp.IP.Policy[name]
	if !ok {
		return fmt.Errorf("ndm: ответ CreatePolicy не содержит policy %q (body: %s)", name, string(body))
	}
	// raw может быть как объектом, содержащим вложенные status-массивы, так и просто map.
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return fmt.Errorf("ndm: парсинг nested-status %q: %w", name, err)
	}
	for key, val := range nested {
		var entries []rciStatusEntry
		if err := tryUnmarshalStatuses(val, &entries); err != nil {
			// Не статус-массив, пропускаем.
			continue
		}
		for _, e := range entries {
			if e.Status == "error" {
				return fmt.Errorf("ndm: CreatePolicy %q.%s: %s [%s]", name, key, e.Message, e.Code)
			}
		}
	}
	return nil
}

// tryUnmarshalStatuses пытается извлечь status[] из произвольного JSON-куска,
// который может быть как объектом со status, так и просто массивом status.
func tryUnmarshalStatuses(raw json.RawMessage, out *[]rciStatusEntry) error {
	var wrapper struct {
		Status []rciStatusEntry `json:"status"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Status) > 0 {
		*out = wrapper.Status
		return nil
	}
	return json.Unmarshal(raw, out)
}

// EnsurePolicy идемпотентно гарантирует, что policy с заданным именем существует
// и привязана к указанному WAN-интерфейсу. Возвращает PolicyInfo с актуальным
// mark (ждёт асинхронного присвоения до 5s).
func (c *Client) EnsurePolicy(ctx context.Context, name, description, wanIface string) (*PolicyInfo, error) {
	existing, err := c.GetPolicy(ctx, name)
	if err == nil {
		// Уже есть. Проверим, что WAN-интерфейс совпадает; если нет — пересоздаём.
		if hasInterface(existing.Permit, wanIface) {
			return existing, nil
		}
		// Mismatch permit: пересоздаём (delete + create).
		if delErr := c.DeletePolicy(ctx, name); delErr != nil {
			return nil, fmt.Errorf("ndm: EnsurePolicy: пересоздание (delete): %w", delErr)
		}
	} else if !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrPolicyMarkPending) {
		return nil, err
	}

	if err := c.CreatePolicy(ctx, name, description, wanIface); err != nil {
		return nil, err
	}
	return c.waitForMark(ctx, name, 5*time.Second)
}

// waitForMark опрашивает GetPolicy до тех пор, пока Keenetic не присвоит mark,
// либо не истечёт timeout. Используется после CreatePolicy.
func (c *Client) waitForMark(ctx context.Context, name string, timeout time.Duration) (*PolicyInfo, error) {
	deadline := time.Now().Add(timeout)
	for {
		info, err := c.GetPolicy(ctx, name)
		if err == nil {
			return info, nil
		}
		if !errors.Is(err, ErrPolicyMarkPending) && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("ndm: ожидание mark для policy %q превысило %s: %w", name, timeout, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// DeletePolicy удаляет policy через POST с {"no": true}.
func (c *Client) DeletePolicy(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("ndm: DeletePolicy: name не может быть пустым")
	}
	payload := map[string]any{
		"ip": map[string]any{
			"policy": map[string]any{
				name: map[string]any{
					"no": true,
				},
			},
		},
	}
	if _, err := c.PostJSON(ctx, "/", payload); err != nil {
		return err
	}
	return nil
}

// SaveConfig вызывает /rci/system/configuration/save.
// Без этого изменения в running-config не переживут перезагрузку роутера.
func (c *Client) SaveConfig(ctx context.Context) error {
	if _, err := c.PostJSON(ctx, "/system/configuration/save", map[string]any{}); err != nil {
		return err
	}
	return nil
}

func hasInterface(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// rawHotspotHostRC — формат хоста в /rci/show/rc/ip/hotspot.
// Поле policy может отсутствовать (omitempty) — значит хост без привязки.
type rawHotspotHostRC struct {
	Mac    string `json:"mac"`
	Policy string `json:"policy,omitempty"`
}

type rawHotspotRC struct {
	Host []rawHotspotHostRC `json:"host"`
}

// GetHostsWithPolicy возвращает MAC-адреса хостов, привязанных к policy.
// Используется в --uninstall для отвязки перед DeletePolicy.
// Если эндпоинт недоступен на этой прошивке — возвращает ошибку, caller
// должен залогировать WARN и продолжить (uninstall не блокируется).
func (c *Client) GetHostsWithPolicy(ctx context.Context, policyName string) ([]string, error) {
	var rc rawHotspotRC
	if err := c.Get(ctx, "/show/rc/ip/hotspot", &rc); err != nil {
		return nil, fmt.Errorf("ndm: GetHostsWithPolicy: %w", err)
	}
	var macs []string
	for _, h := range rc.Host {
		if h.Policy == policyName {
			macs = append(macs, h.Mac)
		}
	}
	return macs, nil
}

// UnsetHostPolicy снимает policy-привязку с устройства.
// RCI: POST / с {"ip":{"hotspot":{"host":{"mac":"<MAC>","policy":{"no":true}}}}}.
// Идемпотентно: Keenetic возвращает OK даже если хост не привязан.
func (c *Client) UnsetHostPolicy(ctx context.Context, mac string) error {
	if mac == "" {
		return fmt.Errorf("ndm: UnsetHostPolicy: mac пустой")
	}
	payload := map[string]any{
		"ip": map[string]any{
			"hotspot": map[string]any{
				"host": map[string]any{
					"mac": mac,
					"policy": map[string]any{
						"no": true,
					},
				},
			},
		},
	}
	if _, err := c.PostJSON(ctx, "/", payload); err != nil {
		return fmt.Errorf("ndm: UnsetHostPolicy(%s): %w", mac, err)
	}
	return nil
}

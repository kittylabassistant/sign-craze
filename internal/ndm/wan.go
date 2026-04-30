package ndm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// rawIPRouteShow — формат /rci/show/ip/route.
// Ключи на верхнем уровне неизвестны заранее (объект с полями destination/gateway/...).
// Используем явный slice через map → []entry.
type rawIPRouteEntry struct {
	Destination string `json:"destination"`
	Interface   string `json:"interface"`
	Default     bool   `json:"default,omitempty"`
}

// DetectWANInterface возвращает имя Keenetic-интерфейса с маршрутом по умолчанию
// (например "GigabitEthernet1"). Используется при создании policy для permit interface.
//
// Источник истины — RCI, не linux-имя интерфейса. У Keenetic eth3 в linux
// соответствует GigabitEthernet1 в RCI.
func (c *Client) DetectWANInterface(ctx context.Context) (string, error) {
	body, err := c.PostJSON(ctx, "/show/ip/route", map[string]any{})
	if err != nil {
		// Fallback на GET (некоторые RCI endpoints не принимают POST).
		var routes []rawIPRouteEntry
		if errGet := c.Get(ctx, "/show/ip/route", &routes); errGet == nil {
			return findDefaultRoute(routes)
		}
		return "", err
	}
	// Тело может быть объектом {default: ...} или массивом — пробуем оба формата.
	var asArray []rawIPRouteEntry
	if jerr := json.Unmarshal(body, &asArray); jerr == nil && len(asArray) > 0 {
		return findDefaultRoute(asArray)
	}
	var asWrap struct {
		Routes []rawIPRouteEntry `json:"routes"`
	}
	if jerr := json.Unmarshal(body, &asWrap); jerr == nil && len(asWrap.Routes) > 0 {
		return findDefaultRoute(asWrap.Routes)
	}
	return "", fmt.Errorf("ndm: не удалось распарсить /show/ip/route: %s", string(body))
}

func findDefaultRoute(entries []rawIPRouteEntry) (string, error) {
	for _, e := range entries {
		if e.Default || e.Destination == "0.0.0.0/0" || strings.HasSuffix(e.Destination, "/0") {
			if e.Interface != "" {
				return e.Interface, nil
			}
		}
	}
	return "", errors.New("ndm: маршрут по умолчанию не найден в /show/ip/route")
}

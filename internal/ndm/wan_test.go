package ndm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestDetectWANInterface_RealKeeneticResponse — реальный формат
// Keenetic /rci/show/ip/route: top-level ключ "route" (singular),
// массив объектов с destination/gateway/interface.
// Регрессия: прошлый парсер ждал "routes" (plural) и падал.
func TestDetectWANInterface_RealKeeneticResponse(t *testing.T) {
	body := `{
	  "route": [
	    {
	      "destination": "0.0.0.0/0",
	      "gateway": "192.168.1.254",
	      "interface": "GigabitEthernet1"
	    },
	    {
	      "destination": "10.1.30.0/24",
	      "gateway": "0.0.0.0",
	      "interface": "Bridge1"
	    }
	  ]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/show/ip/route") {
			t.Errorf("неожиданный путь: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	c := newClientWithBase(srv.URL, 5*time.Second)
	iface, err := c.DetectWANInterface(context.Background())
	if err != nil {
		t.Fatalf("DetectWANInterface: %v", err)
	}
	if iface != "GigabitEthernet1" {
		t.Errorf("interface = %q, ожидалось GigabitEthernet1", iface)
	}
}

// TestDetectWANInterface_PluralRoutes — fallback на ключ "routes" (plural)
// для совместимости с моками/устаревшими прошивками.
func TestDetectWANInterface_PluralRoutes(t *testing.T) {
	body := `{"routes":[{"destination":"0.0.0.0/0","interface":"PPPoE0"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	c := newClientWithBase(srv.URL, 5*time.Second)
	iface, err := c.DetectWANInterface(context.Background())
	if err != nil {
		t.Fatalf("DetectWANInterface: %v", err)
	}
	if iface != "PPPoE0" {
		t.Errorf("interface = %q, ожидалось PPPoE0", iface)
	}
}

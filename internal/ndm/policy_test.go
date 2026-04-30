package ndm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fixtureSignCrazeShow — минимальный валидный ответ /rci/show/ip/policy
// с двумя policy. Mark берётся из реального дампа Keenetic Ultra.
const fixtureSignCrazeShow = `{
  "Policy0": {
    "description": "XKeen",
    "mark": "ffffaaa",
    "table4": 4096,
    "table6": 4096
  },
  "sign-craze": {
    "description": "sign-craze",
    "mark": "ffffaab",
    "table4": 4098,
    "table6": 4098
  }
}`

const fixtureSignCrazeRC = `{
  "Policy0": {
    "description": "XKeen",
    "permit": [{"enabled": true, "interface": "GigabitEthernet1"}],
    "multipath": true
  },
  "sign-craze": {
    "description": "sign-craze",
    "permit": [
      {"enabled": true, "interface": "GigabitEthernet1"},
      {"no": true, "enabled": false, "interface": "UsbDsl0"}
    ],
    "multipath": true
  }
}`

func TestParseMarkHex(t *testing.T) {
	cases := []struct {
		in      string
		want    uint32
		wantErr error
	}{
		{"ffffaab", 0x0FFFFAAB, nil},
		{"ffffaaa", 0x0FFFFAAA, nil},
		{"53", 0x53, nil},
		{"", 0, ErrPolicyMarkPending},
	}
	for _, c := range cases {
		got, err := parseMarkHex(c.in)
		if c.wantErr != nil {
			if !errors.Is(err, c.wantErr) {
				t.Errorf("parseMarkHex(%q) err = %v, ожидалось %v", c.in, err, c.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMarkHex(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseMarkHex(%q) = 0x%x, ожидалось 0x%x", c.in, got, c.want)
		}
	}
}

func newMockServer(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClientWithBase(srv.URL, 2*time.Second), srv
}

func TestGetPolicy_ParsesMarkAndPermit(t *testing.T) {
	c, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/show/ip/policy":
			_, _ = io.WriteString(w, fixtureSignCrazeShow)
		case "/show/rc/ip/policy":
			_, _ = io.WriteString(w, fixtureSignCrazeRC)
		default:
			http.NotFound(w, r)
		}
	})

	info, err := c.GetPolicy(context.Background(), "sign-craze")
	if err != nil {
		t.Fatalf("GetPolicy: %v", err)
	}
	if info.Mark != 0x0FFFFAAB {
		t.Errorf("Mark = 0x%x, ожидалось 0x0FFFFAAB", info.Mark)
	}
	if info.Table4 != 4098 {
		t.Errorf("Table4 = %d, ожидалось 4098", info.Table4)
	}
	if len(info.Permit) != 1 || info.Permit[0] != "GigabitEthernet1" {
		t.Errorf("Permit = %v, ожидалось [GigabitEthernet1]", info.Permit)
	}
	if !info.Multipath {
		t.Error("Multipath должен быть true")
	}
}

func TestGetPolicy_NotFound(t *testing.T) {
	c, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	})
	_, err := c.GetPolicy(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ожидалась ErrNotFound, получено %v", err)
	}
}

func TestGetPolicy_MarkPending(t *testing.T) {
	c, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Mark пустой → Keenetic ещё не присвоил.
		_, _ = io.WriteString(w, `{"sign-craze":{"description":"sign-craze","mark":"","table4":0,"table6":0}}`)
	})
	_, err := c.GetPolicy(context.Background(), "sign-craze")
	if !errors.Is(err, ErrPolicyMarkPending) {
		t.Errorf("ожидалась ErrPolicyMarkPending, получено %v", err)
	}
}

func TestCreatePolicy_PostsCorrectBody(t *testing.T) {
	var receivedBody map[string]any
	c, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, ожидался POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		// Эмулируем нормальный ответ Keenetic после create.
		_, _ = io.WriteString(w, `{
  "ip": {
    "policy": {
      "sign-craze": {
        "status": [{"status":"message","code":"25362433","message":"created"}],
        "description": {"status":[{"status":"message","code":"25362443","message":"updated"}]},
        "permit": {"status":[{"status":"message","code":"25362472","message":"set"}]}
      }
    }
  }
}`)
	})

	if err := c.CreatePolicy(context.Background(), "sign-craze", "sign-craze", "GigabitEthernet1"); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}

	// Проверяем структуру POST body.
	ip := receivedBody["ip"].(map[string]any)
	policy := ip["policy"].(map[string]any)
	body := policy["sign-craze"].(map[string]any)
	if body["description"] != "sign-craze" {
		t.Errorf("description = %v", body["description"])
	}
	permit := body["permit"].(map[string]any)
	if permit["interface"] != "GigabitEthernet1" {
		t.Errorf("permit.interface = %v", permit["interface"])
	}
}

func TestCreatePolicy_ErrorStatus(t *testing.T) {
	c, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
  "ip": {
    "policy": {
      "sign-craze": {
        "status": [{"status":"error","code":"99999999","message":"forbidden"}]
      }
    }
  }
}`)
	})
	err := c.CreatePolicy(context.Background(), "sign-craze", "sign-craze", "GigabitEthernet1")
	if err == nil {
		t.Fatal("ожидалась ошибка от status:error")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("ошибка не содержит сообщение из status: %v", err)
	}
}

func TestEnsurePolicy_NewCreate(t *testing.T) {
	created := false
	markPresent := false
	c, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/show/ip/policy":
			if !markPresent {
				_, _ = io.WriteString(w, `{}`)
			} else {
				_, _ = io.WriteString(w, fixtureSignCrazeShow)
			}
		case "/show/rc/ip/policy":
			_, _ = io.WriteString(w, fixtureSignCrazeRC)
		case "/":
			created = true
			markPresent = true
			_, _ = io.WriteString(w, `{"ip":{"policy":{"sign-craze":{"status":[{"status":"message","message":"ok"}]}}}}`)
		}
	})

	info, err := c.EnsurePolicy(context.Background(), "sign-craze", "sign-craze", "GigabitEthernet1")
	if err != nil {
		t.Fatalf("EnsurePolicy: %v", err)
	}
	if !created {
		t.Error("CreatePolicy не был вызван")
	}
	if info.Mark != 0x0FFFFAAB {
		t.Errorf("Mark = 0x%x, ожидалось 0x0FFFFAAB", info.Mark)
	}
}

func TestEnsurePolicy_AlreadyExists_NoRecreate(t *testing.T) {
	posts := 0
	c, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/show/ip/policy":
			_, _ = io.WriteString(w, fixtureSignCrazeShow)
		case "/show/rc/ip/policy":
			_, _ = io.WriteString(w, fixtureSignCrazeRC)
		case "/":
			posts++
			_, _ = io.WriteString(w, `{}`)
		}
	})

	info, err := c.EnsurePolicy(context.Background(), "sign-craze", "sign-craze", "GigabitEthernet1")
	if err != nil {
		t.Fatalf("EnsurePolicy: %v", err)
	}
	if posts != 0 {
		t.Errorf("EnsurePolicy выполнил %d POST'ов на существующей policy, ожидалось 0", posts)
	}
	if info.Mark != 0x0FFFFAAB {
		t.Errorf("Mark = 0x%x", info.Mark)
	}
}

func TestDeletePolicy_PostsNoTrue(t *testing.T) {
	var got map[string]any
	c, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.DeletePolicy(context.Background(), "sign-craze"); err != nil {
		t.Fatalf("DeletePolicy: %v", err)
	}
	policy := got["ip"].(map[string]any)["policy"].(map[string]any)
	if v := policy["sign-craze"].(map[string]any)["no"]; v != true {
		t.Errorf("no = %v, ожидалось true", v)
	}
}

func TestSaveConfig_Posts(t *testing.T) {
	called := false
	c, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/system/configuration/save" {
			called = true
		}
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.SaveConfig(context.Background()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if !called {
		t.Error("SaveConfig не вызвал /system/configuration/save")
	}
}

func TestWaitForMark_PendingThenReady(t *testing.T) {
	calls := 0
	c, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/show/ip/policy":
			calls++
			if calls < 2 {
				_, _ = io.WriteString(w, `{"sign-craze":{"description":"sign-craze","mark":"","table4":0,"table6":0}}`)
			} else {
				_, _ = io.WriteString(w, fixtureSignCrazeShow)
			}
		case "/show/rc/ip/policy":
			_, _ = io.WriteString(w, fixtureSignCrazeRC)
		}
	})

	info, err := c.WaitForMark(context.Background(), "sign-craze", 2*time.Second)
	if err != nil {
		t.Fatalf("WaitForMark: %v", err)
	}
	if info.Mark != 0x0FFFFAAB {
		t.Errorf("Mark = 0x%x", info.Mark)
	}
	if calls < 2 {
		t.Errorf("ожидалось >= 2 опросов GetPolicy, было %d", calls)
	}
}

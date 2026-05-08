package netif

import (
	"context"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

func TestParseDefaultRouteIface(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "Keenetic ppp0",
			input: "default via 10.0.0.1 dev ppp0 proto static\n",
			want:  "ppp0",
		},
		{
			name:  "ethX с metric",
			input: "default via 192.168.1.1 dev eth3 proto dhcp src 192.168.1.10 metric 100\n",
			want:  "eth3",
		},
		{
			name:    "пустой ввод",
			input:   "",
			wantErr: true,
		},
		{
			name:    "без dev",
			input:   "default via 10.0.0.1\n",
			wantErr: true,
		},
		{
			name:  "несколько строк, берём первую с dev",
			input: "garbage\ndefault via 10.0.0.1 dev wg0\ndefault via 10.0.0.2 dev eth0\n",
			want:  "wg0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDefaultRouteIface(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestDetectWANIface_MockRunner(t *testing.T) {
	runner := exectx.MockMatcher(func(name string, args ...string) (exectx.Result, error) {
		if name != "ip" || len(args) < 3 {
			t.Fatalf("неожиданный вызов: %s %v", name, args)
		}
		return exectx.Result{
			Stdout: []byte("default via 192.168.1.1 dev br-wan proto static\n"),
		}, nil
	})
	got, err := DetectWANIface(context.Background(), runner)
	if err != nil {
		t.Fatalf("DetectWANIface: %v", err)
	}
	if got != "br-wan" {
		t.Fatalf("got=%q want=br-wan", got)
	}
}

func TestDetectWANIface_RunnerError(t *testing.T) {
	runner := exectx.MockMatcher(func(name string, args ...string) (exectx.Result, error) {
		return exectx.Result{ExitCode: 1}, &mockErr{msg: "ip not found"}
	})
	_, err := DetectWANIface(context.Background(), runner)
	if err == nil || !strings.Contains(err.Error(), "ip route") {
		t.Fatalf("ожидалась ошибка с 'ip route', получено: %v", err)
	}
}

type mockErr struct{ msg string }

func (e *mockErr) Error() string { return e.msg }

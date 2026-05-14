package naiveproxy

import (
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestBuildCmdline_Basic(t *testing.T) {
	p := Params{
		ListenPort: 18443,
		ProxyURL:   "https://u:p@host:443",
	}
	args := p.BuildCmdline()
	want := []string{
		"--listen=socks://127.0.0.1:18443",
		"--proxy=https://u:p@host:443",
	}
	if len(args) != len(want) {
		t.Fatalf("len=%d, want %d (%v)", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("args[%d]=%q, want %q", i, args[i], want[i])
		}
	}
}

func TestBuildCmdline_WithPadding(t *testing.T) {
	p := Params{ListenPort: 18443, ProxyURL: "https://u:p@h:443", Padding: true}
	args := p.BuildCmdline()
	found := false
	for _, a := range args {
		if a == "--padding" {
			found = true
		}
	}
	if !found {
		t.Errorf("--padding отсутствует: %v", args)
	}
}

func TestBuildCmdline_WithExtraHeaders(t *testing.T) {
	p := Params{ListenPort: 18443, ProxyURL: "https://u:p@h:443", ExtraHeaders: "X-Foo: bar"}
	args := p.BuildCmdline()
	found := false
	for _, a := range args {
		if a == "--extra-headers=X-Foo: bar" {
			found = true
		}
	}
	if !found {
		t.Errorf("--extra-headers отсутствует: %v", args)
	}
}

func TestParamsFromCanonical_RoundTrip(t *testing.T) {
	ob := types.Outbound{
		Tag: "n1", Type: "socks", Server: "host.example", Port: 443,
		Protocol: types.ProtocolNaive,
		Proto: &types.ProtoOpts{
			NaiveUsername: "user", NaivePassword: "p@ss/?",
			NaiveListenPort: 18443, NaivePadding: true,
		},
	}
	p, err := ParamsFromCanonical(ob)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.ListenPort != 18443 {
		t.Errorf("ListenPort=%d", p.ListenPort)
	}
	if !strings.HasPrefix(p.ProxyURL, "https://") {
		t.Errorf("ProxyURL=%q", p.ProxyURL)
	}
	if !strings.Contains(p.ProxyURL, "host.example:443") {
		t.Errorf("ProxyURL=%q не содержит host.example:443", p.ProxyURL)
	}
	if !p.Padding {
		t.Error("Padding не сохранился")
	}
}

func TestParamsFromCanonical_WrongProtocol(t *testing.T) {
	ob := types.Outbound{Protocol: types.ProtocolVLESS}
	_, err := ParamsFromCanonical(ob)
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestParamsFromCanonical_DefaultListenPort(t *testing.T) {
	ob := types.Outbound{
		Tag: "n", Type: "socks", Server: "h", Port: 443,
		Protocol: types.ProtocolNaive,
		Proto:    &types.ProtoOpts{NaiveUsername: "u"},
	}
	p, err := ParamsFromCanonical(ob)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if p.ListenPort != DefaultListenPort {
		t.Errorf("ListenPort=%d, want %d", p.ListenPort, DefaultListenPort)
	}
}

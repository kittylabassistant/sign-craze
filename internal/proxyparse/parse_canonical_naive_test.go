package proxyparse

import (
	"testing"

	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func TestParseCanonical_NaiveHTTPS_Basic(t *testing.T) {
	ob, canon, err := ParseCanonical("naive+https://user:secretpass@example.com:443#my-naive")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if canon.Protocol != types.ProtocolNaive {
		t.Errorf("Protocol = %q, want %q", canon.Protocol, types.ProtocolNaive)
	}
	if canon.Proto == nil {
		t.Fatal("Proto nil")
	}
	if canon.Proto.NaiveUsername != "user" {
		t.Errorf("NaiveUsername = %q, want \"user\"", canon.Proto.NaiveUsername)
	}
	if canon.Proto.NaivePassword != "secretpass" {
		t.Errorf("NaivePassword = %q", canon.Proto.NaivePassword)
	}
	if ob.Tag != "my-naive" {
		t.Errorf("Tag = %q, want \"my-naive\"", ob.Tag)
	}
	if ob.Server != "example.com" || ob.Port != 443 {
		t.Errorf("Server/Port = %s:%d, want example.com:443", ob.Server, ob.Port)
	}
	if ob.Type != "socks" {
		t.Errorf("Type = %q, want \"socks\"", ob.Type)
	}
}

func TestParseCanonical_NaiveHTTPS_Padding(t *testing.T) {
	_, canon, err := ParseCanonical("naive+https://u:p@host.example:443?padding=true")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !canon.Proto.NaivePadding {
		t.Error("NaivePadding = false, want true")
	}
}

func TestParseCanonical_NaiveHTTPS_ExtraHeaders(t *testing.T) {
	_, canon, err := ParseCanonical("naive+https://u:p@host:443?extra-headers=X-Foo%3A+bar")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if canon.Proto.NaiveExtraHdrs != "X-Foo: bar" {
		t.Errorf("NaiveExtraHdrs = %q, want \"X-Foo: bar\"", canon.Proto.NaiveExtraHdrs)
	}
}

func TestParseCanonical_NaiveQUIC(t *testing.T) {
	_, canon, err := ParseCanonical("naive+quic://u:p@host:443")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if canon.Protocol != types.ProtocolNaive {
		t.Errorf("Protocol = %q", canon.Protocol)
	}
}

func TestParseCanonical_Naive_EmptyUser_Error(t *testing.T) {
	_, _, err := ParseCanonical("naive+https://host:443")
	if err == nil {
		t.Fatal("ожидалась ошибка про отсутствие username")
	}
}

func TestParseCanonical_Naive_DefaultTag(t *testing.T) {
	ob, _, err := ParseCanonical("naive+https://u:p@host:443")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ob.Tag != "naive-out" {
		t.Errorf("Tag = %q, want \"naive-out\"", ob.Tag)
	}
}

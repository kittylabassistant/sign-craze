package naiveproxy

import (
	"testing"
)

func TestNewLifecycle_NotNil(t *testing.T) {
	lc := NewLifecycle("/opt/sbin/naive", Params{ListenPort: 18443, ProxyURL: "https://u:p@h:443"}, DefaultPIDFile)
	if lc == nil {
		t.Fatal("NewLifecycle вернул nil")
	}
}

func TestDefaultLifecycle_NotNil(t *testing.T) {
	if DefaultLifecycle() == nil {
		t.Fatal("DefaultLifecycle вернул nil")
	}
}

package dpi

import (
	"testing"
)

func TestNewLifecycle_СоздаётLifecycle(t *testing.T) {
	lc := NewLifecycle("/opt/sbin/nfqws2", "/opt/etc/sign-craze/nfqws2.conf", "/opt/var/run/sign-craze-nfqws2.pid")
	if lc == nil {
		t.Fatal("NewLifecycle вернул nil")
	}
}

func TestDefaultLifecycle_СоздаётLifecycle(t *testing.T) {
	lc := DefaultLifecycle()
	if lc == nil {
		t.Fatal("DefaultLifecycle вернул nil")
	}
}

func TestDefaultBinPath_СоответствуетСпеке(t *testing.T) {
	if DefaultBinPath != "/opt/sbin/nfqws2" {
		t.Errorf("DefaultBinPath = %q, ожидался /opt/sbin/nfqws2", DefaultBinPath)
	}
}

func TestDefaultPIDFile_СоответствуетСпеке(t *testing.T) {
	if DefaultPIDFile != "/opt/var/run/sign-craze-nfqws2.pid" {
		t.Errorf("DefaultPIDFile = %q, ожидался /opt/var/run/sign-craze-nfqws2.pid", DefaultPIDFile)
	}
}

package cli

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// TestAdminBypassWizard_Defaults проверяет что пустой ввод даёт port 22 и пустой
// список IP — это безопасные дефолты, защищающие от SSH-lockout (safety-fixes #2).
func TestAdminBypassWizard_Defaults(t *testing.T) {
	in := strings.NewReader("\n\n")
	var out bytes.Buffer
	port, ips, err := runAdminBypassWizard(bufio.NewReader(in), &out)
	if err != nil {
		t.Fatalf("runAdminBypassWizard: %v", err)
	}
	if port != 22 {
		t.Errorf("port = %d, ожидалось 22", port)
	}
	if len(ips) != 0 {
		t.Errorf("ips = %v, ожидался пустой список", ips)
	}
}

func TestAdminBypassWizard_CustomPort(t *testing.T) {
	in := strings.NewReader("2222\n\n")
	var out bytes.Buffer
	port, _, err := runAdminBypassWizard(bufio.NewReader(in), &out)
	if err != nil {
		t.Fatalf("runAdminBypassWizard: %v", err)
	}
	if port != 2222 {
		t.Errorf("port = %d, ожидалось 2222", port)
	}
}

func TestAdminBypassWizard_CIDRsAndSingleIPs(t *testing.T) {
	// Одиночные IP должны нормализоваться в /32, CIDR — оставаться как есть.
	in := strings.NewReader("22\n10.0.0.5, 192.168.1.0/24\n")
	var out bytes.Buffer
	_, ips, err := runAdminBypassWizard(bufio.NewReader(in), &out)
	if err != nil {
		t.Fatalf("runAdminBypassWizard: %v", err)
	}
	want := []string{"10.0.0.5/32", "192.168.1.0/24"}
	if len(ips) != len(want) {
		t.Fatalf("ips = %v, ожидалось %v", ips, want)
	}
	for i, w := range want {
		if ips[i] != w {
			t.Errorf("ips[%d] = %q, ожидалось %q", i, ips[i], w)
		}
	}
}

func TestAdminBypassWizard_InvalidPort(t *testing.T) {
	in := strings.NewReader("abc\n\n")
	var out bytes.Buffer
	if _, _, err := runAdminBypassWizard(bufio.NewReader(in), &out); err == nil {
		t.Error("ожидалась ошибка для нечислового порта")
	}
}

func TestAdminBypassWizard_PortOutOfRange(t *testing.T) {
	in := strings.NewReader("70000\n\n")
	var out bytes.Buffer
	if _, _, err := runAdminBypassWizard(bufio.NewReader(in), &out); err == nil {
		t.Error("ожидалась ошибка для порта > 65535")
	}
}

func TestAdminBypassWizard_InvalidCIDR(t *testing.T) {
	in := strings.NewReader("22\nне-cidr\n")
	var out bytes.Buffer
	if _, _, err := runAdminBypassWizard(bufio.NewReader(in), &out); err == nil {
		t.Error("ожидалась ошибка для некорректного CIDR")
	}
}

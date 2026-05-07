package dpi

import (
	"strings"
	"testing"
)

func TestNewLifecycle_СоздаётLifecycle(t *testing.T) {
	lc := NewLifecycle("/opt/sbin/nfqws2", DefaultConfigParams(), "/opt/var/run/sign-craze-nfqws2.pid")
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

func TestBuildCmdline_СтруктураUpstream(t *testing.T) {
	p := DefaultConfigParams()
	p.ISPInterface = "eth0"
	args := p.BuildCmdline()
	if len(args) == 0 {
		t.Fatal("BuildCmdline вернул пустой slice")
	}
	joined := strings.Join(args, " ")

	checks := []string{
		"--user=nobody",
		"--qnum=300",
		"--lua-init=@" + DefaultLuaDir + "/zapret-antidpi.lua",
		"--blob=quic_initial:@" + DefaultBlobDir + "/quic_initial.bin",
		"--blob=tls_clienthello:@" + DefaultBlobDir + "/tls_clienthello.bin",
		"--filter-udp=443",
		"--filter-l7=quic",
		"--filter-tcp=443,80,1984,5222",
		"--lua-desync=fake:blob=tls_clienthello",
		"--lua-desync=fake:blob=quic_initial",
	}
	for _, c := range checks {
		if !strings.Contains(joined, c) {
			t.Errorf("cmdline не содержит %q\nполная cmdline: %s", c, joined)
		}
	}

	newCount := 0
	for _, a := range args {
		if a == "--new" {
			newCount++
		}
	}
	if newCount != 2 {
		t.Errorf("ожидалось 2 маркера '--new' (UDP, QUIC), получено %d", newCount)
	}
}

func TestBuildCmdline_HostlistFlag(t *testing.T) {
	p := DefaultConfigParams()
	p.HostlistPath = "/tmp/hostlist.txt"
	args := p.BuildCmdline()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--hostlist=/tmp/hostlist.txt") {
		t.Errorf("HostlistPath не попал в cmdline: %s", joined)
	}
}

func TestBuildCmdline_StrategyOverride(t *testing.T) {
	p := DefaultConfigParams()
	p.Args = "--filter-tcp=443 --lua-desync=fake:custom"
	args := p.BuildCmdline()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--lua-desync=fake:custom") {
		t.Errorf("override Args не применился: %s", joined)
	}
	if strings.Contains(joined, "--lua-desync=fake:blob=tls_clienthello") {
		t.Errorf("после override остались дефолтные стратегии: %s", joined)
	}
}

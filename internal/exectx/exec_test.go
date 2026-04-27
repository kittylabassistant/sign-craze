package exectx

import (
	"context"
	"testing"
	"time"
)

func TestOSRunner_Echo(t *testing.T) {
	res, err := OS.Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(res.Stdout) != "hello\n" {
		t.Errorf("stdout = %q, want %q", res.Stdout, "hello\n")
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", res.ExitCode)
	}
}

func TestOSRunner_NonZeroExit(t *testing.T) {
	res, err := OS.Run(context.Background(), "false")
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if res.ExitCode == 0 {
		t.Error("exit code should be non-zero")
	}
}

func TestOSRunner_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := OS.Run(ctx, "sleep", "10")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestOSRunner_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := OS.Run(ctx, "sleep", "1")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMockRunner_HitAndMiss(t *testing.T) {
	r := Mock(map[string]Result{
		"iptables -L": {Stdout: []byte("Chain INPUT\n"), ExitCode: 0},
	})

	res, err := r.Run(context.Background(), "iptables", "-L")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(res.Stdout) != "Chain INPUT\n" {
		t.Errorf("stdout mismatch: %q", res.Stdout)
	}

	_, err = r.Run(context.Background(), "iptables", "-S")
	if err == nil {
		t.Fatal("expected error for unregistered command")
	}
}

func TestMockRunner_NonZeroExitReturnsError(t *testing.T) {
	r := Mock(map[string]Result{
		"iptables -C test": {ExitCode: 1, Stderr: []byte("no such rule")},
	})

	res, err := r.Run(context.Background(), "iptables", "-C", "test")
	if err == nil {
		t.Fatal("expected error for non-zero exit code")
	}
	if res.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.ExitCode)
	}
}

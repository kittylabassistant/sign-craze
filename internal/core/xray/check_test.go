package xray

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// TestCheckConfig_Success — успешный `xray run -test -c <config>`: argv
// собран верно, ошибки нет.
func TestCheckConfig_Success(t *testing.T) {
	var calls [][]string
	r := exectx.MockMatcher(func(name string, args ...string) (exectx.Result, error) {
		calls = append(calls, append([]string{name}, args...))
		return exectx.Result{ExitCode: 0, Stdout: []byte("ok")}, nil
	})

	if err := CheckConfig(context.Background(), r, "/opt/sbin/xray", "/tmp/config.json"); err != nil {
		t.Fatalf("CheckConfig: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("ожидался 1 вызов runner, получено %d: %v", len(calls), calls)
	}
	want := []string{"/opt/sbin/xray", "run", "-test", "-c", "/tmp/config.json"}
	if !reflect.DeepEqual(calls[0], want) {
		t.Errorf("argv = %v, ожидалось %v", calls[0], want)
	}
}

// TestCheckConfig_GenericError — ошибка (не таймаут) форматируется через
// core.CheckConfigError с префиксом "xray test:" и длительностью/exit/stderr/stdout.
func TestCheckConfig_GenericError(t *testing.T) {
	r := exectx.MockMatcher(func(_ string, _ ...string) (exectx.Result, error) {
		return exectx.Result{ExitCode: 23, Stderr: []byte("invalid config"), Stdout: []byte("")}, errors.New("exit status 23")
	})

	err := CheckConfig(context.Background(), r, "/opt/sbin/xray", "/tmp/config.json")
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	for _, want := range []string{"xray test:", "exit status 23", "exit: 23", "stderr: invalid config", "длительность:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ошибка %q не содержит %q", err.Error(), want)
		}
	}
}

// TestCheckConfig_FallbackToLegacyTest — v25+ `run -test` отвергнут как
// unknown command → fallback на legacy `test -c` в том же порядке, успех
// второй попытки не должен теряться.
func TestCheckConfig_FallbackToLegacyTest(t *testing.T) {
	var argvs [][]string
	r := exectx.MockMatcher(func(_ string, args ...string) (exectx.Result, error) {
		argvs = append(argvs, args)
		if args[0] == "run" {
			return exectx.Result{ExitCode: 1, Stderr: []byte(`xray: unknown command "run"`)}, errors.New("exit status 1")
		}
		return exectx.Result{ExitCode: 0}, nil
	})

	if err := CheckConfig(context.Background(), r, "/opt/sbin/xray", "/tmp/config.json"); err != nil {
		t.Fatalf("CheckConfig: %v (после fallback должен быть успех)", err)
	}
	if len(argvs) != 2 {
		t.Fatalf("ожидалось 2 попытки (run -test, затем test), получено %d: %v", len(argvs), argvs)
	}
	if argvs[0][0] != "run" || argvs[1][0] != "test" {
		t.Errorf("порядок попыток = %v, ожидалось [[run ...] [test ...]]", argvs)
	}
}

// TestCheckConfig_FallbackAlsoFails — если и legacy `test -c` падает, наружу
// уходит ошибка именно от legacy-попытки (её stderr/exit), а не от первой.
func TestCheckConfig_FallbackAlsoFails(t *testing.T) {
	r := exectx.MockMatcher(func(_ string, args ...string) (exectx.Result, error) {
		if args[0] == "run" {
			return exectx.Result{ExitCode: 1, Stderr: []byte("unknown command")}, errors.New("exit status 1")
		}
		return exectx.Result{ExitCode: 1, Stderr: []byte("legacy test failed")}, errors.New("exit status 1")
	})

	err := CheckConfig(context.Background(), r, "/opt/sbin/xray", "/tmp/config.json")
	if err == nil {
		t.Fatal("ожидалась ошибка после провала обеих попыток")
	}
	if !strings.Contains(err.Error(), "legacy test failed") {
		t.Errorf("ошибка = %q, ожидалось упоминание вывода legacy-попытки", err.Error())
	}
}

// TestIsUnknownCommand покрывает обе распознаваемые формы (command/subcommand)
// и негативные случаи (обычная ошибка конфига не должна триггерить fallback).
func TestIsUnknownCommand(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"unknown command", `xray: unknown command "run"`, true},
		{"unknown subcommand", "unknown subcommand: run", true},
		{"обычная ошибка конфига", "main: failed to read config: EOF", false},
		{"пусто", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnknownCommand(tt.stderr); got != tt.want {
				t.Errorf("isUnknownCommand(%q) = %v, ожидалось %v", tt.stderr, got, tt.want)
			}
		})
	}
}

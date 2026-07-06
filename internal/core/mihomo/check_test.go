package mihomo

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// TestCheckConfig_Success_ConfigPathIsDir — configPath уже директория (как
// её передаёт core.Core.CheckConfig в проде) — используется без изменений.
func TestCheckConfig_Success_ConfigPathIsDir(t *testing.T) {
	var calls [][]string
	r := exectx.MockMatcher(func(name string, args ...string) (exectx.Result, error) {
		calls = append(calls, append([]string{name}, args...))
		return exectx.Result{ExitCode: 0}, nil
	})

	if err := CheckConfig(context.Background(), r, "/opt/sbin/mihomo", "/opt/etc/sign-craze/mihomo"); err != nil {
		t.Fatalf("CheckConfig: %v", err)
	}
	want := []string{"/opt/sbin/mihomo", "-t", "-d", "/opt/etc/sign-craze/mihomo"}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], want) {
		t.Errorf("argv = %v, ожидалось [%v]", calls, want)
	}
}

// TestCheckConfig_ConfigPathIsFile — если передан путь к файлу
// (.yaml/.yml/.json), CheckConfig берёт его родительскую директорию.
func TestCheckConfig_ConfigPathIsFile(t *testing.T) {
	tests := []struct {
		configPath string
		wantDir    string
	}{
		{"/opt/etc/sign-craze/mihomo/config.yaml", "/opt/etc/sign-craze/mihomo"},
		{"/tmp/cfg/config.yml", "/tmp/cfg"},
		{"/tmp/cfg/config.json", "/tmp/cfg"},
	}
	for _, tt := range tests {
		t.Run(tt.configPath, func(t *testing.T) {
			var gotDir string
			r := exectx.MockMatcher(func(_ string, args ...string) (exectx.Result, error) {
				if len(args) == 3 {
					gotDir = args[2]
				}
				return exectx.Result{ExitCode: 0}, nil
			})
			if err := CheckConfig(context.Background(), r, "/opt/sbin/mihomo", tt.configPath); err != nil {
				t.Fatalf("CheckConfig: %v", err)
			}
			if gotDir != tt.wantDir {
				t.Errorf("-d %q, ожидалось %q", gotDir, tt.wantDir)
			}
		})
	}
}

// TestCheckConfig_GenericError — ошибка форматируется через
// core.CheckConfigError с префиксом "mihomo -t:".
func TestCheckConfig_GenericError(t *testing.T) {
	r := exectx.MockMatcher(func(_ string, _ ...string) (exectx.Result, error) {
		return exectx.Result{ExitCode: 1, Stderr: []byte("bad proxy config"), Stdout: []byte("")}, errors.New("exit status 1")
	})

	err := CheckConfig(context.Background(), r, "/opt/sbin/mihomo", "/tmp/config.yaml")
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	for _, want := range []string{"mihomo -t:", "exit status 1", "exit: 1", "stderr: bad proxy config", "длительность:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("ошибка %q не содержит %q", err.Error(), want)
		}
	}
}

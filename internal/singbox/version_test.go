package singbox

import (
	"context"
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    string
		wantErr bool
	}{
		{
			name:   "стандартный формат",
			output: "sing-box version 1.10.0\n",
			want:   "1.10.0",
		},
		{
			name:   "с префиксом v",
			output: "sing-box version v1.10.0\n",
			want:   "1.10.0",
		},
		{
			name:   "многострочный вывод",
			output: "sing-box version 1.10.0\nEnvironment: ...\nOS/Arch: ...\n",
			want:   "1.10.0",
		},
		{
			name:   "только версия последним словом",
			output: "sing-box 1.9.5\n",
			want:   "1.9.5",
		},
		{
			name:    "пустой вывод",
			output:  "",
			wantErr: true,
		},
		{
			name:    "только пробелы",
			output:  "   \n   \n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseVersion(tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseVersion() = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestBinaryVersion_MockRunner(t *testing.T) {
	runner := exectx.Mock(map[string]exectx.Result{
		"/opt/sbin/sing-box version": {
			Stdout:   []byte("sing-box version 1.10.0\n"),
			ExitCode: 0,
		},
	})

	ver, err := BinaryVersion(context.Background(), runner, "/opt/sbin/sing-box")
	if err != nil {
		t.Fatalf("BinaryVersion: %v", err)
	}
	if ver != "1.10.0" {
		t.Errorf("версия = %q, ожидалось %q", ver, "1.10.0")
	}
}

func TestBinaryVersion_RunnerError(t *testing.T) {
	runner := exectx.Mock(map[string]exectx.Result{}) // нет записей

	_, err := BinaryVersion(context.Background(), runner, "/opt/sbin/sing-box")
	if err == nil {
		t.Fatal("ожидалась ошибка при отсутствии sing-box")
	}
}

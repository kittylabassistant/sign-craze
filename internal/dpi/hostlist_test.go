package dpi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHostlist_ОдинДоменНаСтроку(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dpi-hostlist.txt")
	targets := []string{"discord.com", "youtube.com", "googlevideo.com"}

	if err := WriteHostlist(path, targets); err != nil {
		t.Fatalf("WriteHostlist: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	got := string(data)
	want := "discord.com\nyoutube.com\ngooglevideo.com\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestWriteHostlist_ПустойСписокПишетПустойФайл(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	if err := WriteHostlist(path, nil); err != nil {
		t.Fatalf("WriteHostlist: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("ожидался пустой файл, получено %d байт", len(data))
	}
}

func TestWriteHostlist_ПустыеСтрокиПропускаются(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "list.txt")
	targets := []string{"discord.com", "  ", "", "youtube.com", "\t\n"}
	if err := WriteHostlist(path, targets); err != nil {
		t.Fatalf("WriteHostlist: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "discord.com\nyoutube.com\n"
	if string(data) != want {
		t.Errorf("got %q, want %q", string(data), want)
	}
}

func TestWriteHostlist_ОшибкаПриПустомPath(t *testing.T) {
	if err := WriteHostlist("", []string{"discord.com"}); err == nil {
		t.Error("ожидалась ошибка при пустом path")
	}
}

func TestGenerateConfig_СHostlistPath_ВставляетФлаг(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "nfqws2.conf")

	params := DefaultConfigParams()
	params.HostlistPath = "/opt/etc/sign-craze/dpi-hostlist.txt"

	if err := GenerateConfig(params, dst); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	checks := []string{
		`NFQWS_HOSTLIST="/opt/etc/sign-craze/dpi-hostlist.txt"`,
		"--hostlist=/opt/etc/sign-craze/dpi-hostlist.txt",
	}
	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Errorf("конфиг не содержит %q\nконтент:\n%s", c, content)
		}
	}
}

func TestGenerateConfig_БезHostlistPath_БезФлага(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "nfqws2.conf")

	params := DefaultConfigParams()
	// HostlistPath = "" — флаг не должен вставляться.

	if err := GenerateConfig(params, dst); err != nil {
		t.Fatalf("GenerateConfig: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "--hostlist=") {
		t.Errorf("конфиг не должен содержать --hostlist при пустом HostlistPath\nконтент:\n%s", content)
	}
	if !strings.Contains(content, `NFQWS_HOSTLIST=""`) {
		t.Errorf("ожидалась пустая NFQWS_HOSTLIST переменная\nконтент:\n%s", content)
	}
}

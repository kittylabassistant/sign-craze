//go:build xraycheck

package xray

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestXrayCheck_Golden прогоняет `xray test -c <файл>` для каждого
// golden canonical-конфига из testdata/canonical/.
//
// Тест пропускается если `xray` не найден в PATH — это позволяет запускать
// suite локально без установленного xray.
//
// В CI (workflow xray-integration.yml) xray устанавливается до запуска теста,
// поэтому skip не происходит.
func TestXrayCheck_Golden(t *testing.T) {
	xrayBin, err := exec.LookPath("xray")
	if err != nil {
		t.Skip("xray не найден в PATH, пропускаем: " + err.Error())
	}
	t.Logf("xray: %s", xrayBin)

	pattern := filepath.Join("testdata", "canonical", "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(files) == 0 {
		t.Fatalf("golden-файлы не найдены по шаблону %s", pattern)
	}

	for _, f := range files {
		f := f
		base := filepath.Base(f)

		t.Run(base, func(t *testing.T) {
			// filepath.Abs нужен потому что xray требует абсолютный
			// или корректный относительный путь; тест может запускаться из
			// произвольного CWD при go test ./...
			abs, err := filepath.Abs(f)
			if err != nil {
				t.Fatalf("filepath.Abs(%s): %v", f, err)
			}

			// `xray run -test -c <файл>` валидирует конфиг без запуска
			// сервера: парсит JSON и проверяет схему/протоколы. В Xray-core
			// v25+ subcommand `test` отсутствует — есть только флаг `-test`
			// у `run`.
			cmd := exec.Command(xrayBin, "run", "-test", "-c", abs)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("xray test завершился с ошибкой для %s:\n%s", base, string(out))
			}
		})
	}
}

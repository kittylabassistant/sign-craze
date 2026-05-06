//go:build singboxcheck

package singbox

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestSingBoxCheck_Golden прогоняет `sing-box check -c <файл>` для каждого
// golden canonical-конфига из testdata/canonical/.
//
// Тест пропускается если `sing-box` не найден в PATH — это позволяет запускать
// suite локально без установленного sing-box.
//
// В CI (workflow singbox-check.yml) sing-box устанавливается до запуска теста,
// поэтому skip не происходит.
func TestSingBoxCheck_Golden(t *testing.T) {
	singboxBin, err := exec.LookPath("sing-box")
	if err != nil {
		t.Skip("sing-box не найден в PATH, пропускаем: " + err.Error())
	}
	t.Logf("sing-box: %s", singboxBin)

	// Полные конфиги — файлы без суффикса _outbound.json
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

		// _outbound.json — фрагменты outbound для TestRender_GoldenWriteback;
		// они не являются полными sing-box конфигами, sing-box check их отклонит.
		matched, _ := filepath.Match("*_outbound.json", base)
		if matched {
			continue
		}

		t.Run(base, func(t *testing.T) {
			// filepath.Abs нужен потому что sing-box check требует абсолютный
			// или корректный относительный путь; тест может запускаться из
			// произвольного CWD при go test ./...
			abs, err := filepath.Abs(f)
			if err != nil {
				t.Fatalf("filepath.Abs(%s): %v", f, err)
			}

			cmd := exec.Command(singboxBin, "check", "-c", abs)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("sing-box check завершился с ошибкой для %s:\n%s", base, string(out))
			}
		})
	}
}

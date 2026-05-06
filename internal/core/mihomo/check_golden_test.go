//go:build mihomocheck

package mihomo

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMihomoCheck_Golden запускает `mihomo -t -d <dir>` для каждого golden
// canonical-конфига из testdata/canonical/.
//
// Тест пропускается если `mihomo` не найден в PATH — позволяет запускать suite
// локально без установленного mihomo. В CI (workflow mihomo-integration.yml)
// mihomo устанавливается до запуска теста, поэтому skip не происходит.
//
// Алгоритм: для каждого файла *.yaml (исключая *_outbound.yaml) копируем его
// во временную директорию под именем config.yaml, затем вызываем
// `mihomo -t -d <tmpdir>`. Флаг -t означает тестовый режим (валидация без запуска).
func TestMihomoCheck_Golden(t *testing.T) {
	mihomoBin, err := exec.LookPath("mihomo")
	if err != nil {
		t.Skip("mihomo не найден в PATH, пропускаем: " + err.Error())
	}
	t.Logf("mihomo: %s", mihomoBin)

	// Полные конфиги — файлы без суффикса _outbound.yaml
	pattern := filepath.Join("testdata", "canonical", "*.yaml")
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

		// *_outbound.yaml — фрагменты outbound для diff-тестов;
		// они не являются полными mihomo конфигами и не пройдут -t валидацию.
		matched, _ := filepath.Match("*_outbound.yaml", base)
		if matched {
			continue
		}

		t.Run(base, func(t *testing.T) {
			// Создаём временную директорию для mihomo -d <dir>.
			// mihomo ищет config.yaml внутри директории.
			tmpDir, err := os.MkdirTemp("", "mihomo-golden-*")
			if err != nil {
				t.Fatalf("MkdirTemp: %v", err)
			}
			defer os.RemoveAll(tmpDir)

			// Копируем golden-файл как config.yaml во временную директорию.
			src, err := os.Open(f)
			if err != nil {
				t.Fatalf("открытие golden-файла %s: %v", f, err)
			}
			defer src.Close()

			dst, err := os.Create(filepath.Join(tmpDir, "config.yaml"))
			if err != nil {
				t.Fatalf("создание config.yaml: %v", err)
			}
			if _, err := io.Copy(dst, src); err != nil {
				dst.Close()
				t.Fatalf("копирование %s → config.yaml: %v", base, err)
			}
			dst.Close()

			// mihomo -t -d <tmpdir> — валидация без запуска процесса.
			cmd := exec.Command(mihomoBin, "-t", "-d", tmpDir)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("mihomo -t завершился с ошибкой для %s:\n%s", base, string(out))
			}
		})
	}
}

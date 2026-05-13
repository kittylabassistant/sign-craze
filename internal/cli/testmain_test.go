package cli

import (
	"os"
	"testing"
)

// TestMain принудительно отключает ANSI-цвет для всех тестов пакета cli.
// Иначе тесты, которые сверяют содержимое bytes.Buffer (wizard и т.п.),
// зависели бы от TTY-состояния процесса go test (CI vs локальный запуск).
//
// Тесты в color_test.go сами сбрасывают состояние через
// ResetColorStateForTest и инициализируют под свои сценарии.
func TestMain(m *testing.M) {
	_ = os.Setenv("NO_COLOR", "1")
	ResetColorStateForTest()
	InitColor(false)
	os.Exit(m.Run())
}

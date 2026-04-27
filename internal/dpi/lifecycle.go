package dpi

import (
	"github.com/kittylabassistant/sign-craze/internal/service"
)

const (
	// DefaultPIDFile — путь к PID-файлу nfqws2.
	DefaultPIDFile = "/opt/var/run/sign-craze-nfqws2.pid"
)

// NewLifecycle создаёт Lifecycle для процесса nfqws2.
// binPath — путь к бинарю (обычно DefaultBinPath).
// configPath — путь к nfqws2.conf (обычно DefaultConfigPath).
// pidFile — путь к PID-файлу (обычно DefaultPIDFile).
func NewLifecycle(binPath, configPath, pidFile string) service.Lifecycle {
	return service.NewLifecycle(service.ProcessConfig{
		Name:    "nfqws2",
		BinPath: binPath,
		// nfqws2 читает конфиг через переменные окружения из conf-файла;
		// запускается скриптом-обёрткой из пакета nfqws2-keenetic,
		// который источает конфиг и передаёт NFQWS_ARGS нфикусу.
		// Прямой запуск бинаря требует аргументов из конфига.
		Args:    []string{"--config", configPath},
		PIDFile: pidFile,
	})
}

// DefaultLifecycle создаёт Lifecycle с путями по умолчанию.
func DefaultLifecycle() service.Lifecycle {
	return NewLifecycle(DefaultBinPath, DefaultConfigPath, DefaultPIDFile)
}

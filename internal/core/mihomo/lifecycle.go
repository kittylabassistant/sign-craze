package mihomo

import (
	"github.com/kittylabassistant/sign-craze/internal/service"
)

// NewLifecycle создаёт Lifecycle для процесса mihomo.
//
// Аргументы запуска: `mihomo -d <configDir>` — mihomo принимает директорию,
// а не файл (внутри живут config.yaml + cache.db + providers/). Это отличает
// его от sing-box/xray, которым нужен путь к файлу.
//
// configDir обычно DefaultConfigDir.
func NewLifecycle(binPath, configDir, pidFile string) service.Lifecycle {
	return service.NewLifecycle(service.ProcessConfig{
		Name:       "mihomo",
		BinPath:    binPath,
		Args:       []string{"-d", configDir},
		PIDFile:    pidFile,
		StderrPath: DefaultStderrLog,
	})
}

// DefaultLifecycle создаёт Lifecycle с путями по умолчанию.
func DefaultLifecycle() service.Lifecycle {
	return NewLifecycle(DefaultBinPath, DefaultConfigDir, DefaultPIDFile)
}

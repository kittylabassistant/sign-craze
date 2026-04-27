package singbox

import (
	"github.com/kittylabassistant/sign-craze/internal/service"
)

const (
	// DefaultPIDFile — путь к PID-файлу sing-box.
	DefaultPIDFile = "/opt/var/run/sign-craze-singbox.pid"
)

// NewLifecycle создаёт Lifecycle для процесса sing-box.
// binPath — путь к бинарю (обычно DefaultBinPath).
// configPath — путь к config.json (обычно <DefaultConfigDir>/config.json).
// pidFile — путь к PID-файлу (обычно DefaultPIDFile).
func NewLifecycle(binPath, configPath, pidFile string) service.Lifecycle {
	return service.NewLifecycle(service.ProcessConfig{
		Name:    "sing-box",
		BinPath: binPath,
		Args:    []string{"run", "-c", configPath},
		PIDFile: pidFile,
	})
}

// DefaultLifecycle создаёт Lifecycle с путями по умолчанию.
func DefaultLifecycle() service.Lifecycle {
	return NewLifecycle(DefaultBinPath, DefaultConfigDir+"/config.json", DefaultPIDFile)
}

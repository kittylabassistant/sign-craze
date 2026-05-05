package xray

import (
	"github.com/kittylabassistant/sign-craze/internal/service"
)

// NewLifecycle создаёт Lifecycle для процесса xray.
//
// Аргументы запуска: `xray run -c <config>` (формально совпадает с sing-box,
// но семантика различается: xray не имеет TUN inbound, вместо этого надо
// использовать dokodemo-door с tproxy).
func NewLifecycle(binPath, configPath, pidFile string) service.Lifecycle {
	return service.NewLifecycle(service.ProcessConfig{
		Name:       "xray",
		BinPath:    binPath,
		Args:       []string{"run", "-c", configPath},
		PIDFile:    pidFile,
		StderrPath: DefaultStderrLog,
	})
}

// DefaultLifecycle создаёт Lifecycle с путями по умолчанию.
func DefaultLifecycle() service.Lifecycle {
	return NewLifecycle(DefaultBinPath, DefaultConfigPath, DefaultPIDFile)
}

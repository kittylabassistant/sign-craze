package singbox

import (
	"github.com/kittylabassistant/sign-craze/internal/service"
)

const (
	// DefaultPIDFile — путь к PID-файлу sing-box.
	DefaultPIDFile = "/opt/var/run/sign-craze-singbox.pid"
	// DefaultStderrLog — путь к файлу для stdout/stderr sing-box.
	// Используется для диагностики ранних crash'ей: sing-box может упасть
	// до открытия собственного лог-файла, и без захвата stderr диагностика
	// невозможна.
	DefaultStderrLog = "/opt/var/log/sign-craze/sing-box.stderr.log"
)

// newLifecycle создаёт Lifecycle для процесса sing-box.
// binPath — путь к бинарю (обычно DefaultBinPath).
// configPath — путь к config.json (обычно <DefaultConfigDir>/config.json).
// pidFile — путь к PID-файлу (обычно DefaultPIDFile).
func newLifecycle(binPath, configPath, pidFile string) service.Lifecycle {
	return service.NewLifecycle(service.ProcessConfig{
		Name:       "sing-box",
		BinPath:    binPath,
		Args:       []string{"run", "-c", configPath},
		PIDFile:    pidFile,
		StderrPath: DefaultStderrLog,
	})
}

// DefaultLifecycle создаёт Lifecycle с путями по умолчанию.
func DefaultLifecycle() service.Lifecycle {
	return newLifecycle(DefaultBinPath, DefaultConfigDir+"/config.json", DefaultPIDFile)
}

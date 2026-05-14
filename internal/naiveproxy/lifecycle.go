package naiveproxy

import (
	"github.com/kittylabassistant/sign-craze/internal/service"
)

const (
	// DefaultPIDFile — путь к PID-файлу naive daemon.
	DefaultPIDFile = "/opt/var/run/sign-craze-naive.pid"
	// DefaultStderrPath — куда уходят stderr/stdout naive. Без этого ранние
	// ошибки (битый --proxy URL, отказ TLS) теряются — Start вернёт лишь
	// "процесс упал сразу после старта" без причины.
	DefaultStderrPath = "/opt/var/log/sign-craze/naive.stderr.log"
)

// NewLifecycle создаёт Lifecycle для процесса naive.
// binPath — обычно DefaultBinPath; params формируются ParamsFromCanonical(outbound);
// pidFile — обычно DefaultPIDFile.
func NewLifecycle(binPath string, params Params, pidFile string) service.Lifecycle {
	return service.NewLifecycle(service.ProcessConfig{
		Name:       "naive",
		BinPath:    binPath,
		Args:       params.BuildCmdline(),
		PIDFile:    pidFile,
		StderrPath: DefaultStderrPath,
	})
}

// DefaultLifecycle — Lifecycle с дефолтами для stop-flow (где параметры не нужны).
// НЕ использовать для Start — Args будут пустые (ProxyURL=""), naive упадёт сразу.
func DefaultLifecycle() service.Lifecycle {
	return NewLifecycle(DefaultBinPath, Params{ListenPort: DefaultListenPort}, DefaultPIDFile)
}

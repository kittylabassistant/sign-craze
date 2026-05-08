package dpi

import (
	"github.com/kittylabassistant/sign-craze/internal/service"
)

const (
	// DefaultPIDFile — путь к PID-файлу nfqws2.
	DefaultPIDFile = "/opt/var/run/sign-craze-nfqws2.pid"
	// DefaultStderrPath — куда уходят stdout/stderr nfqws2. Без этого ранний
	// crash (битый аргумент, отсутствие kernel-модуля nfnetlink_queue) теряется
	// в /dev/null и Start возвращает только "процесс упал сразу после старта"
	// без причины.
	DefaultStderrPath = "/opt/var/log/sign-craze/nfqws2.stderr.log"
)

// NewLifecycle создаёт Lifecycle для процесса nfqws2.
// binPath — путь к бинарю (обычно DefaultBinPath).
// params — параметры стратегии. Командная строка формируется через
// ConfigParams.BuildCmdline() — бинарь nfqws2 НЕ читает .conf-файл сам,
// upstream nfqws2-keenetic source-ит его в init.d-обёртке. Мы делаем то же
// самое в Go, чтобы избежать зависимости от shell на роутере.
// pidFile — путь к PID-файлу (обычно DefaultPIDFile).
func NewLifecycle(binPath string, params ConfigParams, pidFile string) service.Lifecycle {
	return service.NewLifecycle(service.ProcessConfig{
		Name:       "nfqws2",
		BinPath:    binPath,
		Args:       params.BuildCmdline(),
		PIDFile:    pidFile,
		StderrPath: DefaultStderrPath,
	})
}

// DefaultLifecycle создаёт Lifecycle с путями и параметрами по умолчанию.
// Используется в местах где state ещё не загружен — caller должен
// предпочитать NewLifecycle с актуальными параметрами из state.
func DefaultLifecycle() service.Lifecycle {
	return NewLifecycle(DefaultBinPath, DefaultConfigParams(), DefaultPIDFile)
}

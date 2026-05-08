package firewall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// Пути к kernel-модулям на Keenetic 4.9 mipsle.
// На других kernel-версиях используйте FindKernelModule.
const (
	KernelModulePathXtSocket = "/lib/modules/4.9-ndm-5/xt_socket.ko"
	KernelModulePathXtTProxy = "/lib/modules/4.9-ndm-5/xt_TPROXY.ko"
)

// IsModuleLoaded проверяет /proc/modules — загружен ли модуль с именем name.
func IsModuleLoaded(name string) bool {
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return true
		}
	}
	return false
}

// FindKernelModule возвращает путь к .ko-файлу через glob /lib/modules/*/<name>.ko.
// Если найдено несколько — возвращает первый. Если ничего не найдено — пустая строка.
func FindKernelModule(name string) string {
	matches, _ := filepath.Glob(fmt.Sprintf("/lib/modules/*/%s.ko", name))
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// EnsureKernelModule загружает kernel-модуль через `insmod` если он ещё не загружен.
// На Keenetic нет modprobe (и modules.dep), нужен явный путь к .ko.
//
// Идемпотентно: если модуль уже в /proc/modules — no-op.
// Если path не существует — возвращает ошибку (не Keenetic / другой kernel).
// Race: если другой процесс параллельно insmod'ит — игнорируем "File exists".
func EnsureKernelModule(ctx context.Context, runner exectx.Runner, name, path string) error {
	if IsModuleLoaded(name) {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("kernel module %q отсутствует: %w", path, err)
	}
	res, err := runner.Run(ctx, "insmod", path)
	if err != nil {
		errStr := string(res.Stderr)
		if strings.Contains(errStr, "File exists") || strings.Contains(errStr, "already loaded") {
			return nil // race: загружен параллельно
		}
		return fmt.Errorf("insmod %s: %w (stderr: %s)", path, err, errStr)
	}
	return nil
}

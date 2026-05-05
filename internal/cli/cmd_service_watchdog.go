package cli

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/service"
)

func init() {
	Register(Cmd{
		Long:    "--service-watchdog",
		Help:    "(internal) standalone firewall watchdog daemon, запускается из init.d shim",
		Handler: handleServiceWatchdog,
		Hidden:  true,
	})
}

// handleServiceWatchdog — блокирующий standalone-демон firewall watchdog.
//
// Назначение: запускается из init.d shim в фоне после --service-start, чтобы
// watchdog существовал и переживал ребут роутера. Без этого watchdog существует
// только в процессе `sign-craze --ui on`, который не входит в boot path init.d.
//
// Не имеет web-сервера — только горутина watchdog. Простой и лёгкий процесс
// (~5MB RSS), приемлем даже на KN-1410 с 128MB RAM.
//
// Lifecycle: блокируется до отмены ctx (SIGTERM от init.d stop). При остановке
// горутина watchdog завершается через ctx.Done.
func handleServiceWatchdog(ctx context.Context, _ []string) error {
	wd := firewall.NewWatchdog(0, reconcileFirewall)
	wd.Run(ctx)
	return nil
}

// stopWatchdog корректно останавливает standalone watchdog daemon, запущенный
// init.d shim'ом. Без этого после --uninstall watchdog продолжает крутиться,
// каждые 30s вызывает loadState (Default при ErrNotExist) → ensureKeeneticPolicy
// → saveState, что регенерирует /opt/etc/sign-craze/state.json и блокирует
// последующий --install.
//
// Шаги:
//  1. Прочитать PID из service.DefaultWatchdogPIDPath.
//  2. SIGTERM → ждать до 5s graceful exit (poll /proc/<pid>).
//  3. SIGKILL если не вышел.
//  4. Удалить PID-файл.
//
// Возвращает true если был активный процесс (или PID-файл) и его остановили/
// удалили; false если ничего не было.
func stopWatchdog(_ context.Context) bool {
	pidStr, err := os.ReadFile(service.DefaultWatchdogPIDPath)
	if err != nil {
		// PID-файла нет — но процесс всё равно может работать (например, после
		// частичного uninstall). Скан /proc дороже, в типичном кейсе не нужен.
		return false
	}
	defer os.Remove(service.DefaultWatchdogPIDPath)

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidStr)))
	if err != nil || pid <= 1 {
		return true // PID-файл удалён через defer, мусорное содержимое
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return true
	}

	// Проверка что процесс действительно sign-craze --service-watchdog: PID
	// мог быть переиспользован после reboot. На Linux читаем /proc/<pid>/comm.
	if comm, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm"); err == nil {
		if !strings.HasPrefix(strings.TrimSpace(string(comm)), "sign-craze") {
			log.L().Debug("stopWatchdog: PID не sign-craze, пропуск", "pid", pid, "comm", string(comm))
			return true
		}
	} else {
		// /proc/<pid>/comm нет — процесс уже мёртв.
		return true
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		log.L().Debug("stopWatchdog: SIGTERM failed", "pid", pid, "err", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat("/proc/" + strconv.Itoa(pid)); err != nil {
			log.L().Info("watchdog daemon остановлен", "pid", pid)
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}

	log.L().Warn("watchdog не ответил на SIGTERM, посылаем SIGKILL", "pid", pid)
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		log.L().Debug("stopWatchdog: SIGKILL failed", "pid", pid, "err", err)
	}
	return true
}

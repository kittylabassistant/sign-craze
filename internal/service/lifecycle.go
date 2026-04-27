package service

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/log"
)

const (
	stopGracePeriod = 10 * time.Second
	startPollPeriod = 200 * time.Millisecond
	startTimeout    = 5 * time.Second
)

// Status описывает состояние процесса.
type Status struct {
	Running bool
	PID     int
}

// Lifecycle управляет запуском, остановкой и мониторингом одного процесса.
type Lifecycle interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Status(ctx context.Context) (Status, error)
	Restart(ctx context.Context) error
}

// ProcessConfig описывает процесс, которым управляет Lifecycle.
type ProcessConfig struct {
	Name    string   // имя для логов
	BinPath string   // путь к бинарю
	Args    []string // аргументы запуска
	PIDFile string   // путь к PID-файлу
}

// processLifecycle реализует Lifecycle для одного OS-процесса.
type processLifecycle struct {
	cfg ProcessConfig
}

// NewLifecycle создаёт Lifecycle для указанного процесса.
func NewLifecycle(cfg ProcessConfig) Lifecycle {
	return &processLifecycle{cfg: cfg}
}

// Start запускает процесс в фоне (detach), записывает PID в файл.
// Ждёт подтверждения через /proc до startTimeout.
func (l *processLifecycle) Start(ctx context.Context) error {
	st, err := l.Status(ctx)
	if err != nil {
		return err
	}
	if st.Running {
		return fmt.Errorf("service %s: уже запущен (pid %d)", l.cfg.Name, st.PID)
	}

	cmd := buildCmd(l.cfg.BinPath, l.cfg.Args...)
	// детач: процесс становится лидером группы и не умирает вместе с sign-craze
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("service %s: запуск: %w", l.cfg.Name, err)
	}

	pid := cmd.Process.Pid
	log.L().Info("процесс запущен", "service", l.cfg.Name, "pid", pid)

	if err := writePID(l.cfg.PIDFile, pid); err != nil {
		// PID-файл не записан — убиваем процесс, чтобы не потерять его
		if killErr := cmd.Process.Kill(); killErr != nil {
			log.L().Warn("не удалось завершить процесс при ошибке PID-файла", "service", l.cfg.Name, "err", killErr)
		}
		return fmt.Errorf("service %s: запись PID-файла: %w", l.cfg.Name, err)
	}

	// ждём, пока процесс появится в /proc
	if err := waitAlive(ctx, pid, startTimeout); err != nil {
		if killErr := cmd.Process.Kill(); killErr != nil {
			log.L().Warn("не удалось завершить процесс при таймауте старта", "service", l.cfg.Name, "err", killErr)
		}
		if rmErr := os.Remove(l.cfg.PIDFile); rmErr != nil {
			log.L().Warn("не удалось удалить PID-файл", "service", l.cfg.Name, "err", rmErr)
		}
		return fmt.Errorf("service %s: процесс не запустился: %w", l.cfg.Name, err)
	}

	// Асинхронно подбираем зомби-статус чтобы не засорять таблицу процессов.
	// В production sign-craze завершится раньше sing-box и init усыновит его,
	// но в тестах родитель живёт дольше — без Wait() убитый дочерний процесс
	// остаётся зомби в /proc и ломает проверку processAlive.
	go func() {
		_ = cmd.Wait() //nolint:errcheck // статус выхода не важен при пожинании зомби
	}()
	return nil
}

// Stop отправляет SIGTERM, ждёт gracePeriod, затем SIGKILL.
func (l *processLifecycle) Stop(ctx context.Context) error {
	st, err := l.Status(ctx)
	if err != nil {
		return err
	}
	if !st.Running {
		log.L().Info("сервис не запущен, остановка не нужна", "service", l.cfg.Name)
		return nil
	}

	proc, err := os.FindProcess(st.PID)
	if err != nil {
		return fmt.Errorf("service %s: поиск процесса %d: %w", l.cfg.Name, st.PID, err)
	}

	log.L().Info("отправляем SIGTERM", "service", l.cfg.Name, "pid", st.PID)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("service %s: SIGTERM: %w", l.cfg.Name, err)
	}

	deadline := time.Now().Add(stopGracePeriod)
	for time.Now().Before(deadline) {
		if !processAlive(st.PID) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if processAlive(st.PID) {
		log.L().Warn("процесс не завершился по SIGTERM, отправляем SIGKILL", "service", l.cfg.Name)
		if err := proc.Signal(syscall.SIGKILL); err != nil {
			log.L().Warn("не удалось отправить SIGKILL", "service", l.cfg.Name, "err", err)
		}
	}

	if err := os.Remove(l.cfg.PIDFile); err != nil {
		log.L().Warn("не удалось удалить PID-файл при остановке", "service", l.cfg.Name, "err", err)
	}
	log.L().Info("сервис остановлен", "service", l.cfg.Name)
	return nil
}

// Status проверяет PID-файл и наличие процесса в /proc.
func (l *processLifecycle) Status(_ context.Context) (Status, error) {
	pid, err := readPID(l.cfg.PIDFile)
	if err != nil || pid == 0 {
		return Status{}, nil
	}
	if !processAlive(pid) {
		// устаревший PID-файл
		_ = os.Remove(l.cfg.PIDFile)
		return Status{}, nil
	}
	return Status{Running: true, PID: pid}, nil
}

// Restart останавливает и запускает сервис. Блокировка не захватывается — должна быть взята снаружи.
func (l *processLifecycle) Restart(ctx context.Context) error {
	if err := l.Stop(ctx); err != nil {
		return err
	}
	return l.Start(ctx)
}

// --- вспомогательные функции ---

func writePID(path string, pid int) error {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil // нет файла — нет PID
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("невалидный PID в %s: %w", path, err)
	}
	return pid, nil
}

// processAlive возвращает true если процесс с данным PID существует в /proc.
func processAlive(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

// waitAlive опрашивает /proc/<pid> до timeout.
func waitAlive(ctx context.Context, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if processAlive(pid) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("процесс %d не появился в /proc за %s", pid, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(startPollPeriod):
		}
	}
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

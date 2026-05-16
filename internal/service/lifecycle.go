package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

const (
	stopGracePeriod = 10 * time.Second
	startPollPeriod = 200 * time.Millisecond
	startTimeout    = 5 * time.Second
	// stabilizationDelay — пауза после waitAlive перед второй проверкой /proc.
	// /proc/<pid> существует немедленно после fork, но процесс может упасть
	// через миллисекунды (битый конфиг, неизвестный CLI-флаг). Этот промежуток
	// ловит ранний crash, чтобы Start() не возвращал успех для мёртвого процесса.
	stabilizationDelay = 500 * time.Millisecond
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
	// StderrPath — если задан, stdout+stderr процесса дописываются в этот файл.
	// Нужно для диагностики ранних crash'ей: sing-box может упасть до открытия
	// своего собственного лог-файла, унося stderr в /dev/null.
	StderrPath string
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

	var stderrFile *os.File
	if l.cfg.StderrPath != "" {
		f, openErr := os.OpenFile(l.cfg.StderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
		if openErr != nil {
			log.L().Warn("не удалось открыть stderr-файл для процесса", "service", l.cfg.Name, "path", l.cfg.StderrPath, "err", openErr)
		} else {
			stderrFile = f
			cmd.Stdout = f
			cmd.Stderr = f
		}
	}

	if err := cmd.Start(); err != nil {
		if stderrFile != nil {
			_ = stderrFile.Close()
		}
		return fmt.Errorf("service %s: запуск: %w", l.cfg.Name, err)
	}
	// fd удерживается дочерним процессом; родитель свой может закрыть.
	if stderrFile != nil {
		_ = stderrFile.Close()
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

	// Stabilization-проверка: процесс может упасть сразу после fork (битый конфиг).
	// Ждём stabilizationDelay и проверяем повторно — иначе Start() вернёт успех
	// для процесса, которого уже нет.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(stabilizationDelay):
	}
	if !processAlive(pid, filepath.Base(l.cfg.BinPath)) {
		if rmErr := os.Remove(l.cfg.PIDFile); rmErr != nil {
			log.L().Warn("не удалось удалить PID-файл после ранней смерти", "service", l.cfg.Name, "err", rmErr)
		}
		hint := fmt.Sprintf("см. логи %s", l.cfg.BinPath)
		if l.cfg.StderrPath != "" {
			hint = fmt.Sprintf("см. stderr-лог %s", l.cfg.StderrPath)
		}
		return fmt.Errorf("service %s: процесс упал сразу после старта (%s)", l.cfg.Name, hint)
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

	binName := filepath.Base(l.cfg.BinPath)
	deadline := time.Now().Add(stopGracePeriod)
	for time.Now().Before(deadline) {
		if !processAlive(st.PID, binName) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if processAlive(st.PID, binName) {
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
	if !processAlive(pid, filepath.Base(l.cfg.BinPath)) {
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
	return atomicfs.WriteFileAtomic(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
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

// processExists возвращает true если /proc/<pid> существует — процесс есть в
// таблице, даже если он zombie/dead. Используется в waitAlive: cmd.Start()
// мог вернуть успех, потом процесс упал через мс — наша задача убедиться,
// что fork прошёл (PID валиден); живёт ли он сейчас — отдельная проверка
// через processAlive в stabilization-шаге.
func processExists(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

// commReader читает comm-имя процесса по PID. Интерфейс для подмены в тестах.
type commReader interface {
	readComm(pid int) (string, error)
}

// osCommReader — боевая реализация через /proc.
type osCommReader struct{}

func (osCommReader) readComm(pid int) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\n"), nil
}

// defaultCommReader используется во всех вызовах prodiction-кода.
var defaultCommReader commReader = osCommReader{}

// processAlive возвращает true если процесс с данным PID существует, НЕ зомби,
// и его comm-имя совпадает с expectedName (защита от PID-reuse).
//
// expectedName сравнивается с /proc/<pid>/comm, ядро обрезает имя до 15 байт,
// поэтому используем prefix-match: ожидаемое имя сравнивается с первыми
// len(expectedName) байтами comm (или наоборот, если comm короче).
//
// Если /proc/<pid>/comm недоступен (процесс умер между шагами) → false без ошибки.
func processAlive(pid int, expectedName string) bool {
	return processAliveWith(pid, expectedName, defaultCommReader)
}

// processAliveWith — тестируемая версия с подменяемым commReader.
func processAliveWith(pid int, expectedName string, cr commReader) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	// /proc/<pid>/status содержит строку "State:\tX (descriptive)".
	// Z = zombie, X = dead. Любое другое — живой.
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "State:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			switch fields[1] {
			case "Z", "X":
				return false
			}
		}
		break
	}

	// Проверяем /proc/<pid>/comm для защиты от PID-reuse.
	// Ядро обрезает comm до 15 символов; expectedName может быть длиннее.
	if expectedName != "" {
		comm, commErr := cr.readComm(pid)
		if commErr != nil {
			// /proc/<pid>/comm недоступен — процесс умер между шагами
			return false
		}
		// prefix-match: сравниваем первые min(len(expectedName),15) байт
		const commMaxLen = 15
		expected := expectedName
		if len(expected) > commMaxLen {
			expected = expected[:commMaxLen]
		}
		actual := comm
		if len(actual) > commMaxLen {
			actual = actual[:commMaxLen]
		}
		if actual != expected {
			return false
		}
	}

	return true
}

// waitAlive опрашивает /proc/<pid> до timeout. Принимает любое /proc/<pid>
// (включая zombie) — это подтверждает, что fork прошёл; реальную проверку
// "живой и стабильный" делает stabilization-шаг через processAlive.
func waitAlive(ctx context.Context, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if processExists(pid) {
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

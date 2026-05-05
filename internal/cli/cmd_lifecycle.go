package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/ndm"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func init() {
	Register(Cmd{Long: "--start", Help: "запустить sign-craze", Handler: handleStart})
	Register(Cmd{Long: "--stop", Help: "остановить sign-craze", Handler: handleStop})
	Register(Cmd{Short: "-r", Long: "--restart", Help: "перезапустить sign-craze", Handler: handleRestart})
	Register(Cmd{Long: "--service-start", Help: "(internal) запуск из init.d", Handler: handleServiceStart, Hidden: true})
}

func handleStart(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error { return doStart(ctx) })
}

func doStart(ctx context.Context) error {
	st, err := loadState()
	if err != nil {
		return fmt.Errorf("--start: %w", err)
	}

	// Pre-check установки.
	if _, statErr := os.Stat(singbox.DefaultBinPath); statErr != nil {
		return fmt.Errorf("--start: sing-box не установлен (запустите --install)")
	}

	// Гарантируем актуальность config.json: ручная подмена /opt/sbin/sign-craze
	// в обход --update-core может оставить на диске устаревший конфиг (например,
	// со старым TUN MTU). ensureConfigFresh — fast-path: если рендер совпадает
	// с диском, пропускаем `sing-box check`, иначе регенерируем с валидацией.
	if regenErr := ensureConfigFresh(ctx, st); regenErr != nil {
		return fmt.Errorf("--start: regenerate config: %w", regenErr)
	}

	// В режиме ModePolicy: гарантировать наличие IP-policy в Keenetic RCI
	// и закешировать актуальный mark в state. Mark может смениться при reboot
	// или если юзер удалил policy через UI — поэтому читаем при каждом старте.
	if st.Mode == types.ModePolicy {
		if pErr := ensureKeeneticPolicy(ctx, st); pErr != nil {
			return fmt.Errorf("--start: %w", pErr)
		}
	}

	// Применить firewall (после ensureKeeneticPolicy: PolicyMark уже в state).
	applier, err := newFirewallApplier(st)
	if err != nil {
		return err
	}
	if applyErr := applier.Apply(ctx, st.Mode); applyErr != nil {
		return fmt.Errorf("--start: firewall: %w", applyErr)
	}

	// Восстановить дамп ipset (после ребута). Не фатально — может быть
	// первый запуск до --update-geo, тогда ipset остаются пустыми
	// (safety-fixes #17).
	if rstErr := firewall.RestoreIPSet(ctx, newRunner(), firewall.DefaultDumpFile); rstErr != nil {
		log.L().Warn("--start: восстановление ipset не удалось", "err", rstErr)
	}

	// Pre-cleanup: убрать сталый signbox-tun если остался от предыдущего
	// failed --start (sing-box убит SIGKILL до закрытия TUN fd, kernel на slow
	// MIPS не успевает зачистить netdev). Без этого следующий sing-box падает
	// FATAL: TUNSETIFF: device or resource busy.
	firewall.ForceDeleteTUNDevice(ctx, newRunner(), firewall.TUNDeviceName)

	// Старт sing-box. sing-box создаёт TUN-интерфейс при инициализации
	// tun-inbound, поэтому подключение route в TUN откладывается до AttachTUN.
	sbLC := newSingboxLifecycle()
	if startErr := sbLC.Start(ctx); startErr != nil {
		if rmErr := applier.Remove(ctx); rmErr != nil {
			log.L().Warn("--start: откат firewall не удался", "err", rmErr)
		}
		return fmt.Errorf("--start: sing-box: %w", startErr)
	}

	// Дождаться появления TUN-интерфейса и установить default-route в нашу таблицу.
	if attachErr := applier.AttachTUN(ctx, firewall.TUNDeviceName); attachErr != nil {
		// Подсветить причину: чаще всего sing-box упал или просто медленно
		// инициализирует TUN. Tail sing-box.log даёт юзеру немедленный сигнал
		// без необходимости лезть в /opt/var/log/sign-craze/ руками.
		if tail := lastSingboxLogLines(20); tail != "" {
			log.L().Warn("--start: последние строки sing-box.log", "tail", tail)
		}
		if stopErr := sbLC.Stop(ctx); stopErr != nil {
			log.L().Warn("--start: остановка sing-box после ошибки AttachTUN", "err", stopErr)
		}
		// Post-stop cleanup: kernel на slow MIPS может не освободить netdev
		// после kill — следующая попытка получит EBUSY.
		firewall.ForceDeleteTUNDevice(ctx, newRunner(), firewall.TUNDeviceName)
		if rmErr := applier.Remove(ctx); rmErr != nil {
			log.L().Warn("--start: откат firewall не удался", "err", rmErr)
		}
		return fmt.Errorf("--start: TUN attach: %w", attachErr)
	}

	// Опциональный старт nfqws2.
	if st.DPIEnabled {
		// Регенерация nfqws2.conf + hostlist на каждом старте: гарантирует
		// что конфиг отражает актуальный st.DPITargets даже если юзер изменил
		// targets без --restart, или конфиг был удалён вручную.
		if regenErr := ensureDPIConfigFresh(ctx, st); regenErr != nil {
			log.L().Warn("--start: регенерация nfqws2.conf не удалась", "err", regenErr)
		}
		dpiLC := newDPILifecycle()
		if dpiErr := dpiLC.Start(ctx); dpiErr != nil {
			log.L().Warn("--start: nfqws2 не стартовал, продолжаем без DPI", "err", dpiErr)
		}
	}

	sbStat, statErr := sbLC.Status(ctx)
	if statErr != nil {
		log.L().Warn("--start: чтение статуса sing-box", "err", statErr)
	}
	fmt.Printf("Сервис запущен (sing-box pid=%d, режим=%s)\n", sbStat.PID, st.Mode)
	return nil
}

func handleStop(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error { return doStop(ctx) })
}

func doStop(ctx context.Context) error {
	// Watchdog daemon — отдельный процесс из init.d shim. Если его не убить
	// первым, он перезапишет state.json и переапплит firewall между нашим
	// Remove'ом и вернёт правила в живое состояние через 30s.
	stopWatchdog(ctx)

	dpiLC := newDPILifecycle()
	if err := dpiLC.Stop(ctx); err != nil {
		log.L().Debug("--stop: nfqws2 stop", "err", err)
	}

	sbLC := newSingboxLifecycle()
	if err := sbLC.Stop(ctx); err != nil {
		log.L().Debug("--stop: sing-box stop", "err", err)
	}

	// Принудительно зачистить TUN-интерфейс. На slow MIPS Keenetic kernel
	// иногда не освобождает netdev сразу после kill sing-box — без этого
	// следующий --start падает FATAL: TUNSETIFF: device or resource busy.
	firewall.ForceDeleteTUNDevice(ctx, newRunner(), firewall.TUNDeviceName)

	// Удалить firewall — даже если state нечитаем.
	st, err := loadState()
	if err != nil {
		st = state.Default()
	}
	applier, err := newFirewallApplier(st)
	if err != nil {
		return fmt.Errorf("--stop: firewall init: %w", err)
	}
	if err := applier.Remove(ctx); err != nil {
		return fmt.Errorf("--stop: firewall remove: %w", err)
	}

	fmt.Println("Сервис остановлен")
	return nil
}

func handleRestart(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		if err := doStop(ctx); err != nil {
			log.L().Warn("--restart: ошибка stop, продолжаем", "err", err)
		}
		return doStart(ctx)
	})
}

func handleServiceStart(ctx context.Context, _ []string) error {
	// init.d-вариант --start: ждём сети, без stdout-вывода.
	// Таймаут читается из state.BootTimeoutSec (default 60s) — на медленных
	// роутерах с USB-mount после init.d 30s могло не хватать (safety-fixes #12).
	timeout := 60 * time.Second
	if st, err := loadState(); err == nil && st.BootTimeoutSec > 0 {
		timeout = time.Duration(st.BootTimeoutSec) * time.Second
	}
	log.L().Info("service-start: ожидание сети", "timeout", timeout)
	if err := waitDefaultRoute(ctx, timeout); err != nil {
		log.L().Error("service-start: сеть недоступна", "err", err)
		return err
	}
	return withLock(ctx, func() error { return doStart(ctx) })
}

// ensureKeeneticPolicy гарантирует наличие IP-policy в Keenetic RCI.
//
// Действия:
//  1. Определить WAN-интерфейс через RCI (или взять закешированный из state).
//  2. EnsurePolicy(name, description, wanIface) — идемпотентно создаст или
//     вернёт существующую policy.
//  3. SaveConfig — записать в startup-config (иначе policy не переживёт reboot).
//  4. Обновить state.PolicyMark / PolicyTable / WANInterface.
//
// При недоступном RCI (например, sign-craze запущен в контейнере для CI)
// функция возвращает понятную ошибку — оператор может временно переключиться
// в `--mode full`.
func ensureKeeneticPolicy(ctx context.Context, st *state.State) error {
	client := ndm.NewClient()

	wan := st.WANInterface
	if wan == "" {
		detected, err := client.DetectWANInterface(ctx)
		if err != nil {
			return fmt.Errorf("ndm: автодетект WAN: %w (укажите вручную через state.WANInterface)", err)
		}
		wan = detected
	}

	info, err := client.EnsurePolicy(ctx, st.PolicyName, st.PolicyName, wan)
	if err != nil {
		return fmt.Errorf("ndm: EnsurePolicy: %w", err)
	}
	if err := client.SaveConfig(ctx); err != nil {
		log.L().Warn("ndm: SaveConfig не удался; policy не переживёт перезагрузку", "err", err)
	}

	st.PolicyMark = info.Mark
	st.PolicyTable = info.Table4
	st.WANInterface = wan
	if err := saveState(st); err != nil {
		log.L().Warn("ndm: state save не удался", "err", err)
	}
	log.L().Info("ndm: policy готова",
		"name", info.Name, "mark", fmt.Sprintf("0x%x", info.Mark),
		"table4", info.Table4, "wan", wan,
	)
	return nil
}

// singboxLogPath — путь sing-box собственного лог-файла. Хардкодится в
// шаблоне config (`internal/singbox/templates/tun.json.tmpl`). Используется
// для диагностического tail при ошибке AttachTUN.
const singboxLogPath = "/opt/var/log/sign-craze/sing-box.log"

// lastSingboxLogLines возвращает последние n непустых строк sing-box.log.
// Возвращает пустую строку при ошибке чтения — caller использует это как
// сигнал «нет данных, не логировать tail».
func lastSingboxLogLines(n int) string {
	const tailWindow = 8 * 1024 // 8KB — достаточно для ~80 строк sing-box

	f, err := os.Open(singboxLogPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return ""
	}
	size := info.Size()
	off := int64(0)
	if size > tailWindow {
		off = size - tailWindow
	}
	buf := make([]byte, size-off)
	if _, err := f.ReadAt(buf, off); err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	// Если читали с середины файла, первая строка обрезана — отбросить.
	if off > 0 && len(lines) > 1 {
		lines = lines[1:]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

func waitDefaultRoute(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		res, err := newRunner().Run(ctx, "ip", "route", "show", "default")
		if err == nil && strings.Contains(string(res.Stdout), "default ") {
			return nil
		}
		time.Sleep(time.Second)
	}
	return errors.New("таймаут ожидания маршрута по умолчанию")
}

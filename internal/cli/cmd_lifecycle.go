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
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
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
	if _, statErr := os.Stat(configPath()); statErr != nil {
		return fmt.Errorf("--start: конфиг %s не найден", configPath())
	}

	// Применить firewall.
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

	// Старт sing-box.
	sbLC := newSingboxLifecycle()
	if startErr := sbLC.Start(ctx); startErr != nil {
		if rmErr := applier.Remove(ctx); rmErr != nil {
			log.L().Warn("--start: откат firewall не удался", "err", rmErr)
		}
		return fmt.Errorf("--start: sing-box: %w", startErr)
	}

	// Опциональный старт nfqws2.
	if st.DPIEnabled {
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
	dpiLC := newDPILifecycle()
	if err := dpiLC.Stop(ctx); err != nil {
		log.L().Debug("--stop: nfqws2 stop", "err", err)
	}

	sbLC := newSingboxLifecycle()
	if err := sbLC.Stop(ctx); err != nil {
		log.L().Debug("--stop: sing-box stop", "err", err)
	}

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

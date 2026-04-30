package firewall

import (
	"context"
	"fmt"
	"strings"
	"time"

	scerrors "github.com/kittylabassistant/sign-craze/internal/errors"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// CheckFWMarkAvailable проверяет, что указанный fwmark не занят другим
// инструментом (XKeen / самописные скрипты) с другой таблицей маршрутизации.
// Если правило с тем же fwmark указывает на нашу таблицу — это идемпотентно
// и возвращает nil. Иначе возвращает ErrFWMarkConflict с описанием конфликта
// (safety-fixes #3).
func CheckFWMarkAvailable(ctx context.Context, runner exectx.Runner, fwmark uint32, ourTable int) error {
	res, err := runner.Run(ctx, "ip", "rule", "show")
	if err != nil {
		// Логируем фактическую ошибку и продолжаем: на роутере без CAP_NET_ADMIN
		// (Docker без --privileged, тестовые окружения) `ip rule` падает —
		// блокировать установку из-за этого нельзя. Но команда должна быть видна
		// в логе для диагностики.
		log.L().Warn("firewall: pre-flight ip rule show недоступен, проверка fwmark пропущена", "err", err)
		return nil
	}
	needle := fmt.Sprintf("fwmark 0x%x", fwmark)
	ourLookup := fmt.Sprintf("lookup %d", ourTable)
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		if !strings.Contains(line, needle) {
			continue
		}
		if strings.Contains(line, ourLookup) {
			// Наше правило, идемпотентность — OK.
			continue
		}
		return fmt.Errorf("%w: %s (ожидалась таблица %d; возможно конфликт с XKeen)",
			scerrors.ErrFWMarkConflict, strings.TrimSpace(line), ourTable)
	}
	return nil
}

// EnsureIPRule добавляет правило fwmark → table если оно отсутствует. Идемпотентно.
func EnsureIPRule(ctx context.Context, runner exectx.Runner, fwmark uint32, table, priority int) error {
	if ipRuleExists(ctx, runner, fwmark, table) {
		return nil
	}
	args := []string{
		"rule", "add",
		"fwmark", fmt.Sprintf("0x%x", fwmark),
		"table", fmt.Sprintf("%d", table),
		"priority", fmt.Sprintf("%d", priority),
	}
	if _, err := runner.Run(ctx, "ip", args...); err != nil {
		return fmt.Errorf("firewall: добавление ip rule fwmark 0x%x table %d: %w", fwmark, table, err)
	}
	log.L().Debug("firewall: ip rule добавлено", "fwmark", fmt.Sprintf("0x%x", fwmark), "table", table)
	return nil
}

// DeleteIPRule удаляет правило fwmark → table. Идемпотентно.
func DeleteIPRule(ctx context.Context, runner exectx.Runner, fwmark uint32, table int) error {
	if !ipRuleExists(ctx, runner, fwmark, table) {
		return nil
	}
	args := []string{
		"rule", "del",
		"fwmark", fmt.Sprintf("0x%x", fwmark),
		"table", fmt.Sprintf("%d", table),
	}
	if _, err := runner.Run(ctx, "ip", args...); err != nil {
		return fmt.Errorf("firewall: удаление ip rule fwmark 0x%x table %d: %w", fwmark, table, err)
	}
	log.L().Debug("firewall: ip rule удалено", "fwmark", fmt.Sprintf("0x%x", fwmark), "table", table)
	return nil
}

// EnsureTUNRoute добавляет default-маршрут через TUN-интерфейс sing-box в указанную
// таблицу. Идемпотентно через `ip route replace` (создаёт если нет, переустанавливает
// если есть — без RTNETLINK errors на busybox-`ip` Keenetic).
//
// Должен вызываться ПОСЛЕ старта sing-box и появления TUN-интерфейса
// (см. WaitForInterface), иначе `ip route` падает «Cannot find device».
func EnsureTUNRoute(ctx context.Context, runner exectx.Runner, dev string, table int) error {
	args := []string{
		"route", "replace",
		"default",
		"dev", dev,
		"table", fmt.Sprintf("%d", table),
	}
	if _, err := runner.Run(ctx, "ip", args...); err != nil {
		return fmt.Errorf("firewall: добавление default через %s в таблицу %d: %w", dev, table, err)
	}
	log.L().Debug("firewall: TUN route добавлен/обновлён", "dev", dev, "table", table)
	return nil
}

// DeleteTUNRoute удаляет default-маршрут через dev из таблицы. Идемпотентно.
//
// Используем безусловный `ip route del default table N` — если маршрута нет,
// busybox-`ip` возвращает ошибку, которую логируем как Debug и игнорируем
// (стандартный паттерн для idempotent cleanup).
func DeleteTUNRoute(ctx context.Context, runner exectx.Runner, dev string, table int) error {
	args := []string{
		"route", "del",
		"default",
		"dev", dev,
		"table", fmt.Sprintf("%d", table),
	}
	if _, err := runner.Run(ctx, "ip", args...); err != nil {
		log.L().Debug("firewall: удаление TUN route (возможно отсутствует)", "dev", dev, "table", table, "err", err)
		return nil
	}
	log.L().Debug("firewall: TUN route удалён", "dev", dev, "table", table)
	return nil
}

// ipRuleExists проверяет наличие правила fwmark → table в выводе `ip rule show`.
func ipRuleExists(ctx context.Context, runner exectx.Runner, fwmark uint32, table int) bool {
	res, err := runner.Run(ctx, "ip", "rule", "show")
	if err != nil {
		return false
	}
	needle := fmt.Sprintf("fwmark 0x%x lookup %d", fwmark, table)
	return strings.Contains(string(res.Stdout), needle)
}

// WaitForInterface поллит `ip link show <dev>` пока интерфейс не появится или
// не истечёт timeout. Используется applier'ом после Start sing-box (sing-box сам
// создаёт TUN-интерфейс при инициализации tun-inbound, это занимает ~0.5–2s
// на медленном MIPS).
//
// Возвращает nil если интерфейс появился, ошибку timeout — если нет.
// Ошибки runner.Run между поллами игнорируются (busybox `ip link show <dev>`
// возвращает exit 1 пока интерфейса нет — это и есть индикатор «ещё нет»).
func WaitForInterface(ctx context.Context, runner exectx.Runner, dev string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		res, err := runner.Run(ctx, "ip", "link", "show", dev)
		if err == nil && strings.Contains(string(res.Stdout), dev) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("firewall: интерфейс %s не появился за %s", dev, timeout)
}

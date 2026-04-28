package firewall

import (
	"context"
	"fmt"
	"strings"

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
		// Не удалось прочитать ip rule — лучше пропустить, чем заблокировать
		// установку (например, в Docker без CAP_NET_ADMIN). Логируется выше.
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

// EnsureLocalRoute добавляет маршрут local 0.0.0.0/0 dev lo в таблицу. Идемпотентно.
func EnsureLocalRoute(ctx context.Context, runner exectx.Runner, table int) error {
	if localRouteExists(ctx, runner, table) {
		return nil
	}
	args := []string{
		"route", "add",
		"local", "0.0.0.0/0",
		"dev", "lo",
		"table", fmt.Sprintf("%d", table),
	}
	if _, err := runner.Run(ctx, "ip", args...); err != nil {
		return fmt.Errorf("firewall: добавление local route в таблицу %d: %w", table, err)
	}
	log.L().Debug("firewall: local route добавлен", "table", table)
	return nil
}

// DeleteLocalRoute удаляет маршрут local 0.0.0.0/0 dev lo из таблицы. Идемпотентно.
func DeleteLocalRoute(ctx context.Context, runner exectx.Runner, table int) error {
	if !localRouteExists(ctx, runner, table) {
		return nil
	}
	args := []string{
		"route", "del",
		"local", "0.0.0.0/0",
		"dev", "lo",
		"table", fmt.Sprintf("%d", table),
	}
	if _, err := runner.Run(ctx, "ip", args...); err != nil {
		return fmt.Errorf("firewall: удаление local route из таблицы %d: %w", table, err)
	}
	log.L().Debug("firewall: local route удалён", "table", table)
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

// localRouteExists проверяет наличие local-маршрута в таблице.
func localRouteExists(ctx context.Context, runner exectx.Runner, table int) bool {
	res, err := runner.Run(ctx, "ip", "route", "show", "table", fmt.Sprintf("%d", table))
	if err != nil {
		return false
	}
	return strings.Contains(string(res.Stdout), "local")
}

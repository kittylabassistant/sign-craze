package firewall

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// ErrFWMarkConflict — fwmark уже используется другим инструментом
// (например, XKeen) с другой таблицей маршрутизации.
var ErrFWMarkConflict = errors.New("fwmark уже занят")

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
			ErrFWMarkConflict, strings.TrimSpace(line), ourTable)
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

// EnsureTUNRoute добавляет default-маршрут через TUN-интерфейс активного ядра
// (sing-box, либо mihomo в TUN-mode) в указанную таблицу. Идемпотентно через
// `ip route replace` (создаёт если нет, переустанавливает если есть — без
// RTNETLINK errors на busybox-`ip` Keenetic).
//
// Должен вызываться ПОСЛЕ старта ядра, когда TUN-интерфейс уже создан
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

// ForceDeleteTUNDevice безусловно удаляет TUN-интерфейс dev через `ip tuntap del`.
// Идемпотентно — отсутствующий интерфейс не считается ошибкой.
//
// Зачем: ядро создаёт TUN при старте через TUNSETIFF на /dev/net/tun.
// Если процесс ядра убит SIGKILL раньше чем успел нормально закрыть fd,
// или если ядро по какой-то причине не успевает зачистить netdev (наблюдалось
// на slow MIPS Keenetic после AttachTUN-таймаута + sign-box Stop), интерфейс
// остаётся "висеть" в kernel-таблице. Следующий sign-box при старте получает
// `TUNSETIFF: device or resource busy` и падает FATAL до открытия лог-файла.
// Этот хелпер вызывается перед стартом ядра (cleanup) и после остановки
// (post-stop), гарантируя пустой netdev-namespace для signbox-tun.
func ForceDeleteTUNDevice(ctx context.Context, runner exectx.Runner, dev string) {
	if _, err := runner.Run(ctx, "ip", "tuntap", "del", "dev", dev, "mode", "tun"); err != nil {
		// busybox `ip tuntap del` возвращает exit 1 если интерфейс не существует —
		// это и есть нормальный idempotent-кейс, не логируем как warning.
		log.L().Debug("firewall: ForceDeleteTUNDevice (возможно отсутствует)", "dev", dev, "err", err)
		return
	}
	log.L().Debug("firewall: TUN device удалён", "dev", dev)
}

// EnsureLocalRoute добавляет маршрут `local 0.0.0.0/0 dev lo table <table>`
// через `ip route replace`. Необходим для TPROXY-режима: без него ядро не может
// доставить TPROXY-пакет в сокет sing-box (EHOSTUNREACH на lookup таблицы).
// Идемпотентно через `ip route replace`.
func EnsureLocalRoute(ctx context.Context, runner exectx.Runner, table int) error {
	args := []string{
		"route", "replace",
		"local", "0.0.0.0/0",
		"dev", "lo",
		"table", fmt.Sprintf("%d", table),
	}
	if _, err := runner.Run(ctx, "ip", args...); err != nil {
		return fmt.Errorf("firewall: добавление local route в таблицу %d: %w", table, err)
	}
	log.L().Debug("firewall: local route добавлен/обновлён", "table", table)
	return nil
}

// DeleteLocalRoute удаляет маршрут `local 0.0.0.0/0 dev lo table <table>`. Идемпотентно.
func DeleteLocalRoute(ctx context.Context, runner exectx.Runner, table int) error {
	args := []string{
		"route", "del",
		"local", "0.0.0.0/0",
		"dev", "lo",
		"table", fmt.Sprintf("%d", table),
	}
	if _, err := runner.Run(ctx, "ip", args...); err != nil {
		// Отсутствие маршрута — штатный idempotent-кейс, не ошибка.
		log.L().Debug("firewall: удаление local route (возможно отсутствует)", "table", table, "err", err)
		return nil
	}
	log.L().Debug("firewall: local route удалён", "table", table)
	return nil
}

// EnsureLinkUp поднимает link state интерфейса dev (`ip link set <dev> up`).
// Идемпотентно — повторный вызов на уже UP-интерфейсе не считается ошибкой
// (kernel молча возвращает успех).
//
// Зачем: sing-box создаёт TUN через TUNSETIFF (netdev появляется в state
// DOWN), затем отдельным ioctl ставит IFF_UP. На slow MIPS между этими
// шагами есть gap, в котором WaitForInterface уже видит интерфейс по имени,
// а EnsureTUNRoute падает с "RTNETLINK answers: Network is down". Явный
// `ip link set up` закрывает окно гонки без необходимости расширять
// WaitForInterface парсингом state-флагов.
func EnsureLinkUp(ctx context.Context, runner exectx.Runner, dev string) error {
	if _, err := runner.Run(ctx, "ip", "link", "set", dev, "up"); err != nil {
		return fmt.Errorf("firewall: ip link set %s up: %w", dev, err)
	}
	log.L().Debug("firewall: link UP", "dev", dev)
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
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("firewall: интерфейс %s не появился за %s", dev, timeout)
}

package firewall

import (
	"context"
	"fmt"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
)

// CheckRequiredIptablesModules verifies iptables на хосте имеет нужные
// match/target модули. На стоковой Keenetic-прошивке часто отсутствуют
// xt_TPROXY и xt_set — без них Apply упадёт с
// `iptables: unknown option "--tproxy-port"` или
// `iptables: No chain/target/match by that name`.
//
// Probe: `iptables -j TPROXY --help` / `iptables -m set --help`. Если
// модуль есть — iptables печатает help в stdout (exit 0). Если нет —
// stderr содержит "Couldn't find target/match" (exit 2).
//
// Возвращает ошибку с актуальным opkg-инструктажем для пользователя.
func CheckRequiredIptablesModules(ctx context.Context, runner exectx.Runner) error {
	type mod struct {
		flag  string // -j или -m
		name  string
		opkg  string // имя пакета Entware
		human string
	}
	required := []mod{
		{"-j", "TPROXY", "iptables-mod-tproxy", "TPROXY (target xt_TPROXY)"},
		{"-m", "set", "iptables-mod-ipset", "set (match xt_set)"},
	}

	var missing []mod
	for _, m := range required {
		if !iptablesModuleAvailable(ctx, runner, m.flag, m.name) {
			missing = append(missing, m)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	var names, pkgs []string
	for _, m := range missing {
		names = append(names, m.human)
		pkgs = append(pkgs, m.opkg)
	}
	return fmt.Errorf(
		"iptables не имеет требуемых модулей: %s.\n"+
			"установите через Entware:\n"+
			"  opkg update && opkg install %s ipset",
		strings.Join(names, ", "),
		strings.Join(pkgs, " "),
	)
}

// iptablesModuleAvailable пробует `iptables {flag} {name} --help`.
// Модуль есть → exit 0 без "Couldn't find" в stderr.
// Модуль отсутствует → exit != 0 или stderr с "Couldn't find".
func iptablesModuleAvailable(ctx context.Context, runner exectx.Runner, flag, name string) bool {
	res, err := runner.Run(ctx, "iptables", flag, name, "--help")
	if err != nil {
		// busybox iptables может выйти с ненулевым кодом для
		// неизвестного модуля. Проверим stderr — если там типичная
		// фраза, значит модуль точно отсутствует.
		stderr := string(res.Stderr)
		if strings.Contains(stderr, "Couldn't find") || strings.Contains(stderr, "No such file") {
			return false
		}
		// Иные ошибки трактуем как недоступность во избежание ложного
		// успеха (лучше явно ругнуться, чем упасть на Apply).
		return false
	}
	if strings.Contains(string(res.Stderr), "Couldn't find") {
		return false
	}
	return true
}

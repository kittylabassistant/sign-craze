package firewall

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// Sysctl-узлы Keenetic-специфичной аппаратной ускорилки conntrack.
//
// Когда они = 1 (default), kernel кэширует RTCACHE первой пары пакетов flow
// и обходит mangle/iptables для последующих пакетов того же flow. Это ломает
// policy-routing через signbox-tun: ACK/PSH-пакеты existing flow получают
// устаревший route (default WAN) вместо актуального fwmark→table 83→TUN.
//
// Симптом: первые пакеты handshake идут через TUN, потом FASTNAT перехватывает,
// reply-direction ACK летит мимо прокси через WAN. Tcpdump на TUN показывает
// retransmits от gvisor без ACK — клиентский ACK ушёл в WAN, до gvisor не дошёл.
//
// На не-Keenetic-системах эти узлы отсутствуют — Disable/Restore становятся no-op.
const (
	fastNATSysctl   = "/proc/sys/net/netfilter/nf_conntrack_fastnat"
	fastRouteSysctl = "/proc/sys/net/netfilter/nf_conntrack_fastroute"

	// fastPathStateFile хранит исходные значения sysctl до Disable, чтобы
	// Restore вернул их обратно. JSON: {"fastnat":"1","fastroute":"1"}.
	fastPathStateFile = "/opt/var/lib/sign-craze/fastpath.state"
)

type fastPathState struct {
	FastNAT   string `json:"fastnat,omitempty"`
	FastRoute string `json:"fastroute,omitempty"`
}

// DisableFastPath отключает Keenetic FASTNAT/FASTROUTE и сохраняет старые
// значения в state-файл для последующего Restore. На системах без этих sysctl
// — no-op (oba ENOENT).
func DisableFastPath() error {
	return disableFastPathAt(fastNATSysctl, fastRouteSysctl, fastPathStateFile)
}

func disableFastPathAt(natPath, routePath, statePath string) error {
	nat, natErr := readSysctl(natPath)
	route, routeErr := readSysctl(routePath)

	natMissing := errors.Is(natErr, fs.ErrNotExist)
	routeMissing := errors.Is(routeErr, fs.ErrNotExist)
	if natMissing && routeMissing {
		log.L().Debug("firewall: fastpath sysctl недоступен (не Keenetic), пропуск")
		return nil
	}
	if natErr != nil && !natMissing {
		return fmt.Errorf("firewall: чтение %s: %w", natPath, natErr)
	}
	if routeErr != nil && !routeMissing {
		return fmt.Errorf("firewall: чтение %s: %w", routePath, routeErr)
	}

	// Сохранить исходные значения только если state-файла ещё нет — иначе
	// повторный Apply (после нашего же первого) затрёт оригиналы нашими же 0.
	if _, statErr := os.Stat(statePath); errors.Is(statErr, fs.ErrNotExist) {
		st := fastPathState{FastNAT: nat, FastRoute: route}
		data, err := json.Marshal(st)
		if err != nil {
			return fmt.Errorf("firewall: сериализация fastpath state: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
			return fmt.Errorf("firewall: создание dir для fastpath state: %w", err)
		}
		if err := atomicfs.WriteFileAtomic(statePath, data, 0o640); err != nil {
			return fmt.Errorf("firewall: сохранение fastpath state: %w", err)
		}
	}

	if !natMissing && nat != "0" {
		if err := writeSysctl(natPath, "0"); err != nil {
			return fmt.Errorf("firewall: отключение fastnat: %w", err)
		}
		log.L().Info("firewall: Keenetic fastnat отключён", "был", nat)
	}
	if !routeMissing && route != "0" {
		if err := writeSysctl(routePath, "0"); err != nil {
			return fmt.Errorf("firewall: отключение fastroute: %w", err)
		}
		log.L().Info("firewall: Keenetic fastroute отключён", "был", route)
	}
	return nil
}

// RestoreFastPath возвращает sysctl в исходное состояние из state-файла.
// Если state-файла нет (Disable никогда не вызывалась) — no-op. Удаляет
// state-файл после восстановления.
func RestoreFastPath() error {
	return restoreFastPathAt(fastNATSysctl, fastRouteSysctl, fastPathStateFile)
}

func restoreFastPathAt(natPath, routePath, statePath string) error {
	data, err := os.ReadFile(statePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("firewall: чтение fastpath state: %w", err)
	}
	var st fastPathState
	if jerr := json.Unmarshal(data, &st); jerr != nil {
		log.L().Warn("firewall: некорректный fastpath state, пропуск восстановления", "err", jerr)
		_ = os.Remove(statePath)
		return nil
	}

	if st.FastNAT != "" {
		if werr := writeSysctl(natPath, st.FastNAT); werr != nil && !errors.Is(werr, fs.ErrNotExist) {
			log.L().Warn("firewall: восстановление fastnat", "err", werr)
		} else if werr == nil {
			log.L().Info("firewall: Keenetic fastnat восстановлен", "значение", st.FastNAT)
		}
	}
	if st.FastRoute != "" {
		if werr := writeSysctl(routePath, st.FastRoute); werr != nil && !errors.Is(werr, fs.ErrNotExist) {
			log.L().Warn("firewall: восстановление fastroute", "err", werr)
		} else if werr == nil {
			log.L().Info("firewall: Keenetic fastroute восстановлен", "значение", st.FastRoute)
		}
	}
	return os.Remove(statePath)
}

func readSysctl(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeSysctl(path, value string) error {
	if _, err := strconv.Atoi(value); err != nil {
		return fmt.Errorf("некорректное sysctl-значение %q", value)
	}
	return os.WriteFile(path, []byte(value+"\n"), 0o644)
}

// enableRouteLocalnet включает net.ipv4.conf.all.route_localnet=1.
// Без этого REDIRECT в loopback-DNAT дропает пакеты с не-loopback src
// как martian. На системах без этого sysctl (не Linux) — no-op.
func enableRouteLocalnet() error {
	const path = "/proc/sys/net/ipv4/conf/all/route_localnet"
	if _, err := os.Stat(path); err != nil {
		return nil // не Linux или sysctl нет — silently skip
	}
	cur, err := readSysctl(path)
	if err != nil || cur == "1" {
		return nil
	}
	return writeSysctl(path, "1")
}

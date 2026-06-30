package firewall

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// probeChainName — короткое имя цепочки, используемое preflight'ом для
// dry-run проверки наличия match/target. Создаётся, в неё добавляется
// тестовое правило, потом цепочка удаляется. Имя короткое и явное —
// чтобы не сломать пользовательские цепочки совпадением имён.
const probeChainName = "signcraze_probe"

// tunAvailableCheck — точка инъекции для unit-тестов, чтобы не требовать
// /dev/net/tun на CI-runner'ах (Docker/Podman без --privileged TUN не выдаёт).
// В production переопределяется на реальную проверку.
var tunAvailableCheck = checkTUNAvailableReal

// CheckTUNAvailable проверяет, что в системе доступен kernel TUN — без него
// sing-box `tun` inbound не запустится. На стоковой Keenetic-прошивке TUN
// обычно включён (CONFIG_TUN=y, используется встроенным OpenVPN/WireGuard),
// но не на всех моделях.
func CheckTUNAvailable() error {
	return tunAvailableCheck()
}

func checkTUNAvailableReal() error {
	fi, err := os.Stat("/dev/net/tun")
	if err != nil {
		return fmt.Errorf("kernel TUN недоступен: %w "+
			"(попробуйте `modprobe tun`; если ядро без CONFIG_TUN — sign-craze несовместим)", err)
	}
	if fi.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("/dev/net/tun не является character device (mode=%v)", fi.Mode())
	}
	return nil
}

// requiredBinaries — внешние бинари, без которых firewall-слой sign-craze не
// работает. В Entware ставятся пакетами `iptables` (даёт iptables +
// iptables-restore) и `ipset`.
var requiredBinaries = []string{"iptables", "iptables-restore", "ipset"}

// CheckRequiredBinaries проверяет доступность iptables/iptables-restore/ipset
// (через `<bin> --version`; PATH расширен на /opt/{sbin,bin} в exectx). При
// отсутствии любого — actionable-ошибка с командой opkg.
//
// Без этой проверки пользователь, поставивший sign-craze через `curl|sh` (минуя
// opkg-зависимости из control.tmpl), получает на `--start` криптичную
// exec-ошибку «not found» вместо понятной подсказки (issue #3).
func CheckRequiredBinaries(ctx context.Context, runner exectx.Runner) error {
	var missing []string
	for _, bin := range requiredBinaries {
		if _, err := runner.Run(ctx, bin, "--version"); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"не найдены обязательные бинари: %s. Установите через Entware: `opkg install iptables ipset`",
			strings.Join(missing, ", "),
		)
	}
	return nil
}

// CheckRequiredIptablesModules проверяет наличие match `set` (xt_set) в iptables.
// MARK target есть всегда (включён в любую iptables-сборку), его не пробуем.
// xt_TPROXY больше не требуется (sign-craze v0.3+ использует TUN-mode).
//
// Probe через **реальный dry-run**: создаём временную цепочку signcraze_probe,
// добавляем туда правило с `-m set --match-set <fake> dst -j MARK --set-mark 0x1`.
// Если match отсутствует — iptables возвращает "No chain/target/match by that
// name". В любом случае чистим цепочку через FlushAndDeleteChain.
//
// Если ipset не установлен или signcraze_ipv4 ещё не создан — это всё равно
// проверка userspace plugin libxt_set.so (iptables ругается на отсутствие
// модуля, не на пустой ipset).
func CheckRequiredIptablesModules(ctx context.Context, runner exectx.Runner) error {
	ipt := New(runner)

	// EnsureChain идемпотентен (игнорирует "already exists"), так что pre-cleanup
	// от предыдущего падения preflight'а не нужен — defer FlushAndDeleteChain
	// почистит за собой при любом исходе.
	if err := ipt.EnsureChain(ctx, "mangle", probeChainName); err != nil {
		return fmt.Errorf("preflight: создание probe-цепочки: %w", err)
	}
	defer func() {
		if cleanupErr := ipt.FlushAndDeleteChain(ctx, "mangle", probeChainName); cleanupErr != nil {
			log.L().Debug("preflight: cleanup probe chain", "err", cleanupErr)
		}
	}()

	// Probe match `set`: нужен xt_set (libxt_set.so + xt_set kernel module).
	// Используем фиктивный set-name "signcraze_probe_set" — match сначала
	// проверяется userspace, ошибка про модуль приходит ДО проверки существования
	// набора, поэтому нет нужды реально создавать ipset.
	res, err := runner.Run(ctx, "iptables",
		"-t", "mangle", "-A", probeChainName,
		"-m", "set", "--match-set", "signcraze_probe_set", "dst",
		"-j", "MARK", "--set-mark", "0x1",
	)
	if err != nil {
		stderr := string(res.Stderr)
		if strings.Contains(stderr, "No chain/target/match by that name") ||
			strings.Contains(stderr, "Couldn't load match") ||
			strings.Contains(stderr, "Couldn't find match") {
			return fmt.Errorf(
				"iptables не имеет match `set` (xt_set/libxt_set.so). " +
					"Установите ipset через Entware: `opkg install ipset`",
			)
		}
		// Если ошибка про несуществующий ipset (`set signcraze_probe_set doesn't exist`)
		// — это означает что match `set` РАБОТАЕТ, просто наш фейковый набор не найден.
		// Это положительный исход probe.
		if strings.Contains(stderr, "doesn't exist") || strings.Contains(stderr, "not exist") {
			return nil
		}
		return fmt.Errorf("preflight: probe iptables match set: %w (stderr: %s)", err, stderr)
	}
	return nil
}

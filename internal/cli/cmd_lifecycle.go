package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/ndm"
	"github.com/kittylabassistant/sign-craze/internal/peer"
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

	c := mustActiveCore()

	// Pre-check установки активного ядра.
	if _, statErr := os.Stat(c.BinaryPath()); statErr != nil {
		return fmt.Errorf("--start: %s не установлен (запустите --install)", c.Name())
	}

	// Pre-start supervised peers (mieru = ADR-0020). Этапы:
	//   1. AllocateAllMieruPorts — выделить LocalSocksPort, persist state.
	//   2. WriteMieruConfig — записать /opt/etc/sign-craze/mieru-<tag>.conf.json.
	//   3. StartMieruPeers — запустить mieru-клиенты ДО рендера sing-box config,
	//      потому что render ссылается на 127.0.0.1:LocalSocksPort.
	// Каждый этап идемпотентен; failure на любом шаге прерывает --start без
	// побочных эффектов на firewall (он ещё не применён).
	if peer.HasMieruOutbounds(st.Outbounds) {
		if err := peer.AllocateAllMieruPorts(st.Outbounds); err != nil {
			return fmt.Errorf("--start: mieru port allocation: %w", err)
		}
		if err := saveState(st); err != nil {
			return fmt.Errorf("--start: persist mieru ports: %w", err)
		}
		for _, ob := range st.Outbounds {
			if ob.Protocol != types.ProtocolMieru {
				continue
			}
			if _, wErr := peer.WriteMieruConfig(peer.MieruConfigDir, ob); wErr != nil {
				return fmt.Errorf("--start: mieru config %q: %w", ob.Tag, wErr)
			}
		}
		if err := peer.StartMieruPeers(ctx, st.Outbounds); err != nil {
			// Откатить ранее запущенные peers — частичный успех нежелателен.
			peer.StopMieruPeers(ctx, st.Outbounds)
			return fmt.Errorf("--start: %w", err)
		}
	}

	// Старт naive-демонов ДО рендера конфига: sing-box при CheckConfig проверяет
	// доступность 127.0.0.1:NaiveListenPort — naive должен быть готов к этому моменту.
	if st.NaiveEnabled {
		for _, ob := range st.Outbounds {
			if ob.Protocol != types.ProtocolNaive {
				continue
			}
			lc, lcErr := newNaiveLifecycleFromOutbound(ob)
			if lcErr != nil {
				log.L().Warn("--start: naive lifecycle build", "tag", ob.Tag, "err", lcErr)
				continue
			}
			if startErr := lc.Start(ctx); startErr != nil {
				log.L().Warn("--start: naive не стартовал", "tag", ob.Tag, "err", startErr)
			} else {
				listenPort := 0
				if ob.Proto != nil {
					listenPort = ob.Proto.NaiveListenPort
				}
				log.L().Info("naive запущен", "tag", ob.Tag, "listen_port", listenPort)
			}
		}
	}

	// Рендеринг и запись конфига активного ядра — единый путь для всех ядер.
	// ensureConfigFreshForCore читает routing.json, прокидывает CoreRenderParams
	// (RoutingConfig + InboundMode + DefaultOutboundTag) в c.RenderConfig.
	//
	// Fast-path bytes.Equal пропускает CheckConfig если конфиг идентичен — это
	// важно на slow MIPS softfloat, где sing-box check / xray test / mihomo -t
	// занимают десятки секунд.
	if regenErr := ensureConfigFreshForCore(ctx, c, st); regenErr != nil {
		return fmt.Errorf("--start: regenerate config (%s): %w", c.Name(), regenErr)
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
		// Откатить уже запущенные mieru peers — без firewall их трафик
		// никуда не пойдёт, держать процессы бессмысленно.
		peer.StopMieruPeers(ctx, st.Outbounds)
		return fmt.Errorf("--start: firewall: %w", applyErr)
	}

	// Восстановить дамп ipset (после ребута). Не фатально — может быть
	// первый запуск до --update-geo, тогда ipset остаются пустыми
	// (safety-fixes #17).
	if rstErr := firewall.RestoreIPSet(ctx, exectx.OS, firewall.DefaultDumpFile); rstErr != nil {
		log.L().Warn("--start: восстановление ipset не удалось", "err", rstErr)
	}

	// Pre-cleanup TUN — только когда активное ядро создаёт TUN-интерфейс.
	// sing-box при state.Inbound!="tproxy" создаёт signbox-tun; xray/mihomo
	// всегда TProxy. needsTUN инкапсулирует эту логику.
	// Убрать сталый signbox-tun если остался от предыдущего failed --start
	// (sing-box убит SIGKILL до закрытия TUN fd, kernel на slow MIPS не успевает
	// зачистить netdev). Без этого следующий sing-box падает FATAL: TUNSETIFF:
	// device or resource busy.
	if needsTUN(c, st) {
		firewall.ForceDeleteTUNDevice(ctx, exectx.OS, firewall.TUNDeviceName)
	}

	// Старт активного ядра. Sing-box создаёт TUN-интерфейс при инициализации
	// tun-inbound — подключение route в TUN откладывается до AttachTUN ниже.
	// Xray/mihomo работают через TProxy + fwmark и не требуют TUN-attach.
	coreLC := c.NewLifecycle()
	if startErr := coreLC.Start(ctx); startErr != nil {
		if rmErr := applier.Remove(ctx); rmErr != nil {
			log.L().Warn("--start: откат firewall не удался", "err", rmErr)
		}
		// Остановить ранее запущенные mieru peers — sing-box не стартанул,
		// peers не имеют потребителя.
		peer.StopMieruPeers(ctx, st.Outbounds)
		return fmt.Errorf("--start: %s: %w", c.Name(), startErr)
	}

	// TUN attach — только когда ядро создаёт TUN. Дождаться появления
	// TUN-интерфейса и установить default-route в нашу таблицу.
	if needsTUN(c, st) {
		if attachErr := applier.AttachTUN(ctx, firewall.TUNDeviceName); attachErr != nil {
			// Подсветить причину: чаще всего sing-box упал или просто медленно
			// инициализирует TUN. Tail sing-box.log даёт юзеру немедленный сигнал
			// без необходимости лезть в /opt/var/log/sign-craze/ руками.
			if tail := lastSingboxLogLines(20); tail != "" {
				log.L().Warn("--start: последние строки sing-box.log", "tail", tail)
			}
			if stopErr := coreLC.Stop(ctx); stopErr != nil {
				log.L().Warn("--start: остановка sing-box после ошибки AttachTUN", "err", stopErr)
			}
			// Post-stop cleanup: kernel на slow MIPS может не освободить netdev
			// после kill — следующая попытка получит EBUSY.
			firewall.ForceDeleteTUNDevice(ctx, exectx.OS, firewall.TUNDeviceName)
			if rmErr := applier.Remove(ctx); rmErr != nil {
				log.L().Warn("--start: откат firewall не удался", "err", rmErr)
			}
			peer.StopMieruPeers(ctx, st.Outbounds)
			return fmt.Errorf("--start: TUN attach: %w", attachErr)
		}
	} else {
		log.L().Info("--start: TProxy режим, TUN attach пропущен", "core", c.Name())
	}

	// Опциональный старт nfqws2.
	if st.DPIEnabled {
		// Регенерация nfqws2.conf + hostlist на каждом старте: гарантирует
		// что конфиг отражает актуальный st.DPITargets даже если юзер изменил
		// targets без --restart, или конфиг был удалён вручную.
		iface, ifaceErr := detectISPInterface(ctx)
		if ifaceErr != nil {
			log.L().Warn("--start: ISP-интерфейс не определён, nfqws2 не стартует", "err", ifaceErr)
		} else {
			if regenErr := writeDPIConfig(iface, st); regenErr != nil {
				log.L().Warn("--start: регенерация nfqws2.conf не удалась", "err", regenErr)
			}
			dpiLC := newDPILifecycleFromState(st, iface)
			if dpiErr := dpiLC.Start(ctx); dpiErr != nil {
				log.L().Warn("--start: nfqws2 не стартовал, продолжаем без DPI", "err", dpiErr)
			}
		}
	}

	coreStat, statErr := coreLC.Status(ctx)
	if statErr != nil {
		log.L().Warn("--start: чтение статуса ядра", "core", c.Name(), "err", statErr)
	}
	fmt.Printf("%s (%s pid=%d, режим=%s)\n", OK("Сервис запущен"), c.Name(), coreStat.PID, st.Mode)
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

	// naive: останавливаем до sing-box (sing-box держит подключение к 127.0.0.1:18443).
	if stopErr := newNaiveLifecycle().Stop(ctx); stopErr != nil {
		log.L().Warn("--stop: naive не остановился чисто", "err", stopErr)
	}

	c := mustActiveCore()
	coreLC := c.NewLifecycle()
	if err := coreLC.Stop(ctx); err != nil {
		log.L().Debug("--stop: core stop", "core", c.Name(), "err", err)
	}

	// Удалить firewall — даже если state нечитаем.
	st, err := loadState()
	if err != nil {
		st = state.Default()
	}

	// Остановить supervised peers (mieru) после sing-box. Порядок важен
	// (Inv-Peer-Order в BEHAVIOR_SPEC §7): sing-box должен закрыть свои
	// SOCKS5-коннекты к mieru до того, как mieru уйдёт.
	if peer.HasMieruOutbounds(st.Outbounds) {
		peer.StopMieruPeers(ctx, st.Outbounds)
	}

	// Принудительно зачистить TUN-интерфейс — только когда ядро создаёт TUN.
	// На slow MIPS Keenetic kernel иногда не освобождает netdev сразу после
	// kill — без этого следующий --start падает FATAL: TUNSETIFF: device or
	// resource busy. Xray/mihomo (и sing-box+tproxy) TUN не создают.
	if needsTUN(c, st) {
		firewall.ForceDeleteTUNDevice(ctx, exectx.OS, firewall.TUNDeviceName)
	}

	applier, err := newFirewallApplier(st)
	if err != nil {
		return fmt.Errorf("--stop: firewall init: %w", err)
	}
	if err := applier.Remove(ctx); err != nil {
		return fmt.Errorf("--stop: firewall remove: %w", err)
	}

	fmt.Println(Info("Сервис остановлен"))
	return nil
}

func handleRestart(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		// UI-демон — отдельный процесс с собственным PID-файлом; doStop/doStart
		// его не трогают. Без явного рестарта старый UI остаётся жить со старой
		// in-memory копией бинаря (актуально после замены sign-craze на диске).
		uiWasRunning := stopUIIfRunning(ctx)

		if err := doStop(ctx); err != nil {
			log.L().Warn("--restart: ошибка stop, продолжаем", "err", err)
		}
		if err := doStart(ctx); err != nil {
			return err
		}

		if uiWasRunning {
			if err := startUI(ctx); err != nil {
				log.L().Warn("--restart: повторный запуск UI не удался", "err", err)
			}
		}
		return nil
	})
}

// uiLifecycleFn — фабрика UI lifecycle, переопределяемая в тестах для
// inj fake без реального PID-файла и форка sign-craze --ui-daemon.
var uiLifecycleFn = uiLifecycle

// stopUIIfRunning останавливает фоновый UI-демон, если он был запущен.
// Возвращает true если процесс был активен и его остановили — caller использует
// флаг чтобы поднять UI обратно после рестарта основного сервиса.
func stopUIIfRunning(ctx context.Context) bool {
	lc, err := uiLifecycleFn()
	if err != nil {
		log.L().Warn("--restart: построение UI lifecycle", "err", err)
		return false
	}
	st, err := lc.Status(ctx)
	if err != nil {
		log.L().Warn("--restart: чтение статуса UI", "err", err)
		return false
	}
	if !st.Running {
		return false
	}
	if err := lc.Stop(ctx); err != nil {
		log.L().Warn("--restart: остановка UI не удалась", "err", err)
		return false
	}
	return true
}

// startUI поднимает UI-демона повторно после рестарта.
func startUI(ctx context.Context) error {
	lc, err := uiLifecycleFn()
	if err != nil {
		return err
	}
	return lc.Start(ctx)
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

	// На холодном boot kernel может не успеть подгрузить xt_NFQUEUE до
	// нашего вызова rc.unslung — `iptables -j NFQUEUE` падает с
	// "No chain/target/match by that name". Retry с backoff: doStart
	// идемпотентен (откатывает свой state при ошибке), а shim S99 идёт
	// после S51dropbear → retry-цикл не зависит и не влияет на SSH 222.
	const (
		moduleTimeout = 60 * time.Second
		moduleBackoff = 3 * time.Second
	)
	deadline := time.Now().Add(moduleTimeout)
	var lastErr error
	for attempt := 1; ; attempt++ {
		err := withLock(ctx, func() error { return doStart(ctx) })
		if err == nil {
			return nil
		}
		if !isMissingNetfilterModuleErr(err) {
			return err
		}
		lastErr = err
		if !time.Now().Add(moduleBackoff).Before(deadline) {
			break
		}
		log.L().Warn("service-start: netfilter module не готов, retry",
			"attempt", attempt, "backoff", moduleBackoff, "err", err)
		select {
		case <-time.After(moduleBackoff):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("service-start: netfilter module не появился за %s: %w", moduleTimeout, lastErr)
}

// isMissingNetfilterModuleErr различает случай не загруженного
// netfilter-модуля (xt_NFQUEUE на cold boot Keenetic) от других ошибок
// firewall apply. iptables печатает в stderr эту строку для отсутствующих
// match/target — applier заворачивает stderr в `%w` через fmt.Errorf,
// поэтому strings.Contains по полному сообщению ошибки надёжен.
func isMissingNetfilterModuleErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "No chain/target/match by that name")
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

// needsTUN сообщает, требуется ли TUN-интерфейс для активного ядра.
//
// Sing-box создаёт signbox-tun только когда state.Inbound!="tproxy" (если
// явно tproxy — sing-box использует TPROXY inbound, TUN не нужен).
// Xray и mihomo всегда работают через TProxy + fwmark, TUN не создают.
//
// Используется в doStart/doStop для условного TUN cleanup и AttachTUN.
// Альтернатива (Core.Inbound() string в интерфейсе) хуже, потому что
// решение зависит от state.Inbound, а интерфейс stateless.
func needsTUN(c core.Core, st *state.State) bool {
	if c == nil || c.Name() != "sing-box" {
		return false
	}
	return st == nil || st.Inbound != "tproxy"
}

func waitDefaultRoute(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		res, err := exectx.OS.Run(ctx, "ip", "route", "show", "default")
		if err == nil && strings.Contains(string(res.Stdout), "default ") {
			return nil
		}
		time.Sleep(time.Second)
	}
	return errors.New("таймаут ожидания маршрута по умолчанию")
}

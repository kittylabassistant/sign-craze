package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/core"
	"github.com/kittylabassistant/sign-craze/internal/dpi"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/naiveproxy"
	"github.com/kittylabassistant/sign-craze/internal/proxyparse"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

const minFreeBytes = 30 * 1024 * 1024

func init() {
	Register(Cmd{Short: "-i", Long: "--install", Help: "установка с интерактивной настройкой outbound [--core <ядро>] [--proxy <URL>] [--preset <name>]", Handler: handleInstall})
	Register(Cmd{Long: "--install-auto", Help: "установка без интерактива (stub direct outbound) [--core <ядро>] [--proxy <URL>] [--preset <name>]", Handler: handleInstallAuto})
	Register(Cmd{Long: "--install-offline", Help: "установка из локального tarball <путь> [--core <ядро>] [--proxy <URL>] [--preset <name>]", Handler: handleInstallOffline})
	Register(Cmd{Long: "--reinstall", Help: "переустановка поверх существующей [--proxy <URL>] [--preset <name>]", Handler: handleReinstall})
}

type installMode int

const (
	installInteractive installMode = iota
	installAuto
	installOffline
)

// parseStringFlag вытягивает флаг --name <value> / --name=<value> из args.
// Возвращает значение (или пустую строку) и остаток args.
func parseStringFlag(args []string, name string) (value string, rest []string) {
	rest = make([]string, 0, len(args))
	flag := "--" + name
	for i := 0; i < len(args); i++ {
		if args[i] == flag && i+1 < len(args) {
			value = args[i+1]
			i++
			continue
		}
		if after, ok := strings.CutPrefix(args[i], flag+"="); ok {
			value = after
			continue
		}
		rest = append(rest, args[i])
	}
	return
}

// parseBoolFlag вытягивает булев флаг --name из args.
// Возвращает true если флаг присутствует и остаток args.
func parseBoolFlag(args []string, name string) (found bool, rest []string) {
	rest = make([]string, 0, len(args))
	flag := "--" + name
	for _, a := range args {
		if a == flag {
			found = true
			continue
		}
		rest = append(rest, a)
	}
	return
}

func handleInstall(ctx context.Context, args []string) error {
	coreName, rest := parseStringFlag(args, "core")
	proxyURL, rest := parseStringFlag(rest, "proxy")
	withDPI, rest := parseBoolFlag(rest, "with-dpi")
	withNaive, rest := parseBoolFlag(rest, "with-naive")
	presetName, rest := parseStringFlag(rest, "preset")
	inbound, _ := parseStringFlag(rest, "inbound")
	if inbound == "" {
		inbound = state.DefaultInbound
	}
	return withLock(ctx, func() error {
		return doInstall(ctx, installInteractive, "", false, coreName, proxyURL, withDPI, withNaive, inbound, presetName)
	})
}

func handleInstallAuto(ctx context.Context, args []string) error {
	coreName, rest := parseStringFlag(args, "core")
	proxyURL, rest := parseStringFlag(rest, "proxy")
	withDPI, rest := parseBoolFlag(rest, "with-dpi")
	withNaive, rest := parseBoolFlag(rest, "with-naive")
	presetName, rest := parseStringFlag(rest, "preset")
	inbound, _ := parseStringFlag(rest, "inbound")
	if inbound == "" {
		inbound = state.DefaultInbound
	}
	return withLock(ctx, func() error {
		return doInstall(ctx, installAuto, "", false, coreName, proxyURL, withDPI, withNaive, inbound, presetName)
	})
}

func handleInstallOffline(ctx context.Context, args []string) error {
	coreName, rest := parseStringFlag(args, "core")
	proxyURL, rest := parseStringFlag(rest, "proxy")
	withDPI, rest := parseBoolFlag(rest, "with-dpi")
	withNaive, rest := parseBoolFlag(rest, "with-naive")
	presetName, rest := parseStringFlag(rest, "preset")
	inbound, rest := parseStringFlag(rest, "inbound")
	if inbound == "" {
		inbound = state.DefaultInbound
	}
	if len(rest) == 0 {
		return fmt.Errorf("--install-offline: требуется путь к tarball")
	}
	// install-offline всегда force=true: если state.json остался от прерванной
	// установки (например, валидация конфига упала), пользователь явно
	// указывает локальный tarball — переустановка поверх ожидаемое поведение.
	return withLock(ctx, func() error {
		return doInstall(ctx, installOffline, rest[0], true, coreName, proxyURL, withDPI, withNaive, inbound, presetName)
	})
}

func handleReinstall(ctx context.Context, args []string) error {
	coreName, rest := parseStringFlag(args, "core")
	proxyURL, rest := parseStringFlag(rest, "proxy")
	withDPI, rest := parseBoolFlag(rest, "with-dpi")
	withNaive, rest := parseBoolFlag(rest, "with-naive")
	presetName, rest := parseStringFlag(rest, "preset")
	inbound, _ := parseStringFlag(rest, "inbound")
	if inbound == "" {
		inbound = state.DefaultInbound
	}
	if coreName == "" {
		if prevSt, loadErr := loadState(); loadErr == nil && prevSt.Core != "" {
			coreName = prevSt.Core
		}
	}
	mode := installAuto
	if proxyURL != "" {
		mode = installInteractive
	}
	return withLock(ctx, func() error {
		return doInstall(ctx, mode, "", true, coreName, proxyURL, withDPI, withNaive, inbound, presetName)
	})
}

func doInstall(ctx context.Context, mode installMode, offlineTar string, force bool, coreName string, proxyURL string, withDPI bool, withNaive bool, inboundMode string, presetName string) error {
	// Auto-detect ядра по proxy URL если --core явно не указан.
	// Multi-match → sing-box default + info-print. Конфликт явного --core с
	// несовместимым URL обрабатывается ниже (после core.Get).
	coreExplicit := coreName != ""
	if !coreExplicit && proxyURL != "" {
		if recommended, allCompat, ok := detectCoreFromProxyURL(proxyURL); ok {
			if len(allCompat) > 1 {
				fmt.Printf(
					"%s %s. %s %s %s\n",
					Info("URL совместим с:"), strings.Join(allCompat, ", "),
					Hint("Выбран"), Bold(recommended), Hint("(default). Для другого ядра: --core <name>"),
				)
			} else {
				fmt.Printf("%s %s — будет установлено.\n", Info("URL требует ядро"), Bold(recommended))
			}
			coreName = recommended
		}
	}
	if coreName == "" {
		coreName = state.DefaultCore
	}
	activeC, err := core.Get(coreName)
	if err != nil {
		return fmt.Errorf("--install: %w", err)
	}

	// Conflict check: явный --core <X> + URL несовместим с X → ранний error.
	// Без этого проблема всплывёт лишь на CheckConfig после скачивания тарбола.
	if coreExplicit && proxyURL != "" {
		if tempO, _, parseErr := parseProxyURLToOutbound(proxyURL); parseErr == nil {
			if vErr := activeC.ValidateOutbound(tempO); vErr != nil {
				_, allCompat := core.RecommendCore(tempO)
				hint := "уберите --core (auto-detect подберёт ядро)"
				if len(allCompat) > 0 {
					hint = fmt.Sprintf("совместимые ядра: %s. Уберите --core или укажите одно из них", strings.Join(allCompat, ", "))
				}
				return fmt.Errorf("--install: URL несовместим с ядром %q: %w\nПодсказка: %s", coreName, vErr, hint)
			}
		}
	}

	// 0. Idempotency check: если уже установлено и не force — отказ
	// (safety-fixes #16). До этого --install молча перезаписывал всё.
	//
	// Различаем два случая:
	//   а) full install: state.json + core binary оба существуют → требуем --reinstall;
	//   б) degraded: state.json есть, core binary нет (типичный артефакт частичного
	//      uninstall — например, watchdog daemon воскресил state.json через
	//      reconcile/saveState после удаления). В этом случае подсказываем
	//      --uninstall ещё раз: уже исправленный --uninstall корректно убивает
	//      watchdog и удаляет state.json.
	if !force {
		_, stateErr := os.Stat(state.DefaultPath)
		_, binErr := os.Stat(activeC.BinaryPath())
		stateExists := stateErr == nil
		binExists := binErr == nil
		switch {
		case stateExists && binExists:
			return fmt.Errorf("--install: sign-craze уже установлен. Используйте --reinstall для переустановки или --uninstall чтобы сначала очистить")
		case stateExists && !binExists:
			return fmt.Errorf("--install: обнаружено degraded состояние (state.json есть, бинарь %s отсутствует). Запустите --uninstall для очистки остатков, затем повторите --install", activeC.BinaryPath())
		case !stateExists && binExists:
			return fmt.Errorf("--install: бинарь %s уже установлен (%s) без state.json. Запустите --uninstall, затем --install", coreName, activeC.BinaryPath())
		}
	}

	// 1. Pre-check: /opt существует и есть место.
	if err := checkOptMounted(); err != nil {
		return fmt.Errorf("--install: %w", err)
	}

	// Pre-check (warn-only): iptables/iptables-restore/ipset нужны для --start,
	// но не для самой установки. При `curl|sh` (минуя opkg-зависимости) их легко
	// забыть — предупреждаем заранее, чтобы пользователь не словил firewall-ошибку
	// на --start (issue #3). Установку не блокируем.
	if err := firewall.CheckRequiredBinaries(ctx, exectx.OS); err != nil {
		fmt.Printf("%s %v\n", Warn("Внимание"), err)
	}

	// 2. Создать директории.
	for _, d := range []string{
		"/opt/sbin",
		singbox.DefaultConfigDir,
		"/opt/var/lib/sign-craze",
		"/opt/var/lib/sign-craze/geo",
		"/opt/var/lib/sign-craze/backups",
		singbox.DefaultCacheDir,
		"/opt/var/log/sign-craze",
		"/opt/var/run",
		"/opt/var/lock",
		"/opt/etc/init.d",
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("--install: mkdir %s: %w", d, err)
		}
	}

	// 3. Получить tarball целевого ядра.
	var tarPath string
	switch mode {
	case installOffline:
		if _, err := os.Stat(offlineTar); err != nil {
			return fmt.Errorf("--install-offline: tarball %s: %w", offlineTar, err)
		}
		tarPath = offlineTar
	default:
		arch, err := types.DetectHostArch()
		if err != nil {
			return fmt.Errorf("--install: %w", err)
		}
		fmt.Printf("%s %s (arch=%s)...\n", Info("Загрузка"), coreName, arch)
		res, err := activeC.Download(ctx, arch, activeC.CacheDir())
		if err != nil {
			return fmt.Errorf("--install: %w", err)
		}
		fmt.Printf("%s %s (%s)\n", OK("Скачан"), Hint(res.Path), res.Version)
		tarPath = res.Path
	}

	// 4. Собрать outbounds.
	// Приоритет: --proxy <URL> (передан в args) > интерактивный wizard > stub direct.
	var outbounds []types.Outbound
	switch {
	case proxyURL != "":
		o, _, err := parseProxyURLToOutbound(proxyURL)
		if err != nil {
			return fmt.Errorf("--install --proxy: %w", err)
		}
		outbounds = []types.Outbound{o}
	case mode == installInteractive:
		ob, err := runProxyWizard(os.Stdin, os.Stdout)
		if err != nil {
			return fmt.Errorf("--install: wizard: %w", err)
		}
		outbounds = ob
	}
	if len(outbounds) == 0 {
		// Стаб direct, чтобы sing-box check прошёл.
		outbounds = []types.Outbound{{Tag: "direct", Type: "direct"}}
	}

	// 5. Сохранить state перед установкой бинаря. Default() содержит безопасные
	// CIDR-исключения (RFC1918 + loopback + multicast) — защита от SSH-lockout,
	// AdminPort=22 — защита SSH-сессии. Для кастомизации править state.json.
	st := state.Default()
	st.Outbounds = outbounds
	st.Core = coreName
	if inboundMode != "" {
		st.Inbound = inboundMode
	}
	if err := state.Save(state.DefaultPath, st); err != nil {
		return fmt.Errorf("--install: state: %w", err)
	}

	// 6–7. Подготовить бинарь и валидировать конфиг ДО установки в финальный
	// путь — защита от состояния "бинарь установлен, конфиг битый" (safety-fixes #10).
	//
	// Для sing-box используется специализированный PrepareAndValidate (извлекает
	// бинарь во временный путь, рендерит и проверяет конфиг через sing-box check -c,
	// только потом атомарно переносит). Для остальных ядер используем общий путь:
	// Install → RenderConfig → WriteFileAtomic → CheckConfig.
	if coreName == "sing-box" {
		params := singboxParamsForInstall(st)

		tempBin, err := singbox.PrepareAndValidate(ctx, exectx.OS, singbox.DefaultCacheDir, tarPath, configPath(), params)
		if err != nil {
			return fmt.Errorf("--install: %w", err)
		}
		defer os.RemoveAll(filepath.Dir(tempBin))

		// Атомарно перенести валидированный бинарь в финальный путь.
		// Стримим через io.Reader: на 128MB-роутерах os.ReadFile(~12MB) +
		// BackupAndReplace([]byte) даёт OOM-Kill (binary в RAM × 2 — copy
		// в map[]byte). BackupAndReplaceFromReader держит только малый буфер.
		binFile, err := os.Open(tempBin)
		if err != nil {
			return fmt.Errorf("--install: открытие валидированного бинаря: %w", err)
		}
		_, err = atomicfs.BackupAndReplaceFromReader(singbox.DefaultBinPath, binFile, 0o755)
		_ = binFile.Close()
		if err != nil {
			return fmt.Errorf("--install: установка бинаря: %w", err)
		}
	} else {
		// Общий путь: Install → RenderConfig → WriteFileAtomic → CheckConfig.
		if err := os.MkdirAll(filepath.Dir(activeC.BinaryPath()), 0o755); err != nil {
			return fmt.Errorf("--install: mkdir bin dir: %w", err)
		}
		if err := activeC.Install(ctx, exectx.OS, tarPath); err != nil {
			return fmt.Errorf("--install: установка %s: %w", coreName, err)
		}

		cfgBytes, err := activeC.RenderConfig(types.CoreRenderParams{
			Mode:      st.Mode,
			Outbounds: st.Outbounds,
		})
		if err != nil {
			return fmt.Errorf("--install: рендер конфига %s: %w", coreName, err)
		}

		if err := os.MkdirAll(activeC.ConfigDir(), 0o755); err != nil {
			return fmt.Errorf("--install: mkdir config dir: %w", err)
		}
		if err := atomicfs.WriteFileAtomic(activeC.ConfigPath(), cfgBytes, 0o640); err != nil {
			return fmt.Errorf("--install: запись конфига %s: %w", coreName, err)
		}

		if err := activeC.CheckConfig(ctx, exectx.OS, activeC.ConfigPath()); err != nil {
			return fmt.Errorf("--install: проверка конфига %s: %w", coreName, err)
		}
	}

	// 7.5 Опционально: установить nfqws2 + blobs + включить DPI с preset
	// discord-youtube. По завершении --start уже стартует nfqws2 с
	// рабочей стратегией для YouTube + Discord без ручных шагов.
	if withDPI {
		if err := setupDPIDefault(ctx, st); err != nil {
			return fmt.Errorf("--install --with-dpi: %w", err)
		}
	}

	// 7.6 Опционально: скачать и установить naive бинарь + выставить NaiveEnabled.
	// Срабатывает при --with-naive ИЛИ если в outbounds уже есть naive outbound
	// (добавлен через --proxy naive+https://...).
	naiveInOutbounds := false
	for _, ob := range st.Outbounds {
		if ob.Protocol == types.ProtocolNaive {
			naiveInOutbounds = true
			break
		}
	}
	if withNaive || naiveInOutbounds {
		if err := setupNaiveDefault(ctx, st); err != nil {
			return fmt.Errorf("--install --with-naive: %w", err)
		}
	}

	// 7.7 Опционально: применить routing-preset поверх routing.json.
	if presetName != "" {
		if err := applyInstallPreset(ctx, presetName, activeC, st); err != nil {
			return fmt.Errorf("--install --preset: %w", err)
		}
	}

	// 8. Создать init.d shim.
	if err := service.WriteShim(service.DefaultShimPath, service.ShimParams{BinPath: service.DefaultSignCrazeBin}); err != nil {
		return fmt.Errorf("--install: init.d shim: %w", err)
	}

	// 9. Создать NDM netfilter.d hook. Без него правила mangle теряются при
	// первом же rebuild iptables со стороны Keenetic (привязка устройства к
	// policy через UI, save startup-config, reconnect WAN). Hook содержит
	// путь к PID-файлу активного ядра — при смене ядра должен быть
	// перегенерирован.
	if err := service.WriteHook(service.DefaultNetfilterHookPath, service.HookParams{
		BinPath: service.DefaultSignCrazeBin,
		PIDPath: activeC.PIDPath(),
	}); err != nil {
		return fmt.Errorf("--install: netfilter.d hook: %w", err)
	}

	fmt.Println()
	fmt.Println(OK("Установка завершена."))
	if outbounds[0].Type == "direct" {
		fmt.Println(Warn("ВНИМАНИЕ:") + " outbound настроен как 'direct' — проксирования нет.")
		fmt.Println(Hint("Передайте URL через --proxy <URL> или зайдите в admin UI на :9091."))
	}
	if withDPI {
		fmt.Println(Info("DPI включён:") + " nfqws2 + preset discord-youtube. После --start заработает обход YT/Discord.")
	}
	if withNaive || naiveInOutbounds {
		fmt.Println(Info("naive установлен:") + " бинарь naive готов. Добавьте outbound через --proxy naive+https://... или web UI.")
	}
	if force {
		fmt.Println(Hint("Перезапуск сервиса: sign-craze --restart"))
	} else {
		fmt.Println(Hint("Запуск сервиса: sign-craze --start"))
	}
	return nil
}

// setupDPIDefault выполняет шаги --with-dpi: загрузка nfqws2 + blobs из
// upstream nfqws2-keenetic, заполнение DPITargets из preset "discord-youtube",
// генерация nfqws2.conf + hostlist, сохранение state с DPIEnabled=true.
//
// На момент вызова state уже сохранён (шаг 5 doInstall) — здесь мы только
// читаем его обратно, дополняем DPI-полями и пересохраняем.
func setupDPIDefault(ctx context.Context, st *state.State) error {
	if err := installNfqws2WithBlobs(ctx); err != nil {
		return err
	}

	preset := dpi.FindPreset("discord-youtube")
	if preset == nil {
		return fmt.Errorf("preset discord-youtube не найден")
	}
	st.DPIEnabled = true
	st.DPITargets = append([]string(nil), preset.Targets...)

	iface, err := detectISPInterface(ctx)
	if err != nil {
		// Не фатально: на момент install ISP-маршрут может ещё не быть установлен
		// (например, до подключения WAN). Конфиг будет сгенерирован при --start.
		log.L().Warn("--with-dpi: ISP-интерфейс не определён, конфиг nfqws2 будет создан при --start", "err", err)
	} else {
		if err := writeDPIConfig(iface, st); err != nil {
			return fmt.Errorf("nfqws2.conf: %w", err)
		}
	}
	if err := state.Save(state.DefaultPath, st); err != nil {
		return fmt.Errorf("state: %w", err)
	}
	return nil
}

// setupNaiveDefault скачивает и устанавливает бинарь naive, выставляет
// State.NaiveEnabled=true. Вызывается при --with-naive или автоматически
// когда в state добавлен naive outbound (Protocol=ProtocolNaive).
//
// На момент вызова state уже сохранён (шаг 5 doInstall) — здесь мы только
// дополняем NaiveEnabled и пересохраняем.
func setupNaiveDefault(ctx context.Context, st *state.State) error {
	arch, err := types.DetectHostArch()
	if err != nil {
		return fmt.Errorf("naive setup: detect arch: %w", err)
	}
	if mkErr := os.MkdirAll(naiveproxy.DefaultCacheDir, 0o755); mkErr != nil {
		return fmt.Errorf("naive setup: mkdir cache: %w", mkErr)
	}
	res, err := naiveproxy.Download(ctx, arch, naiveproxy.DefaultCacheDir)
	if err != nil {
		return fmt.Errorf("naive setup: download: %w", err)
	}
	if instErr := naiveproxy.Install(res.Path, naiveproxy.DefaultBinPath); instErr != nil {
		return fmt.Errorf("naive setup: install: %w", instErr)
	}
	st.NaiveEnabled = true
	if saveErr := state.Save(state.DefaultPath, st); saveErr != nil {
		return fmt.Errorf("naive setup: state: %w", saveErr)
	}
	log.L().Info("naive установлен", "version", res.Version, "bin", naiveproxy.DefaultBinPath)
	return nil
}

// detectCoreFromProxyURL парсит URL и возвращает рекомендованное ядро +
// список всех совместимых ядер. ok=false если URL невалиден или
// canonical-парсинг не дал совместимых ядер — в этом случае caller
// fallback'ит на state.DefaultCore.
func detectCoreFromProxyURL(url string) (recommended string, allCompatible []string, ok bool) {
	o, rec, err := parseProxyURLToOutbound(url)
	if err != nil || rec == "" {
		return "", nil, false
	}
	_, all := core.RecommendCore(o)
	return rec, all, true
}

// parseURLToOutbound — единая точка разбора proxy-URL, общая для CLI-флага
// --proxy (parseProxyURLToOutbound) и интерактивного wizard (wizardURL).
// Раньше эти два call-site дублировали парсинг независимо, и только один из
// них (--proxy) проставлял naive-порт-дефолт — баг B3: naive+https://... URL,
// введённый через wizard, давал Outbound с NaiveListenPort=0, и
// singbox/render.go (renderCanonical, case ProtocolNaive) падал с "NaiveListenPort
// не выделен" уже на PrepareAndValidate при --install.
//
// Порядок шагов: ParseCanonical (единственный парсер — legacy proxyparse.Parse
// удалён) → перенос canonical-полей в Outbound → naive-порт-дефолт →
// Validate → RecommendCore.
//
// ParseCanonical строже legacy-парсера: например, неизвестный security=
// у VLESS/... или pbk/sid без security=reality теперь возвращают ошибку
// вместо тихого игнорирования — осознанное ужесточение валидации.
//
// Naive-порт-дефолт обязан жить именно здесь (на этапе парсинга URL, до
// Validate() и записи в state.json), а не в рендере ядра:
// internal/naiveproxy/config.go независимо читает то же самое поле
// Proto.NaiveListenPort для аргументов запуска naive-демона — если дефолтить
// в render, а не тут, порты sing-box outbound'а и naive-демона могут разойтись.
//
// Возвращает Outbound с встроенным canonical и имя рекомендованного ядра
// для auto-detect. recommendedCore="" при невалидном/несовместимом URL.
func parseURLToOutbound(url string) (types.Outbound, string, error) {
	o, canon, err := proxyparse.ParseCanonical(url)
	if err != nil {
		return types.Outbound{}, "", fmt.Errorf("парсинг URL: %w", err)
	}
	o.Protocol = canon.Protocol
	o.Transport = canon.Transport
	o.TLS = canon.TLS
	o.Proto = canon.Proto
	// Для naive outbound: выставить NaiveListenPort если не задан.
	if o.Protocol == types.ProtocolNaive {
		if o.Proto == nil {
			o.Proto = &types.ProtoOpts{}
		}
		if o.Proto.NaiveListenPort == 0 {
			o.Proto.NaiveListenPort = 18443
		}
	}
	if err := o.Validate(); err != nil {
		return types.Outbound{}, "", fmt.Errorf("валидация: %w", err)
	}
	recommended, _ := core.RecommendCore(o)
	return o, recommended, nil
}

// parseProxyURLToOutbound — алиас parseURLToOutbound для call-site'ов
// CLI-флага --proxy (doInstall, detectCoreFromProxyURL). Имя сохранено
// отдельно ради читаемости вызовов и обратной совместимости существующих
// тестов.
func parseProxyURLToOutbound(url string) (types.Outbound, string, error) {
	return parseURLToOutbound(url)
}

func checkOptMounted() error {
	info, err := os.Stat("/opt")
	if err != nil {
		return fmt.Errorf("/opt не доступен: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("/opt не директория")
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/opt", &stat); err != nil {
		return fmt.Errorf("/opt statfs: %w", err)
	}
	free := uint64(stat.Bavail) * uint64(stat.Bsize)
	if free < minFreeBytes {
		return fmt.Errorf("недостаточно места в /opt: %d МБ < 30 МБ", free/1024/1024)
	}
	log.L().Debug("/opt: свободно", "bytes", free)
	return nil
}

// runProxyWizard опрашивает пользователя и возвращает список outbound'ов.
// Поддерживается только URL-режим (socks5/http/vless/vmess/ss/trojan через
// proxyparse) — ручной ввод убран, т.к. URL покрывает все live-кейсы и
// меньше шансов ошибиться с типом/портом/credentials.
func runProxyWizard(in io.Reader, out io.Writer) ([]types.Outbound, error) {
	r := bufio.NewReader(in)

	fmt.Fprintln(out)
	fmt.Fprintln(out, Header("Настройка outbound прокси:"))
	fmt.Fprintln(out, "  "+Cyan("1)")+" Полный URL (socks5://, http://, vless://, vmess://, ss://, trojan://, naive+https://)")
	fmt.Fprintln(out, "  "+Cyan("2)")+" Пропустить (создаст stub direct — sing-box без проксирования)")
	fmt.Fprint(out, Key("Выбор [1/2]: "))
	choice, err := readLineE(r)
	if err != nil {
		return nil, fmt.Errorf("чтение выбора: %w", err)
	}

	switch choice {
	case "1":
		return wizardURL(r, out)
	case "2", "":
		return nil, nil
	default:
		fmt.Fprintln(out, Warn("Неизвестный выбор, пропускаем."))
		return nil, nil
	}
}

func wizardURL(r *bufio.Reader, out io.Writer) ([]types.Outbound, error) {
	fmt.Fprint(out, Key("URL: "))
	url, err := readLineE(r)
	if err != nil {
		return nil, fmt.Errorf("чтение URL: %w", err)
	}
	if url == "" {
		return nil, nil
	}

	// Общий парсер: ParseCanonical → naive-порт-дефолт → Validate → RecommendCore.
	// Тексты ошибок ("парсинг URL: %w", "валидация: %w") формирует
	// parseURLToOutbound — здесь просто пробрасываем их дальше.
	o, _, err := parseURLToOutbound(url)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "%s type=%s server=%s port=%d\n", OK("Outbound:"), o.Type, o.Server, o.Port)
	return []types.Outbound{o}, nil
}

// readLineE — версия readLine с распространением ошибок I/O.
// Возвращает trimmed-строку или ошибку, если stdin не удалось прочитать
// (EOF без newline трактуется как «пользователь оборвал» — error).
// Используется в точках, где надо отличить «пустая строка = пропустить»
// от «не удалось прочитать ввод» (safety-fixes D.5).
func readLineE(r *bufio.Reader) (string, error) {
	s, err := r.ReadString('\n')
	if err != nil && s == "" {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

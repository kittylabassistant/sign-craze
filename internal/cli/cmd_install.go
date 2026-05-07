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
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/proxyparse"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

const minFreeBytes = 30 * 1024 * 1024

func init() {
	Register(Cmd{Short: "-i", Long: "--install", Help: "установка с интерактивной настройкой outbound [--core <ядро>] [--proxy <URL>]", Handler: handleInstall})
	Register(Cmd{Long: "--install-auto", Help: "установка без интерактива (stub direct outbound) [--core <ядро>] [--proxy <URL>]", Handler: handleInstallAuto})
	Register(Cmd{Long: "--install-offline", Help: "установка из локального tarball <путь> [--core <ядро>] [--proxy <URL>]", Handler: handleInstallOffline})
	Register(Cmd{Long: "--reinstall", Help: "переустановка поверх существующей [--proxy <URL>]", Handler: handleReinstall})
}

type installMode int

const (
	installInteractive installMode = iota
	installAuto
	installOffline
)

// parseInstallCoreFlag ищет --core <name> или --core=<name> в аргументах.
// Возвращает имя ядра (или пустую строку если флаг не задан) и остаток args.
func parseInstallCoreFlag(args []string) (coreName string, rest []string) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--core" && i+1 < len(args) {
			coreName = args[i+1]
			i++
			continue
		}
		if after, ok := strings.CutPrefix(args[i], "--core="); ok {
			coreName = after
			continue
		}
		rest = append(rest, args[i])
	}
	return
}

// parseWithDPIFlag вытягивает булев флаг --with-dpi из args.
// При наличии: устанавливаем nfqws2 + blobs, включаем DPI с preset
// "discord-youtube" (out-of-box работа YouTube + Discord без ручных шагов).
func parseWithDPIFlag(args []string) (withDPI bool, rest []string) {
	rest = make([]string, 0, len(args))
	for _, a := range args {
		if a == "--with-dpi" {
			withDPI = true
			continue
		}
		rest = append(rest, a)
	}
	return
}

// parseProxyFlag вытягивает --proxy <URL> / --proxy=<URL> из args.
// URL без --proxy остаётся в rest (для install-offline он трактуется как путь к tarball).
func parseProxyFlag(args []string) (proxyURL string, rest []string) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--proxy" && i+1 < len(args) {
			proxyURL = args[i+1]
			i++
			continue
		}
		if after, ok := strings.CutPrefix(args[i], "--proxy="); ok {
			proxyURL = after
			continue
		}
		rest = append(rest, args[i])
	}
	return
}

func handleInstall(ctx context.Context, args []string) error {
	coreName, rest := parseInstallCoreFlag(args)
	proxyURL, rest := parseProxyFlag(rest)
	withDPI, _ := parseWithDPIFlag(rest)
	return withLock(ctx, func() error { return doInstall(ctx, installInteractive, "", false, coreName, proxyURL, withDPI) })
}

func handleInstallAuto(ctx context.Context, args []string) error {
	coreName, rest := parseInstallCoreFlag(args)
	proxyURL, rest := parseProxyFlag(rest)
	withDPI, _ := parseWithDPIFlag(rest)
	return withLock(ctx, func() error { return doInstall(ctx, installAuto, "", false, coreName, proxyURL, withDPI) })
}

func handleInstallOffline(ctx context.Context, args []string) error {
	coreName, rest := parseInstallCoreFlag(args)
	proxyURL, rest := parseProxyFlag(rest)
	withDPI, rest := parseWithDPIFlag(rest)
	if len(rest) == 0 {
		return fmt.Errorf("--install-offline: требуется путь к tarball")
	}
	return withLock(ctx, func() error { return doInstall(ctx, installOffline, rest[0], false, coreName, proxyURL, withDPI) })
}

func handleReinstall(ctx context.Context, args []string) error {
	_, rest := parseInstallCoreFlag(args)
	proxyURL, rest := parseProxyFlag(rest)
	withDPI, _ := parseWithDPIFlag(rest)
	mode := installAuto
	if proxyURL != "" {
		mode = installInteractive
	}
	return withLock(ctx, func() error { return doInstall(ctx, mode, "", true, "", proxyURL, withDPI) })
}

func doInstall(ctx context.Context, mode installMode, offlineTar string, force bool, coreName string, proxyURL string, withDPI bool) error {
	// Определяем целевое ядро. Пустое имя → sing-box (default).
	if coreName == "" {
		coreName = state.DefaultCore
	}
	activeC, err := core.Get(coreName)
	if err != nil {
		return fmt.Errorf("--install: %w", err)
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
		fmt.Printf("Загрузка %s (arch=%s)...\n", coreName, arch)
		res, err := activeC.Download(ctx, arch, activeC.CacheDir())
		if err != nil {
			return fmt.Errorf("--install: %w", err)
		}
		fmt.Printf("Скачан %s (%s)\n", res.Path, res.Version)
		tarPath = res.Path
	}

	// 4. Собрать outbounds.
	// Приоритет: --proxy <URL> (передан в args) > интерактивный wizard > stub direct.
	var outbounds []types.Outbound
	switch {
	case proxyURL != "":
		o, err := parseProxyURLToOutbound(proxyURL)
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
		params := singbox.DefaultConfigParams()
		params.Mode = st.Mode
		params.Outbounds = st.Outbounds
		if len(st.Outbounds) > 0 {
			params.DefaultOutboundTag = st.Outbounds[0].Tag
		}

		tempBin, err := singbox.PrepareAndValidate(ctx, newRunner(), singbox.DefaultCacheDir, tarPath, configPath(), params)
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
		if err := activeC.Install(ctx, newRunner(), tarPath); err != nil {
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

		if err := activeC.CheckConfig(ctx, newRunner(), activeC.ConfigPath()); err != nil {
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
	fmt.Println("Установка завершена.")
	if outbounds[0].Type == "direct" {
		fmt.Println("ВНИМАНИЕ: outbound настроен как 'direct' — проксирования нет.")
		fmt.Println("Передайте URL через --proxy <URL> или зайдите в admin UI на :9091.")
	}
	if withDPI {
		fmt.Println("DPI включён: nfqws2 + preset discord-youtube. После --start заработает обход YT/Discord.")
	}
	if force {
		fmt.Println("Перезапуск сервиса: sign-craze --restart")
	} else {
		fmt.Println("Запуск сервиса: sign-craze --start")
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

// parseProxyURLToOutbound — общий парсер URL для CLI-флага --proxy.
// Тот же путь, что и в wizardURL: ParseCanonical с fallback на legacy Parse.
func parseProxyURLToOutbound(url string) (types.Outbound, error) {
	o, canon, err := proxyparse.ParseCanonical(url)
	if err != nil {
		var legacyErr error
		o, legacyErr = proxyparse.Parse(url)
		if legacyErr != nil {
			return types.Outbound{}, fmt.Errorf("парсинг URL: %w", err)
		}
	} else {
		o.Protocol = canon.Protocol
		o.Transport = canon.Transport
		o.TLS = canon.TLS
		o.Proto = canon.Proto
	}
	if err := o.Validate(); err != nil {
		return types.Outbound{}, fmt.Errorf("валидация: %w", err)
	}
	return o, nil
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
	fmt.Fprintln(out, "Настройка outbound прокси:")
	fmt.Fprintln(out, "  1) Полный URL (socks5://, http://, vless://, vmess://, ss://, trojan://)")
	fmt.Fprintln(out, "  2) Пропустить (создаст stub direct — sing-box без проксирования)")
	fmt.Fprint(out, "Выбор [1/2]: ")
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
		fmt.Fprintln(out, "Неизвестный выбор, пропускаем.")
		return nil, nil
	}
}

func wizardURL(r *bufio.Reader, out io.Writer) ([]types.Outbound, error) {
	fmt.Fprint(out, "URL: ")
	url := readLine(r)
	if url == "" {
		return nil, nil
	}

	// Пробуем ParseCanonical — возвращает Outbound с базовыми полями
	// и отдельный Canonical aggregator с Protocol/Transport/TLS/Proto.
	o, canon, err := proxyparse.ParseCanonical(url)
	if err != nil {
		// Fallback на legacy Parse для схем, которые ParseCanonical не охватывает.
		var legacyErr error
		o, legacyErr = proxyparse.Parse(url)
		if legacyErr != nil {
			// Возвращаем оригинальную ошибку ParseCanonical — она точнее.
			return nil, fmt.Errorf("парсинг URL: %w", err)
		}
	} else {
		// Переносим canonical-поля в Outbound.
		o.Protocol = canon.Protocol
		o.Transport = canon.Transport
		o.TLS = canon.TLS
		o.Proto = canon.Proto
	}

	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("валидация: %w", err)
	}
	fmt.Fprintf(out, "Outbound: type=%s server=%s port=%d\n", o.Type, o.Server, o.Port)
	return []types.Outbound{o}, nil
}

func readLine(r *bufio.Reader) string {
	s, err := r.ReadString('\n')
	if err != nil && s == "" {
		return ""
	}
	return strings.TrimSpace(s)
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

// configFilePath возвращает /opt/etc/sign-craze/config.json.
// Дублирует cli.configPath() для совместимости с тестами; будет удалён, когда
// тесты переедут на пакетный configPath.
func configFilePath() string {
	return filepath.Join(singbox.DefaultConfigDir, "config.json")
}

var _ = configFilePath // silence unused if no test references

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
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/proxyparse"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

const minFreeBytes = 30 * 1024 * 1024

func init() {
	Register(Cmd{Short: "-i", Long: "--install", Help: "установка с интерактивной настройкой outbound", Handler: handleInstall})
	Register(Cmd{Long: "--install-auto", Help: "установка без интерактива (stub direct outbound)", Handler: handleInstallAuto})
	Register(Cmd{Long: "--install-offline", Help: "установка из локального tarball <путь>", Handler: handleInstallOffline})
	Register(Cmd{Long: "--reinstall", Help: "переустановка поверх существующей (использует --install-auto)", Handler: handleReinstall})
}

type installMode int

const (
	installInteractive installMode = iota
	installAuto
	installOffline
)

func handleInstall(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error { return doInstall(ctx, installInteractive, "", false) })
}

func handleInstallAuto(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error { return doInstall(ctx, installAuto, "", false) })
}

func handleInstallOffline(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--install-offline: требуется путь к tarball")
	}
	return withLock(ctx, func() error { return doInstall(ctx, installOffline, args[0], false) })
}

func handleReinstall(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error { return doInstall(ctx, installAuto, "", true) })
}

func doInstall(ctx context.Context, mode installMode, offlineTar string, force bool) error {
	// 0. Idempotency check: если уже установлено и не force — отказ
	// (safety-fixes #16). До этого --install молча перезаписывал всё.
	//
	// Различаем три состояния:
	//   а) full install: state.json + sing-box бинарь оба существуют → требуем --reinstall;
	//   б) degraded: state.json есть, бинарь нет (типичный артефакт частичного
	//      uninstall — например, watchdog daemon воскресил state.json через
	//      reconcile/saveState после удаления). Подсказываем --uninstall ещё раз;
	//   в) orphan-bin: бинарь есть, state.json нет — необычная ручная подкладка.
	if !force {
		_, stateErr := os.Stat(state.DefaultPath)
		_, binErr := os.Stat(singbox.DefaultBinPath)
		stateExists := stateErr == nil
		binExists := binErr == nil
		switch {
		case stateExists && binExists:
			return fmt.Errorf("--install: sign-craze уже установлен. Используйте --reinstall для переустановки или --uninstall чтобы сначала очистить")
		case stateExists && !binExists:
			return fmt.Errorf("--install: обнаружено degraded состояние (state.json есть, бинарь %s отсутствует). Запустите --uninstall для очистки остатков, затем повторите --install", singbox.DefaultBinPath)
		case !stateExists && binExists:
			return fmt.Errorf("--install: бинарь sing-box уже установлен (%s) без state.json. Запустите --uninstall, затем --install", singbox.DefaultBinPath)
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

	// 3. Получить tarball sing-box.
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
		fmt.Printf("Загрузка sing-box (arch=%s)...\n", arch)
		res, err := singbox.Download(ctx, arch, singbox.DefaultCacheDir)
		if err != nil {
			return fmt.Errorf("--install: %w", err)
		}
		fmt.Printf("Скачан %s (%s)\n", res.Path, res.Version)
		tarPath = res.Path
	}

	// 4. Собрать outbounds.
	var outbounds []types.Outbound
	if mode == installInteractive {
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
	if err := state.Save(state.DefaultPath, st); err != nil {
		return fmt.Errorf("--install: state: %w", err)
	}

	// 6. Подготовить бинарь sing-box во временный путь и валидировать конфиг
	// ДО установки в финальный путь — защита от состояния "бинарь установлен,
	// конфиг битый" (safety-fixes #10).
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

	// 7. Атомарно перенести валидированный бинарь в финальный путь.
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

	// 8. Создать init.d shim.
	if err := service.WriteShim(service.DefaultShimPath, service.ShimParams{BinPath: service.DefaultSignCrazeBin}); err != nil {
		return fmt.Errorf("--install: init.d shim: %w", err)
	}

	// 9. Создать NDM netfilter.d hook. Без него правила mangle теряются при
	// первом же rebuild iptables со стороны Keenetic (привязка устройства к
	// policy через UI, save startup-config, reconnect WAN).
	if err := service.WriteHook(service.DefaultNetfilterHookPath, service.HookParams{BinPath: service.DefaultSignCrazeBin}); err != nil {
		return fmt.Errorf("--install: netfilter.d hook: %w", err)
	}

	fmt.Println()
	fmt.Println("Установка завершена.")
	if outbounds[0].Type == "direct" {
		fmt.Println("ВНИМАНИЕ: outbound настроен как 'direct' — проксирования нет.")
		fmt.Println("Запустите: sign-craze --ui on  и настройте через admin UI на :9091.")
	}
	fmt.Println("Запуск сервиса: sign-craze --start")
	return nil
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
	o, err := proxyparse.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("парсинг URL: %w", err)
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

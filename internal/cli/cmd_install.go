package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
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
	if !force {
		if _, err := os.Stat(state.DefaultPath); err == nil {
			return fmt.Errorf("--install: sign-craze уже установлен (state.json существует). Используйте --reinstall для переустановки или --uninstall сначала")
		}
		if _, err := os.Stat(singbox.DefaultBinPath); err == nil {
			return fmt.Errorf("--install: sing-box уже установлен (%s). Используйте --reinstall для переустановки", singbox.DefaultBinPath)
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
		res, err := singbox.Download(ctx, arch, "/tmp")
		if err != nil {
			return fmt.Errorf("--install: %w", err)
		}
		fmt.Printf("Скачан %s (%s)\n", res.Path, res.Version)
		tarPath = res.Path
	}

	// 4. Собрать outbounds.
	var outbounds []types.Outbound
	adminPort := state.DefaultAdminPort
	var adminIPs []string
	if mode == installInteractive {
		ob, err := runProxyWizard(os.Stdin, os.Stdout)
		if err != nil {
			return fmt.Errorf("--install: wizard: %w", err)
		}
		outbounds = ob

		port, ips, err := runAdminBypassWizard(bufio.NewReader(os.Stdin), os.Stdout)
		if err != nil {
			return fmt.Errorf("--install: admin bypass: %w", err)
		}
		adminPort = port
		adminIPs = ips
	}
	if len(outbounds) == 0 {
		// Стаб direct, чтобы sing-box check прошёл.
		outbounds = []types.Outbound{{Tag: "direct", Type: "direct"}}
	}

	// 5. Сохранить state перед установкой бинаря. Default() содержит безопасные
	// CIDR-исключения (RFC1918 + loopback + multicast) — защита от SSH-lockout.
	st := state.Default()
	st.Outbounds = outbounds
	st.AdminPort = adminPort
	st.AdminIPs = adminIPs
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

	tempBin, err := singbox.PrepareAndValidate(ctx, newRunner(), tarPath, configPath(), params)
	if err != nil {
		return fmt.Errorf("--install: %w", err)
	}
	defer os.RemoveAll(filepath.Dir(tempBin))

	// 7. Атомарно перенести валидированный бинарь в финальный путь.
	binData, err := os.ReadFile(tempBin)
	if err != nil {
		return fmt.Errorf("--install: чтение валидированного бинаря: %w", err)
	}
	if _, err := atomicfs.BackupAndReplace(singbox.DefaultBinPath, binData, 0o755); err != nil {
		return fmt.Errorf("--install: установка бинаря: %w", err)
	}

	// 8. Создать init.d shim.
	if err := service.WriteShim(service.DefaultShimPath, service.ShimParams{BinPath: service.DefaultSignCrazeBin}); err != nil {
		return fmt.Errorf("--install: init.d shim: %w", err)
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
func runProxyWizard(in io.Reader, out io.Writer) ([]types.Outbound, error) {
	r := bufio.NewReader(in)

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Настройка outbound прокси:")
	fmt.Fprintln(out, "  1) Полный URL (socks5://, http://, vless://, vmess://, ss://)")
	fmt.Fprintln(out, "  2) Ручной ввод (тип + параметры)")
	fmt.Fprintln(out, "  3) Пропустить (создаст stub direct — sing-box без проксирования)")
	fmt.Fprint(out, "Выбор [1/2/3]: ")
	choice, err := readLineE(r)
	if err != nil {
		return nil, fmt.Errorf("чтение выбора: %w", err)
	}

	switch choice {
	case "1":
		return wizardURL(r, out)
	case "2":
		return wizardManual(r, out)
	case "3", "":
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

func wizardManual(r *bufio.Reader, out io.Writer) ([]types.Outbound, error) {
	fmt.Fprintln(out, "Тип:")
	fmt.Fprintln(out, "  1) socks (socks5)")
	fmt.Fprintln(out, "  2) http")
	fmt.Fprintln(out, "  3) vless")
	fmt.Fprintln(out, "  4) vmess")
	fmt.Fprintln(out, "  5) shadowsocks")
	fmt.Fprint(out, "Выбор [1-5]: ")
	choice := readLine(r)

	var typ string
	switch choice {
	case "1":
		typ = "socks"
	case "2":
		typ = "http"
	case "3":
		typ = "vless"
	case "4":
		typ = "vmess"
	case "5":
		typ = "shadowsocks"
	default:
		return nil, fmt.Errorf("неизвестный тип")
	}

	fmt.Fprint(out, "Сервер (host): ")
	server := readLine(r)
	if server == "" {
		return nil, fmt.Errorf("сервер не указан")
	}

	fmt.Fprint(out, "Порт: ")
	portStr := readLine(r)
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("некорректный порт %q", portStr)
	}

	o := types.Outbound{Tag: typ + "-proxy", Type: typ, Server: server, Port: types.Port(port)}
	settings := map[string]any{}

	switch typ {
	case "socks", "http":
		fmt.Fprint(out, "Логин (Enter — пропустить): ")
		user := readLine(r)
		if user != "" {
			settings["username"] = user
			fmt.Fprint(out, "Пароль: ")
			settings["password"] = readLine(r)
		}
	case "vless":
		fmt.Fprint(out, "UUID: ")
		settings["uuid"] = readLine(r)
		fmt.Fprint(out, "Flow (Enter — без flow): ")
		if f := readLine(r); f != "" {
			settings["flow"] = f
		}
	case "vmess":
		fmt.Fprint(out, "UUID: ")
		settings["uuid"] = readLine(r)
		fmt.Fprint(out, "alterId [0]: ")
		aid := readLine(r)
		if aid == "" {
			aid = "0"
		}
		settings["alterId"] = aid
	case "shadowsocks":
		fmt.Fprint(out, "Метод (например aes-256-gcm): ")
		settings["method"] = readLine(r)
		fmt.Fprint(out, "Пароль: ")
		settings["password"] = readLine(r)
	}

	if len(settings) > 0 {
		o.Settings = settings
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	return []types.Outbound{o}, nil
}

// runAdminBypassWizard опрашивает admin port (default 22) и список admin CIDR/IP
// для исключения из проксирования. Защищает от SSH-lockout (safety-fixes #2).
// Одиночные IP нормализуются в /32 (IPv4) или /128 (IPv6).
func runAdminBypassWizard(r *bufio.Reader, out io.Writer) (uint16, []string, error) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Настройка admin bypass (защита SSH/web admin от проксирования):")
	fmt.Fprint(out, "Admin port [22]: ")
	portStr := readLine(r)
	port := uint16(state.DefaultAdminPort)
	if portStr != "" {
		v, err := strconv.Atoi(portStr)
		if err != nil || v < 1 || v > 65535 {
			return 0, nil, fmt.Errorf("admin port: некорректный %q", portStr)
		}
		port = uint16(v)
	}

	fmt.Fprint(out, "Admin IP/CIDR через запятую (Enter — пропустить): ")
	cidrStr := readLine(r)
	var ips []string
	if cidrStr != "" {
		for _, raw := range strings.Split(cidrStr, ",") {
			tok := strings.TrimSpace(raw)
			if tok == "" {
				continue
			}
			normalized, err := normalizeCIDR(tok)
			if err != nil {
				return 0, nil, fmt.Errorf("admin IP %q: %w", tok, err)
			}
			// Предупреждение для слишком широких CIDR: /16 (65k IP) для admin
			// почти наверняка ошибка ввода. /0 совсем точно отключит проксирование.
			if pfx, perr := netip.ParsePrefix(normalized); perr == nil {
				broadThreshold := 16
				if pfx.Addr().Is6() {
					broadThreshold = 48
				}
				if pfx.Bits() < broadThreshold {
					fmt.Fprintf(out, "  Внимание: %s — широкий CIDR (/%d), весь диапазон будет ИСКЛЮЧЁН из проксирования.\n",
						normalized, pfx.Bits())
				}
			}
			ips = append(ips, normalized)
		}
	}
	return port, ips, nil
}

// normalizeCIDR принимает CIDR или одиночный IP и возвращает CIDR-форму.
func normalizeCIDR(s string) (string, error) {
	if _, err := netip.ParsePrefix(s); err == nil {
		return s, nil
	}
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return "", fmt.Errorf("не CIDR и не IP: %w", err)
	}
	bits := 32
	if ip.Is6() {
		bits = 128
	}
	return fmt.Sprintf("%s/%d", ip.String(), bits), nil
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

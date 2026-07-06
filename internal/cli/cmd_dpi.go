package cli

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/dpi"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/netif"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
	"github.com/kittylabassistant/sign-craze/internal/state"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

func init() {
	Register(Cmd{Long: "--dpi", Help: "включить/выключить DPI: on|off", Handler: handleDPI})
	Register(Cmd{Long: "--dpi-strategy", Help: "установить стратегию DPI: <preset|file://...>", Handler: handleDPIStrategy})
	Register(Cmd{Long: "--dpi-update", Help: "обновить бинарь nfqws2", Handler: handleDPIUpdate})
	Register(Cmd{Long: "--dpi-targets", Help: "selective DPI: список доменов через запятую (пусто/clear = все)", Handler: handleDPITargets})
	Register(Cmd{Long: "--dpi-targets-list", Help: "показать активные DPI targets", Handler: handleDPITargetsList})
	Register(Cmd{Long: "--dpi-exclude-ips", Help: "IP без DPI-десинка через запятую (пусто/clear = нет исключений)", Handler: handleDPIExcludeIPs})
	Register(Cmd{Long: "--dpi-exclude-ips-list", Help: "показать IP-исключения DPI", Handler: handleDPIExcludeIPsList})
	Register(Cmd{Long: "--dpi-update-urls", Help: "URL-источники hostlist для auto-update (через запятую, clear = выкл)", Handler: handleDPIUpdateURLs})
	Register(Cmd{Long: "--dpi-update-interval", Help: "период auto-update hostlist в часах (0 = выкл, рекомендуется 24)", Handler: handleDPIUpdateInterval})
	Register(Cmd{Long: "--dpi-update-now", Help: "немедленно скачать и применить hostlist из dpi_update_urls", Handler: handleDPIUpdateNow})
}

// dpiTargetsStatePath — путь к state.json, которым пользуется handleDPITargets
// (через state.NewDPITargetsManager). var (а не прямая ссылка на константу
// state.DefaultPath) — по тем же причинам, что и portsStatePath в cmd_ports.go:
// юнит-тесты подменяют путь на временный файл, иначе тест бил бы в
// /opt/etc/sign-craze/state.json, недоступный на запись вне роутера.
// В проде значение равно state.DefaultPath.
var dpiTargetsStatePath = state.DefaultPath

func handleDPI(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--dpi: требуется аргумент on|off")
	}
	switch args[0] {
	case "on":
		// dpiEnable делает side-effects (install/detectISPInterface/writeDPIConfig)
		// МЕЖДУ load и save, поэтому остаётся на явном withLock (withStateMutation
		// не подходит — см. R14 в отчёте рефактора).
		return withLock(ctx, func() error { return dpiEnable(ctx) })
	case "off":
		// dpiDisable сериализуется сам через withStateMutation — внешний withLock
		// здесь не нужен (и создал бы двойной захват одного и того же лока).
		return dpiDisable(ctx)
	default:
		return fmt.Errorf("--dpi: неизвестный аргумент %q", args[0])
	}
}

// detectISPInterface определяет ISP-интерфейс через `ip route show default`.
// Общая утилита для всех DPI-функций, которые рендерят nfqws2.conf.
func detectISPInterface(ctx context.Context) (string, error) {
	routeRes, err := exectx.OS.Run(ctx, "ip", "route", "show", "default")
	if err != nil {
		return "", fmt.Errorf("ip route: %w", err)
	}
	iface, err := netif.ParseDefaultRouteIface(string(routeRes.Stdout))
	if err != nil {
		return "", fmt.Errorf("ISP-интерфейс: %w", err)
	}
	return iface, nil
}

// dpiAssetsPresent — true, когда обязательные lua-расширения и blob-payload'ы
// существуют на диске. Достаточно отсутствия одного критичного файла, чтобы
// nfqws2 упал на старте с "bad file ...lua" / "blob not found".
func dpiAssetsPresent() bool {
	required := []string{
		dpi.DefaultLuaDir + "/zapret-lib.lua",
		dpi.DefaultLuaDir + "/zapret-antidpi.lua",
		dpi.DefaultLuaDir + "/zapret-auto.lua",
		dpi.DefaultBlobDir + "/quic_initial.bin",
		dpi.DefaultBlobDir + "/tls_clienthello.bin",
	}
	for _, p := range required {
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

// installNfqws2WithBlobs скачивает .ipk пакет nfqws2-keenetic, устанавливает
// бинарь и распаковывает blob-файлы (quic_initial.bin, tls_clienthello.bin).
// Используется в dpiEnable и в --install --with-dpi.
func installNfqws2WithBlobs(ctx context.Context) error {
	arch, err := types.DetectHostArch()
	if err != nil {
		return fmt.Errorf("arch: %w", err)
	}
	if mkErr := os.MkdirAll(singbox.DefaultCacheDir, 0o755); mkErr != nil {
		return fmt.Errorf("mkdir cache: %w", mkErr)
	}
	fmt.Printf("Загрузка nfqws2 (arch=%s)...\n", arch)
	res, dlErr := dpi.Download(ctx, arch, singbox.DefaultCacheDir)
	if dlErr != nil {
		return fmt.Errorf("download: %w", dlErr)
	}
	if instErr := dpi.Install(res.Path, dpi.DefaultBinPath); instErr != nil {
		return fmt.Errorf("install: %w", instErr)
	}
	if assetErr := dpi.InstallAssets(res.Path, dpi.DefaultBlobDir, dpi.DefaultLuaDir); assetErr != nil {
		// Не фатально — без lua/blob'ов часть стратегий не работает, но fallback
		// (без --lua-desync=fake:blob=...) может функционировать.
		return fmt.Errorf("install assets: %w", assetErr)
	}
	return nil
}

func dpiEnable(ctx context.Context) error {
	st, err := loadState()
	if err != nil {
		return err
	}

	// Установка бинаря + blob/lua-ассетов, если отсутствует хотя бы что-то.
	// Раньше проверялся только бинарь — но на роутере мог остаться бинарь
	// от прошлой установки (например после ручного opkg install nfqws2-keenetic
	// или прерванного --dpi on), а ассеты отсутствовать. Без zapret-lib.lua
	// nfqws2 падает с "bad file ... .lua" сразу после старта, и --dpi on
	// «успешно» завершался, оставляя пользователя со сломанной DPI.
	binMissing := false
	if _, statErr := os.Stat(dpi.DefaultBinPath); statErr != nil {
		binMissing = true
	}
	assetsMissing := !dpiAssetsPresent()
	if binMissing || assetsMissing {
		if instErr := installNfqws2WithBlobs(ctx); instErr != nil {
			return fmt.Errorf("--dpi on: %w", instErr)
		}
	}

	iface, err := detectISPInterface(ctx)
	if err != nil {
		return fmt.Errorf("--dpi on: %w", err)
	}

	// Сгенерировать nfqws2.conf (+ hostlist если targets заданы).
	if err := writeDPIConfig(iface, st); err != nil {
		return fmt.Errorf("--dpi on: %w", err)
	}

	st.DPIEnabled = true
	if err := saveState(st); err != nil {
		return err
	}
	fmt.Println("DPI включён. Перезапустите сервис: sign-craze --restart")
	return nil
}

// writeDPIConfig пишет nfqws2.conf и (опционально) dpi-hostlist.txt из state.
//
// st.DPIStrategy непустой → override NFQWS_ARGS (TCP/TLS-блок).
//
// Hostlist (--hostlist=<path>) подключается если выполнено хотя бы одно:
//  1. st.DPITargets непустой → hostlist пишется атомарно из этого списка;
//  2. /opt/etc/sign-craze/dpi-hostlist.txt существует (auto-update создал
//     из upstream-источников) — флаг ставится без перезаписи файла.
//
// Если оба условия false → флаг --hostlist не добавляется (desync для
// всего трафика).
func writeDPIConfig(iface string, st *state.State) error {
	params := dpi.DefaultConfigParams()
	params.ISPInterface = iface
	if st != nil && st.DPIStrategy != "" {
		params.Args = st.DPIStrategy
	}
	if st != nil && len(st.DPITargets) > 0 {
		if err := dpi.WriteHostlist(dpi.DefaultHostlistPath, st.DPITargets); err != nil {
			return fmt.Errorf("hostlist: %w", err)
		}
	}
	if hostlistShouldApply(st) {
		params.HostlistPath = dpi.DefaultHostlistPath
	}
	if err := dpi.GenerateConfig(params, dpi.DefaultConfigPath); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}

func dpiDisable(ctx context.Context) error {
	if err := withStateMutation(ctx, func(st *state.State) error {
		st.DPIEnabled = false
		return nil
	}); err != nil {
		return err
	}
	fmt.Println("DPI выключен. Перезапустите сервис: sign-craze --restart")
	return nil
}

// dpiStrategyForbidden — shell-метасимволы и управляющие символы, которых
// не должно быть в --dpi-strategy. Дефолтные стратегии nfqws2 содержат
// только alphanumeric, `-`, `_`, `.`, `,`, `:`, `=`, `@`, `/`, пробел.
// Запрещённые символы могут привести к экспедиции в exec.Command аргументы
// нежелательных флагов через `splitArgs(strings.Fields(...))` или к
// shell-инъекции если стратегия попадёт в shim-script.
const dpiStrategyForbidden = ";&|$`<>(){}\\\n\r\t\x00"

const dpiStrategyMaxLen = 1024

// validateDPIStrategy — blocklist shell-метасимволов в строке стратегии.
// Allowlist слишком узок: легитимные стратегии содержат непредсказуемый
// набор символов в hostnames (`sni=fonts.google.com`), числах (`pos=1,midsld`)
// и т.п. Blocklist чёрный — короткий, безопасный, не ломает upstream-стратегии.
func validateDPIStrategy(s string) error {
	if s == "" {
		return fmt.Errorf("--dpi-strategy: пустая строка")
	}
	if len(s) > dpiStrategyMaxLen {
		return fmt.Errorf("--dpi-strategy: длина %d > лимита %d", len(s), dpiStrategyMaxLen)
	}
	if i := strings.IndexAny(s, dpiStrategyForbidden); i >= 0 {
		return fmt.Errorf("--dpi-strategy: запрещённый символ %q в позиции %d (shell-метасимволы и управляющие не допускаются)", s[i], i)
	}
	return nil
}

func handleDPIStrategy(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--dpi-strategy: требуется preset или file://путь")
	}
	strategy := strings.TrimSpace(args[0])
	if err := validateDPIStrategy(strategy); err != nil {
		return err
	}
	if err := withStateMutation(ctx, func(st *state.State) error {
		st.DPIStrategy = strategy
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("DPI-стратегия установлена: %s\n", strategy)
	fmt.Println("Перезапустите сервис: sign-craze --restart")
	return nil
}

func handleDPITargets(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--dpi-targets: требуется список доменов (или 'clear' для очистки)")
	}
	raw := strings.TrimSpace(args[0])
	var targets []string
	switch raw {
	case "clear", "none", "-", "":
		targets = nil
	default:
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			targets = append(targets, part)
		}
	}

	return withLock(ctx, func() error {
		return setDPITargets(ctx, dpiTargetsStatePath, targets)
	})
}

// setDPITargets сохраняет targets через state.NewDPITargetsManager — тот же
// менеджер (и тот же lock-файл на state.json), которым пользуется Web UI.
// Раньше handleDPITargets мутировал ВЕСЬ *state.State в памяти
// (st.DPITargets = targets) и сохранял его целиком через saveState() под
// ДРУГИМ локом (locks.DefaultPath, см. withLock) — гонка с Web UI (B4):
// raw Save мог затереть конкурентную правку Web UI (например, добавленный
// порт), примененную и сохранённую менеджером между raw Load и raw Save.
//
// Порядок действий важен: SetTargets сохраняет targets ПЕРВЫМ (атомарно, под
// общим с Web UI локом). Регенерация nfqws2-конфига — side-effect, которого
// нет в state.json, поэтому она выполняется ПОСЛЕ успешного SetTargets, на
// свежепрочитанном через state.Load состоянии (нужны актуальные
// DPIEnabled/DPIStrategy/DPITargets). Если регенерация конфига упадёт —
// targets в state.json уже сохранены (см. подводные камни в описании фикса).
//
// Вынесено из handleDPITargets отдельной функцией, чтобы тестировать мутацию
// без внешнего withLock (системный /opt/var/lock/sign-craze.lock, путь
// захардкожен в lock.go и не подменяется в юнит-тестах).
func setDPITargets(ctx context.Context, statePath string, targets []string) error {
	mgr := state.NewDPITargetsManager(statePath)
	if err := mgr.SetTargets(ctx, targets); err != nil {
		return fmt.Errorf("--dpi-targets: %w", err)
	}

	st, err := state.Load(statePath)
	if err != nil {
		return fmt.Errorf("--dpi-targets: %w", err)
	}

	// Регенерация конфига имеет смысл только если DPI уже включён.
	// Иначе конфиг будет создан при следующем `--dpi on` с актуальными targets.
	if st.DPIEnabled {
		if err := regenDPIConfigFn(ctx, st); err != nil {
			return fmt.Errorf("--dpi-targets: %w", err)
		}
	}

	if len(targets) == 0 {
		fmt.Println("DPI targets очищены (desync применяется ко всему трафику).")
	} else {
		fmt.Printf("DPI targets: %d домен(ов). Перезапустите: sign-craze --restart\n", len(targets))
	}
	return nil
}

// regenDPIConfigFn — регенерация nfqws2-конфига после смены targets при
// включённом DPI. Seam для тестов: продакшен-реализация детектит
// WAN-интерфейс через `ip route` и пишет в захардкоженные пути internal/dpi
// (/opt/etc/sign-craze/...) — в юнит-тестах и то и другое недетерминировано
// (на GitHub-раннере /opt записываем, в песочнице — нет), поэтому тесты
// подменяют функцию целиком.
var regenDPIConfigFn = func(ctx context.Context, st *state.State) error {
	iface, ifErr := detectISPInterface(ctx)
	if ifErr != nil {
		return ifErr
	}
	return writeDPIConfig(iface, st)
}

func handleDPITargetsList(_ context.Context, _ []string) error {
	st, err := loadState()
	if err != nil {
		return err
	}
	if len(st.DPITargets) == 0 {
		fmt.Println("DPI targets не заданы (desync применяется ко всему трафику).")
		return nil
	}
	for _, t := range st.DPITargets {
		fmt.Println(t)
	}
	return nil
}

func handleDPIExcludeIPs(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--dpi-exclude-ips: требуется список IP (или 'clear' для очистки)")
	}
	raw := strings.TrimSpace(args[0])
	var ips []string
	switch raw {
	case "clear", "none", "-", "":
		ips = nil
	default:
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, err := netip.ParseAddr(part); err != nil {
				return fmt.Errorf("--dpi-exclude-ips: некорректный IP %q: %w", part, err)
			}
			ips = append(ips, part)
		}
	}
	if err := withStateMutation(ctx, func(st *state.State) error {
		st.DPIExcludeIPs = ips
		return nil
	}); err != nil {
		return err
	}
	if len(ips) == 0 {
		fmt.Println("DPI exclude-IPs очищены.")
	} else {
		fmt.Printf("DPI exclude-IPs: %d адрес(ов). Перезапустите: sign-craze --restart\n", len(ips))
	}
	return nil
}

func handleDPIExcludeIPsList(_ context.Context, _ []string) error {
	st, err := loadState()
	if err != nil {
		return err
	}
	if len(st.DPIExcludeIPs) == 0 {
		fmt.Println("DPI exclude-IPs не заданы.")
		return nil
	}
	for _, ip := range st.DPIExcludeIPs {
		fmt.Println(ip)
	}
	return nil
}

func handleDPIUpdateURLs(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--dpi-update-urls: требуется список URL (или 'clear' для отключения)")
	}
	raw := strings.TrimSpace(args[0])
	var urls []string
	switch raw {
	case "clear", "none", "-", "":
		urls = nil
	default:
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if !strings.HasPrefix(part, "http://") && !strings.HasPrefix(part, "https://") {
				return fmt.Errorf("--dpi-update-urls: URL %q должен начинаться с http:// или https://", part)
			}
			urls = append(urls, part)
		}
	}
	if err := withStateMutation(ctx, func(st *state.State) error {
		st.DPIUpdateURLs = urls
		return nil
	}); err != nil {
		return err
	}
	if len(urls) == 0 {
		fmt.Println("DPI auto-update выключен.")
	} else {
		fmt.Printf("DPI auto-update URLs: %d. Запустите: sign-craze --dpi-update-now\n", len(urls))
	}
	return nil
}

func handleDPIUpdateInterval(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("--dpi-update-interval: требуется число часов (0 = выкл)")
	}
	hours, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil {
		return fmt.Errorf("--dpi-update-interval: некорректное число %q: %w", args[0], err)
	}
	if hours < 0 {
		return fmt.Errorf("--dpi-update-interval: значение не может быть отрицательным")
	}
	if err := withStateMutation(ctx, func(st *state.State) error {
		st.DPIUpdateIntervalHours = hours
		return nil
	}); err != nil {
		return err
	}
	if hours == 0 {
		fmt.Println("DPI auto-update interval=0 (отключено).")
	} else {
		fmt.Printf("DPI auto-update interval=%dч.\n", hours)
	}
	return nil
}

func handleDPIUpdateNow(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		st, err := loadState()
		if err != nil {
			return err
		}
		if len(st.DPIUpdateURLs) == 0 {
			return fmt.Errorf("--dpi-update-now: dpi_update_urls пуст; задайте через --dpi-update-urls")
		}
		report, err := dpi.UpdateHostlist(ctx, st.DPIUpdateURLs, st.DPITargets, dpi.DefaultHostlistPath)
		if err != nil {
			return fmt.Errorf("--dpi-update-now: %w", err)
		}
		st.DPILastUpdate = report.Timestamp.UTC().Format("2006-01-02T15:04:05Z07:00")
		if err := saveState(st); err != nil {
			return fmt.Errorf("--dpi-update-now: сохранение state: %w", err)
		}
		fmt.Printf("Hostlist обновлён: %d доменов, %d источников успешно, %d ошибок.\n",
			report.TotalHosts, report.SourcesOK, report.SourcesFailed)
		if st.DPIEnabled {
			fmt.Println("Перезапустите для применения: sign-craze --restart")
		}
		return nil
	})
}

func handleDPIUpdate(ctx context.Context, _ []string) error {
	return withLock(ctx, func() error {
		arch, err := types.DetectHostArch()
		if err != nil {
			return fmt.Errorf("--dpi-update: %w", err)
		}
		if mkErr := os.MkdirAll(singbox.DefaultCacheDir, 0o755); mkErr != nil {
			return fmt.Errorf("--dpi-update: mkdir cache: %w", mkErr)
		}
		fmt.Printf("Загрузка nfqws2 (arch=%s)...\n", arch)
		res, err := dpi.Download(ctx, arch, singbox.DefaultCacheDir)
		if err != nil {
			return fmt.Errorf("--dpi-update: %w", err)
		}
		// Бинарь устанавливаем только если свежее. Assets (lua/blobs)
		// перепроверяем всегда — пользователь мог удалить директории
		// или мы добавили новые требования (lua-расширения в v0.x → v1.x).
		if res.Downloaded {
			if err := dpi.Install(res.Path, dpi.DefaultBinPath); err != nil {
				return fmt.Errorf("--dpi-update: %w", err)
			}
		}
		if err := dpi.InstallAssets(res.Path, dpi.DefaultBlobDir, dpi.DefaultLuaDir); err != nil {
			return fmt.Errorf("--dpi-update: install assets: %w", err)
		}
		if res.Downloaded {
			fmt.Printf("nfqws2 обновлён до %s.\n", res.Version)
		} else {
			fmt.Printf("nfqws2 актуален (%s); ресурсы (lua, blobs) перепроверены.\n", res.Version)
		}
		return nil
	})
}

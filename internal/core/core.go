package core

import (
	"context"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// ConfigFormat определяет формат конфига ядра.
type ConfigFormat int

const (
	// ConfigJSON — JSON-конфиг (sing-box, xray).
	ConfigJSON ConfigFormat = iota
	// ConfigYAML — YAML-конфиг (mihomo/clash).
	ConfigYAML
)

// DownloadResult — результат вызова Core.Download.
type DownloadResult struct {
	// Downloaded — false, если ETag совпал и файл не перезаписан.
	Downloaded bool
	// Version — тег релиза ("v1.10.0", "v1.8.4").
	Version string
	// Path — абсолютный путь к скачанному archive (.tar.gz / .zip / .gz).
	Path string
}

// GeoFormat определяет формат rule-set/geo-данных, ожидаемый ядром.
type GeoFormat int

const (
	// GeoSRS — sing-box rule-set (.srs).
	GeoSRS GeoFormat = iota
	// GeoDAT — v2ray geoip.dat / geosite.dat (xray).
	GeoDAT
	// GeoMRS — mihomo rule-set (.mrs).
	GeoMRS
)

// Core — общий интерфейс для всех прокси-ядер. Регистрация через registry.go.
// Реализации: internal/core/singbox, internal/core/xray, internal/core/mihomo.
//
// Каждое ядро (sing-box, xray, mihomo) реализует Core и регистрируется в
// registry. CLI-команды получают Core через registry.Get(name) — без прямой
// зависимости на конкретное ядро.
type Core interface {
	// Name — каноническое имя ядра ("sing-box", "xray", "mihomo").
	// Совпадает с ключом в registry и значением state.Core.
	Name() string

	// BinaryPath — абсолютный путь к бинарю ядра (/opt/sbin/<name>).
	BinaryPath() string

	// ConfigDir — директория конфигов и состояния ядра.
	// Sing-box: /opt/etc/sign-craze. Xray/mihomo: /opt/etc/sign-craze/<name>.
	// Используется backup/restore, --uninstall, web UI для адресации файлов.
	ConfigDir() string

	// CacheDir — директория для tarball-кеша и временной распаковки.
	// Должна быть на постоянной FS с GB места (не tmpfs) — на Keenetic /tmp
	// это обычно ~50MB tmpfs, не хватит для распаковки бинарей.
	CacheDir() string

	// ConfigPath — абсолютный путь к файлу конфига ядра.
	// Sing-box: <ConfigDir>/config.json. Mihomo: <ConfigDir>/config.yaml.
	ConfigPath() string

	// PIDPath — путь к PID-файлу ядра. service.Lifecycle и netfilter-hook
	// читают это значение, поэтому для каждого ядра путь должен быть
	// уникальным (подпись имени ядра).
	PIDPath() string

	// ConfigFormat — формат конфига (JSON/YAML).
	ConfigFormat() ConfigFormat

	// GeoFormat — ожидаемый формат rule-set/geo-данных.
	GeoFormat() GeoFormat

	// NewLifecycle создаёт service.Lifecycle для запуска/остановки процесса.
	// Аргументы запуска (LaunchArgs) и пути зашиты внутри Core.
	NewLifecycle() service.Lifecycle

	// Download скачивает архив с бинарём ядра под указанную архитектуру
	// в dstDir. Возвращает путь к archive (.tar.gz/.zip/.gz) и тег релиза.
	// При совпадении ETag не перезаписывает файл (Downloaded=false).
	Download(ctx context.Context, arch types.Arch, dstDir string) (DownloadResult, error)

	// Install устанавливает бинарь ядра из archive в BinaryPath().
	//
	//  1. Распаковка archive (поток, без буферизации в RAM).
	//  2. ELF-magic check.
	//  3. Атомарная запись в BinaryPath() через atomicfs.
	//
	// НЕ валидирует config (для этого CheckConfig вызывается отдельно
	// после установки бинаря — иначе на свежей системе config'а ещё нет).
	Install(ctx context.Context, runner exectx.Runner, archivePath string) error

	// CheckConfig запускает встроенную валидацию конфига ядра
	// (sing-box check / xray test / mihomo -t). Возвращает nil при OK.
	CheckConfig(ctx context.Context, runner exectx.Runner, configPath string) error

	// BinaryVersion запрашивает версию установленного ядра через `<binary> version`
	// (или эквивалентную команду). Возвращает "X.Y.Z" без префикса "v".
	BinaryVersion(ctx context.Context, runner exectx.Runner) (string, error)

	// ParseVersion извлекает версию из вывода `<binary> version`.
	// stdoutFirstLine — первая строка вывода (без \n).
	ParseVersion(stdoutFirstLine string) (string, error)

	// RenderConfig генерирует конфиг ядра из types.CoreRenderParams.
	//
	// Каждое ядро строит собственные ConfigParams из переданных параметров
	// и вызывает свой Render.
	// Возвращает []byte — JSON (sing-box, xray) или YAML (mihomo).
	//
	// CoreRenderParams живёт в pkg/types, а не internal/state, чтобы избежать
	// circular import (core → state → web → core). CLI заполняет его из state.State.
	//
	// Используется CLI (cmd_lifecycle.doStart) и web routing UI для preview.
	RenderConfig(p types.CoreRenderParams) ([]byte, error)

	// ValidateOutbound проверяет, поддерживает ли ядро данный outbound.
	// Возвращает nil если совместимо, иначе ошибку с подсказкой про другое ядро.
	//
	// Используется auto-detect логикой (internal/core/detect.go) для выбора
	// ядра по URL — каждое ядро инкапсулирует собственную матрицу совместимости
	// (sing-box не поддерживает PQ-VLESS, xray не поддерживает TUIC/WG/hy2,
	// mihomo не поддерживает Vision UDP443 и т.п.).
	ValidateOutbound(ob types.Outbound) error
}

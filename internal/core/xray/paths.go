package xray

const (
	// DefaultBinPath — путь установки бинаря xray.
	DefaultBinPath = "/opt/sbin/xray"
	// DefaultConfigDir — каталог конфигов и состояния xray.
	// Отделён от sing-box, чтобы оба ядра могли сосуществовать на одном
	// устройстве. Структура: <dir>/config.json (xray), assets/ (geoip.dat,
	// geosite.dat) — см. Phase B.x.
	DefaultConfigDir = "/opt/etc/sign-craze/xray"
	// DefaultConfigPath — путь к config.json xray.
	DefaultConfigPath = DefaultConfigDir + "/config.json"
	// DefaultCacheDir — каталог кеша zip-архивов и временной распаковки.
	// На постоянной FS (флешка), не tmpfs — распаковка xray ~16MB не
	// помещается в /tmp 50MB на Keenetic.
	DefaultCacheDir = "/opt/var/lib/sign-craze/cache"
	// DefaultPIDFile — путь к PID-файлу xray.
	DefaultPIDFile = "/opt/var/run/sign-craze-xray.pid"
	// DefaultStderrLog — лог stderr xray для диагностики ранних crash'ей.
	DefaultStderrLog = "/opt/var/log/sign-craze/xray.stderr.log"
)

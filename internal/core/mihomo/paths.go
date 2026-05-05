package mihomo

const (
	// DefaultBinPath — путь установки бинаря mihomo.
	DefaultBinPath = "/opt/sbin/mihomo"
	// DefaultConfigDir — каталог конфигов и состояния mihomo.
	// Mihomo читает config.yaml из этой директории (флаг `-d`).
	// Также здесь живут cache.db и provider/-файлы (rule-providers).
	DefaultConfigDir = "/opt/etc/sign-craze/mihomo"
	// DefaultConfigPath — путь к config.yaml mihomo (для совместимости с
	// core.Core.ConfigPath()). На уровне команды запуска используется ConfigDir.
	DefaultConfigPath = DefaultConfigDir + "/config.yaml"
	// DefaultCacheDir — каталог кеша gzip-архивов и временной распаковки.
	DefaultCacheDir = "/opt/var/lib/sign-craze/cache"
	// DefaultPIDFile — путь к PID-файлу mihomo.
	DefaultPIDFile = "/opt/var/run/sign-craze-mihomo.pid"
	// DefaultStderrLog — лог stderr mihomo для диагностики ранних crash'ей.
	DefaultStderrLog = "/opt/var/log/sign-craze/mihomo.stderr.log"
)

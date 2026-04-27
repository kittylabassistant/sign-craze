package web

import "context"

// StatusInfo — ответ GET /api/status.
type StatusInfo struct {
	SingBox ServiceState `json:"singbox"`
	Nfqws2  ServiceState `json:"nfqws2"`
	Mode    string       `json:"mode"`
	Version VersionInfo  `json:"version"`
	UptimeS int64        `json:"uptime_s"`
}

// ServiceState описывает состояние одного сервиса.
type ServiceState struct {
	Running bool `json:"running"`
	PID     int  `json:"pid"`
}

// VersionInfo содержит версии компонентов.
type VersionInfo struct {
	SignCraze string `json:"sign_craze"`
	SingBox   string `json:"sing_box"`
}

// StatusReader возвращает текущее состояние сервисов.
type StatusReader interface {
	Status(ctx context.Context) (StatusInfo, error)
}

// ConfigRW читает и обновляет config.json sing-box.
type ConfigRW interface {
	ReadConfig(ctx context.Context) ([]byte, error)
	WriteConfig(ctx context.Context, data []byte) error
}

// PortsManager управляет списком проксируемых портов.
type PortsManager interface {
	ListPorts(ctx context.Context) ([]int, error)
	AddPort(ctx context.Context, port int) error
	DeletePort(ctx context.Context, port int) error
}

// ExcludesManager управляет IP/CIDR-исключениями.
type ExcludesManager interface {
	ListExcludes(ctx context.Context) ([]string, error)
	AddExclude(ctx context.Context, cidr string) error
	DeleteExclude(ctx context.Context, cidr string) error
}

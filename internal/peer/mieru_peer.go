package peer

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

// Стандартные пути supervised peer mieru. Совместимы с filesystem-схемой
// BEHAVIOR_SPEC §5 и инвариантами §7.
const (
	// DefaultMieruBinPath — путь к бинарю mieru-клиента, который sign-craze
	// устанавливает при `--install` (см. ADR-0020, M6 в плане).
	DefaultMieruBinPath = "/opt/sbin/mieru"

	// DefaultMieruPIDDir — директория PID-файлов peers. Отдельная от
	// /opt/var/run/sign-craze-*.pid (core), чтобы peers не путались с core.
	DefaultMieruPIDDir = "/opt/var/run/sign-craze"

	// DefaultMieruLogDir — общая директория логов sign-craze. Каждый peer
	// получает свой файл `mieru-<tag>.log`.
	DefaultMieruLogDir = "/opt/var/log/sign-craze"
)

// MieruPaths собирает пути, использованные при супервизии конкретного peer.
// Возвращается из MieruPathsFor — даёт чистый структурированный доступ к
// PID-файлу, лог-файлу и пути конфига, без хардкода путей в нескольких местах.
type MieruPaths struct {
	BinPath    string
	ConfigPath string
	PIDFile    string
	LogPath    string
}

// MieruPathsFor вычисляет пути для одного mieru-outbound по тегу.
// Используется как при `--start` (запуск), так и при `--stop` (поиск PID-файла).
func MieruPathsFor(tag string) MieruPaths {
	return MieruPaths{
		BinPath:    DefaultMieruBinPath,
		ConfigPath: MieruConfigPath(MieruConfigDir, tag),
		PIDFile:    filepath.Join(DefaultMieruPIDDir, "mieru-"+tag+".pid"),
		LogPath:    filepath.Join(DefaultMieruLogDir, "mieru-"+tag+".log"),
	}
}

// NewMieruLifecycle строит service.Lifecycle для одного mieru-peer.
// Используется внутри StartMieruPeers/StopMieruPeers, но экспортируется
// чтобы тесты cmd_lifecycle могли инъецировать stub-lifecycle.
//
// CLI mieru: `mieru run -c <config>` запускает клиента в foreground режиме.
// `service.Lifecycle.waitAlive` корректно отслеживает PID, потому что mieru
// не делает self-daemonize.
func NewMieruLifecycle(paths MieruPaths) service.Lifecycle {
	return service.NewLifecycle(service.ProcessConfig{
		Name:       "mieru-" + filepath.Base(paths.ConfigPath),
		BinPath:    paths.BinPath,
		Args:       []string{"run", "-c", paths.ConfigPath},
		PIDFile:    paths.PIDFile,
		StderrPath: paths.LogPath,
	})
}

// StartMieruPeers поднимает все mieru-outbound'ы из списка. Останавливается на
// первой ошибке, при этом ранее запущенные peers НЕ откатываются — caller
// (`doStart` в cli) должен решить, делать ли cleanup через StopMieruPeers.
//
// Outbound'ы, уже запущенные (Status.Running=true), пропускаются — это даёт
// идемпотентность повторного `--start` или race против watchdog'а.
func StartMieruPeers(ctx context.Context, outbounds []types.Outbound) error {
	for _, ob := range outbounds {
		if ob.Protocol != types.ProtocolMieru {
			continue
		}
		if err := startOne(ctx, ob); err != nil {
			return err
		}
	}
	return nil
}

func startOne(ctx context.Context, ob types.Outbound) error {
	paths := MieruPathsFor(ob.Tag)
	lc := NewMieruLifecycle(paths)

	st, err := lc.Status(ctx)
	if err != nil {
		return fmt.Errorf("peer mieru %q: status: %w", ob.Tag, err)
	}
	if st.Running {
		log.L().Info("mieru peer уже запущен", "tag", ob.Tag, "pid", st.PID)
		return nil
	}

	if err := lc.Start(ctx); err != nil {
		return fmt.Errorf("peer mieru %q: start: %w", ob.Tag, err)
	}
	log.L().Info("mieru peer запущен", "tag", ob.Tag, "config", paths.ConfigPath, "local_port", ob.Proto.MieruLocalPort)
	return nil
}

// StopMieruPeers останавливает все mieru-outbound'ы из списка. Ошибки
// логируются, но не прерывают цикл — sign-craze должен попытаться остановить
// все peers, даже если один из них не отвечает.
//
// Порядок остановки обратный порядку Outbound'ов — на случай если позднее
// появится зависимость между peers (на v0.X.Y такого нет).
func StopMieruPeers(ctx context.Context, outbounds []types.Outbound) {
	for i := len(outbounds) - 1; i >= 0; i-- {
		ob := outbounds[i]
		if ob.Protocol != types.ProtocolMieru {
			continue
		}
		paths := MieruPathsFor(ob.Tag)
		lc := NewMieruLifecycle(paths)
		if err := lc.Stop(ctx); err != nil {
			log.L().Warn("остановка mieru peer не удалась", "tag", ob.Tag, "err", err)
		}
	}
}

// MieruPeerStatus описывает наблюдаемое состояние одного peer.
// Используется в --status и в /api/status web-ui.
type MieruPeerStatus struct {
	Tag       string `json:"tag"`
	Running   bool   `json:"running"`
	PID       int    `json:"pid"`
	LocalPort int    `json:"local_port"`
	Config    string `json:"config_path"`
}

// CollectMieruStatuses возвращает статус всех mieru-peers из outbounds.
// Outbound'ы других протоколов пропускаются. Ошибки при чтении PID-файла
// преобразуются в Running=false (не фатально).
func CollectMieruStatuses(ctx context.Context, outbounds []types.Outbound) []MieruPeerStatus {
	var out []MieruPeerStatus
	for _, ob := range outbounds {
		if ob.Protocol != types.ProtocolMieru {
			continue
		}
		paths := MieruPathsFor(ob.Tag)
		lc := NewMieruLifecycle(paths)
		st, sErr := lc.Status(ctx)
		entry := MieruPeerStatus{
			Tag:    ob.Tag,
			Config: paths.ConfigPath,
		}
		if ob.Proto != nil {
			entry.LocalPort = ob.Proto.MieruLocalPort
		}
		if sErr == nil {
			entry.Running = st.Running
			entry.PID = st.PID
		}
		out = append(out, entry)
	}
	return out
}

// HasMieruOutbounds возвращает true если в списке есть хотя бы один
// mieru-outbound. Используется в install/start чтобы условно вызывать
// peer-логику только когда mieru реально нужен.
func HasMieruOutbounds(outbounds []types.Outbound) bool {
	for _, ob := range outbounds {
		if ob.Protocol == types.ProtocolMieru {
			return true
		}
	}
	return false
}

package state

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/locks"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/version"
	"github.com/kittylabassistant/sign-craze/internal/web"
)

// stateLockTimeout — лимит ожидания flock на state-файле для веб-API.
// 5 секунд достаточно для последовательных мутаций; дольше — возвращаем ошибку,
// чтобы клиент попробовал снова, а не висел.
const stateLockTimeout = 5 * time.Second

// withStateLock сериализует Load→mutate→Save через flock на отдельном lock-файле
// рядом со state.json (statePath + ".lock"). Защищает от TOCTOU race между
// одновременными запросами Add/Delete из web API + CLI (safety-fixes B.2).
func withStateLock(parent context.Context, statePath string, fn func() error) error {
	ctx, cancel := context.WithTimeout(parent, stateLockTimeout)
	defer cancel()
	lock, err := locks.Acquire(ctx, statePath+".lock")
	if err != nil {
		return fmt.Errorf("state: lock %s: %w", statePath, err)
	}
	defer func() {
		if rErr := lock.Release(); rErr != nil {
			slog.Warn("state: ошибка снятия lock", "path", statePath, "err", rErr)
		}
	}()
	return fn()
}

// withState — общий скелет Load→mutate→Save под withStateLock. Вынесен из
// AddPort/DeletePort/AddExclude/DeleteExclude/SetTargets: единственное, чем
// эти методы отличались друг от друга — что именно мутировать в *State.
// Если mutate вернёт ошибку — Save НЕ вызывается (диск не тронут).
func withState(ctx context.Context, statePath string, mutate func(*State) error) error {
	return withStateLock(ctx, statePath, func() error {
		s, err := Load(statePath)
		if err != nil {
			return err
		}
		if err := mutate(s); err != nil {
			return err
		}
		return Save(statePath, s)
	})
}

// SingboxVersion — функция получения версии sing-box; var для подмены в тестах.
// По умолчанию вызывает singbox.BinaryVersion (внедряется в init()).
var SingboxVersion = func(_ context.Context, _ exectx.Runner, _ string) (string, error) {
	return "", nil
}

// statusReader реализует web.StatusReader из state + Lifecycle.
type statusReader struct {
	singboxLC  service.Lifecycle
	dpiLC      service.Lifecycle
	statePath  string
	singboxBin string
	runner     exectx.Runner
	startedAt  int64 // Unix-сек запуска процесса (uptime от него)
}

// NewStatusReader конструирует StatusReader.
// startedAt — момент создания (используется для uptime в /api/status).
func NewStatusReader(
	singboxLC, dpiLC service.Lifecycle,
	statePath, singboxBin string,
	runner exectx.Runner,
	startedAt int64,
) web.StatusReader {
	return &statusReader{
		singboxLC:  singboxLC,
		dpiLC:      dpiLC,
		statePath:  statePath,
		singboxBin: singboxBin,
		runner:     runner,
		startedAt:  startedAt,
	}
}

func (r *statusReader) Status(ctx context.Context) (web.StatusInfo, error) {
	st, err := Load(r.statePath)
	if err != nil {
		return web.StatusInfo{}, err
	}

	sb, err := r.singboxLC.Status(ctx)
	if err != nil {
		return web.StatusInfo{}, fmt.Errorf("singbox status: %w", err)
	}
	dp, err := r.dpiLC.Status(ctx)
	if err != nil {
		return web.StatusInfo{}, fmt.Errorf("dpi status: %w", err)
	}

	sbVer := ""
	if _, errStat := os.Stat(r.singboxBin); errStat == nil {
		v, vErr := SingboxVersion(ctx, r.runner, r.singboxBin)
		if vErr == nil {
			sbVer = v
		}
	}

	uptime := int64(0)
	if r.startedAt > 0 {
		uptime = nowUnix() - r.startedAt
	}

	return web.StatusInfo{
		SingBox: web.ServiceState{Running: sb.Running, PID: sb.PID},
		Nfqws2:  web.ServiceState{Running: dp.Running, PID: dp.PID},
		Mode:    string(st.Mode),
		Version: web.VersionInfo{
			SignCraze: version.Version,
			SingBox:   sbVer,
		},
		UptimeS: uptime,
	}, nil
}

// nowUnix — var для подмены в тестах.
var nowUnix = timeNowSec

// configRW реализует web.ConfigRW: чтение/запись /opt/etc/sign-craze/config.json
// с валидацией через `sing-box check -c`.
type configRW struct {
	configPath string
	runner     exectx.Runner
	singboxBin string
}

// NewConfigRW конструирует ConfigRW.
// При WriteConfig выполняется `<singboxBin> check -c <tmpPath>`; если бинарь отсутствует — проверка пропускается.
func NewConfigRW(configPath, singboxBin string, runner exectx.Runner) web.ConfigRW {
	return &configRW{
		configPath: configPath,
		runner:     runner,
		singboxBin: singboxBin,
	}
}

func (c *configRW) ReadConfig(_ context.Context) ([]byte, error) {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		return nil, fmt.Errorf("config: чтение %s: %w", c.configPath, err)
	}
	return data, nil
}

func (c *configRW) WriteConfig(ctx context.Context, data []byte) error {
	if _, err := os.Stat(c.singboxBin); err == nil && c.runner != nil {
		// Валидация во временном файле перед атомарной записью.
		tmp, err := os.CreateTemp("", "config-*.json")
		if err != nil {
			return fmt.Errorf("config: tmp: %w", err)
		}
		tmpName := tmp.Name()
		// Гарантируем закрытие fd на любом пути выхода (и Remove temp-файла).
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}()
		if _, err := tmp.Write(data); err != nil {
			return fmt.Errorf("config: tmp write: %w", err)
		}
		// Закрываем fd до запуска sing-box check, чтобы тот мог открыть файл на чтение.
		if err := tmp.Close(); err != nil {
			return fmt.Errorf("config: tmp close: %w", err)
		}
		if _, err := c.runner.Run(ctx, c.singboxBin, "check", "-c", tmpName); err != nil {
			return fmt.Errorf("config: sing-box check failed: %w", err)
		}
	}
	return atomicfs.WriteFileAtomic(c.configPath, data, 0o644)
}

// portsManager реализует web.PortsManager поверх state.json.
type portsManager struct {
	statePath string
}

// NewPortsManager конструирует PortsManager.
func NewPortsManager(statePath string) web.PortsManager {
	return &portsManager{statePath: statePath}
}

func (m *portsManager) ListPorts(_ context.Context) ([]int, error) {
	s, err := Load(m.statePath)
	if err != nil {
		return nil, err
	}
	out := make([]int, len(s.Ports))
	for i, p := range s.Ports {
		out[i] = int(p)
	}
	sort.Ints(out)
	return out, nil
}

func (m *portsManager) AddPort(ctx context.Context, port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("ports: некорректный порт %d", port)
	}
	return withState(ctx, m.statePath, func(s *State) error {
		for _, p := range s.Ports {
			if int(p) == port {
				return nil // идемпотентно
			}
		}
		s.Ports = append(s.Ports, uint16(port))
		return nil
	})
}

func (m *portsManager) DeletePort(ctx context.Context, port int) error {
	return withState(ctx, m.statePath, func(s *State) error {
		out := s.Ports[:0]
		for _, p := range s.Ports {
			if int(p) != port {
				out = append(out, p)
			}
		}
		s.Ports = out
		return nil
	})
}

// excludesManager реализует web.ExcludesManager поверх state.json.
type excludesManager struct {
	statePath string
}

// NewExcludesManager конструирует ExcludesManager.
func NewExcludesManager(statePath string) web.ExcludesManager {
	return &excludesManager{statePath: statePath}
}

func (m *excludesManager) ListExcludes(_ context.Context) ([]string, error) {
	s, err := Load(m.statePath)
	if err != nil {
		return nil, err
	}
	out := append([]string{}, s.Excludes...)
	sort.Strings(out)
	return out, nil
}

func (m *excludesManager) AddExclude(ctx context.Context, cidr string) error {
	if _, err := netip.ParsePrefix(cidr); err != nil {
		// Принимаем также одиночные IP (преобразуем в /32 или /128).
		ip, errIP := netip.ParseAddr(cidr)
		if errIP != nil {
			return fmt.Errorf("excludes: некорректный CIDR/IP %q: %w", cidr, errors.Join(err, errIP))
		}
		bits := 32
		if ip.Is6() {
			bits = 128
		}
		cidr = fmt.Sprintf("%s/%d", ip.String(), bits)
	}
	return withState(ctx, m.statePath, func(s *State) error {
		for _, e := range s.Excludes {
			if e == cidr {
				return nil
			}
		}
		s.Excludes = append(s.Excludes, cidr)
		return nil
	})
}

func (m *excludesManager) DeleteExclude(ctx context.Context, cidr string) error {
	return withState(ctx, m.statePath, func(s *State) error {
		out := s.Excludes[:0]
		for _, e := range s.Excludes {
			if e != cidr {
				out = append(out, e)
			}
		}
		s.Excludes = out
		return nil
	})
}

// dpiTargetsManager реализует web.DPITargetsManager поверх state.json.
// Не регенерирует nfqws2.conf — конфиг подхватится при следующем --restart.
type dpiTargetsManager struct {
	statePath string
}

// NewDPITargetsManager конструирует DPITargetsManager.
func NewDPITargetsManager(statePath string) web.DPITargetsManager {
	return &dpiTargetsManager{statePath: statePath}
}

func (m *dpiTargetsManager) ListTargets(_ context.Context) ([]string, error) {
	s, err := Load(m.statePath)
	if err != nil {
		return nil, err
	}
	out := append([]string{}, s.DPITargets...)
	return out, nil
}

func (m *dpiTargetsManager) SetTargets(ctx context.Context, targets []string) error {
	clean := make([]string, 0, len(targets))
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		clean = append(clean, t)
	}
	return withState(ctx, m.statePath, func(s *State) error {
		s.DPITargets = clean
		return nil
	})
}

// ParsedExcludes возвращает Excludes как []netip.Prefix (для firewall.Config.Excludes).
func ParsedExcludes(s *State) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(s.Excludes))
	for _, e := range s.Excludes {
		p, err := netip.ParsePrefix(e)
		if err != nil {
			return nil, fmt.Errorf("excludes: %s: %w", e, err)
		}
		out = append(out, p)
	}
	return out, nil
}

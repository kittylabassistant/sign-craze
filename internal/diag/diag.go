package diag

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/locks"
	"github.com/kittylabassistant/sign-craze/internal/service"
	"github.com/kittylabassistant/sign-craze/internal/singbox"
)

// Status — итог одной проверки.
type Status string

const (
	PASS Status = "PASS"
	WARN Status = "WARN"
	FAIL Status = "FAIL"
)

// Result описывает результат одной проверки.
type Result struct {
	Name   string
	Status Status
	Detail string
}

// Check — функция-проверка. Не должна паниковать; возвращает Result со Status.
type Check func(ctx context.Context) Result

// Deps — зависимости для построения списка проверок.
type Deps struct {
	Runner        exectx.Runner
	SingboxLC     service.Lifecycle
	DPILC         service.Lifecycle
	SignCrazeBin  string // /opt/sbin/sign-craze
	SingboxBin    string // /opt/sbin/sing-box
	ConfigPath    string // /opt/etc/sign-craze/config.json
	GeoDir        string // /opt/var/lib/sign-craze/geo
	LockPath      string // /opt/var/lock/sign-craze.lock
	GeoMaxAgeDays int    // 7 — после превышения WARN
}

// DefaultDeps возвращает Deps со стандартными путями.
func DefaultDeps(runner exectx.Runner, sb, dpi service.Lifecycle) Deps {
	return Deps{
		Runner:        runner,
		SingboxLC:     sb,
		DPILC:         dpi,
		SignCrazeBin:  "/opt/sbin/sign-craze",
		SingboxBin:    singbox.DefaultBinPath,
		ConfigPath:    singbox.DefaultConfigDir + "/config.json",
		GeoDir:        "/opt/var/lib/sign-craze/geo",
		LockPath:      locks.DefaultPath,
		GeoMaxAgeDays: 7,
	}
}

// DefaultChecks возвращает стандартный набор проверок из BEHAVIOR_SPEC §1.--diag.
func DefaultChecks(d Deps) []Check {
	return []Check{
		checkBinaryExecutable("binary-self", d.SignCrazeBin),
		checkBinaryExecutable("binary-singbox", d.SingboxBin),
		checkConfigValid(d),
		checkCommandAvailable("iptables", d.Runner, "iptables", "--version"),
		checkCommandAvailable("ip6tables", d.Runner, "ip6tables", "--version"),
		checkCommandAvailable("ipset", d.Runner, "ipset", "--version"),
		checkDefaultRoute(d.Runner),
		checkServiceRunning("singbox", d.SingboxLC),
		checkServiceRunning("nfqws2", d.DPILC),
		checkGeoFiles(d.GeoDir, d.GeoMaxAgeDays),
		checkLockFree(d.LockPath),
	}
}

// Run выполняет все проверки последовательно и возвращает результаты.
func Run(ctx context.Context, checks []Check) []Result {
	out := make([]Result, 0, len(checks))
	for _, c := range checks {
		out = append(out, runSafe(ctx, c))
	}
	return out
}

func runSafe(ctx context.Context, c Check) (r Result) {
	defer func() {
		if rec := recover(); rec != nil {
			r = Result{Name: "unknown", Status: FAIL, Detail: fmt.Sprintf("panic: %v", rec)}
		}
	}()
	return c(ctx)
}

// AnyFail возвращает true если есть хотя бы один FAIL в результатах.
func AnyFail(results []Result) bool {
	for _, r := range results {
		if r.Status == FAIL {
			return true
		}
	}
	return false
}

// --- конкретные проверки ---

func checkBinaryExecutable(name, path string) Check {
	return func(_ context.Context) Result {
		info, err := os.Stat(path)
		if err != nil {
			return Result{Name: name, Status: FAIL, Detail: fmt.Sprintf("отсутствует: %v", err)}
		}
		if info.IsDir() {
			return Result{Name: name, Status: FAIL, Detail: "не файл"}
		}
		if info.Mode().Perm()&0o111 == 0 {
			return Result{Name: name, Status: FAIL, Detail: "не исполняемый"}
		}
		return Result{Name: name, Status: PASS, Detail: path}
	}
}

func checkConfigValid(d Deps) Check {
	return func(ctx context.Context) Result {
		if _, err := os.Stat(d.ConfigPath); err != nil {
			return Result{Name: "config-valid", Status: FAIL, Detail: fmt.Sprintf("конфиг отсутствует: %v", err)}
		}
		if _, err := os.Stat(d.SingboxBin); err != nil {
			return Result{Name: "config-valid", Status: WARN, Detail: "пропуск: sing-box не установлен"}
		}
		if d.Runner == nil {
			return Result{Name: "config-valid", Status: WARN, Detail: "пропуск: runner не задан"}
		}
		_, err := d.Runner.Run(ctx, d.SingboxBin, "check", "-c", d.ConfigPath)
		if err != nil {
			return Result{Name: "config-valid", Status: FAIL, Detail: err.Error()}
		}
		return Result{Name: "config-valid", Status: PASS, Detail: "sing-box check ok"}
	}
}

func checkCommandAvailable(name string, runner exectx.Runner, cmd string, args ...string) Check {
	return func(ctx context.Context) Result {
		if runner == nil {
			return Result{Name: name, Status: WARN, Detail: "пропуск: runner не задан"}
		}
		_, err := runner.Run(ctx, cmd, args...)
		if err != nil {
			return Result{Name: name, Status: FAIL, Detail: err.Error()}
		}
		return Result{Name: name, Status: PASS, Detail: "доступен"}
	}
}

func checkDefaultRoute(runner exectx.Runner) Check {
	return func(ctx context.Context) Result {
		if runner == nil {
			return Result{Name: "default-route", Status: WARN, Detail: "пропуск: runner не задан"}
		}
		res, err := runner.Run(ctx, "ip", "route", "show", "default")
		if err != nil {
			return Result{Name: "default-route", Status: FAIL, Detail: err.Error()}
		}
		if !strings.Contains(string(res.Stdout), "default ") {
			return Result{Name: "default-route", Status: FAIL, Detail: "маршрут по умолчанию не найден"}
		}
		return Result{Name: "default-route", Status: PASS, Detail: strings.TrimSpace(strings.SplitN(string(res.Stdout), "\n", 2)[0])}
	}
}

func checkServiceRunning(name string, lc service.Lifecycle) Check {
	return func(ctx context.Context) Result {
		st, err := lc.Status(ctx)
		if err != nil {
			return Result{Name: "service-" + name, Status: FAIL, Detail: err.Error()}
		}
		if !st.Running {
			return Result{Name: "service-" + name, Status: WARN, Detail: "не запущен"}
		}
		return Result{Name: "service-" + name, Status: PASS, Detail: fmt.Sprintf("pid %d", st.PID)}
	}
}

func checkGeoFiles(dir string, maxAgeDays int) Check {
	return func(_ context.Context) Result {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return Result{Name: "geo-files", Status: WARN, Detail: fmt.Sprintf("директория недоступна: %v", err)}
		}
		var srsCount int
		var newest time.Time
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".srs") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			srsCount++
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		if srsCount == 0 {
			return Result{Name: "geo-files", Status: WARN, Detail: "нет .srs файлов"}
		}
		age := time.Since(newest)
		if age > time.Duration(maxAgeDays)*24*time.Hour {
			return Result{Name: "geo-files", Status: WARN, Detail: fmt.Sprintf("%d файлов, последнее обновление %s назад", srsCount, age.Round(time.Hour))}
		}
		return Result{Name: "geo-files", Status: PASS, Detail: fmt.Sprintf("%d файлов, актуально", srsCount)}
	}
}

func checkLockFree(path string) Check {
	return func(ctx context.Context) Result {
		// Если файл блокировки отсутствует — никто не держит, PASS.
		if _, err := os.Stat(path); err != nil {
			return Result{Name: "lock-free", Status: PASS, Detail: "блокировка не существует"}
		}
		// Попробовать получить блокировку с очень коротким таймаутом.
		quickCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		l, err := locks.Acquire(quickCtx, path)
		if err != nil {
			return Result{Name: "lock-free", Status: FAIL, Detail: "удерживается другим процессом"}
		}
		if relErr := l.Release(); relErr != nil {
			return Result{Name: "lock-free", Status: WARN, Detail: fmt.Sprintf("Release: %v", relErr)}
		}
		return Result{Name: "lock-free", Status: PASS, Detail: "свободна"}
	}
}

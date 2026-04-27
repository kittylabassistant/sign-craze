package version

import (
	"runtime/debug"
	"strings"
	"time"
)

// Version — версия sign-craze.
// В dev-сборке равна "dev". В релизе перезаписывается через:
//
//	go build -ldflags="-X github.com/kittylabassistant/sign-craze/internal/version.Version=<tag>"
var Version = "dev"

// BuildInfo содержит метаданные сборки, извлечённые через runtime/debug.
type BuildInfo struct {
	GoVersion string
	Commit    string
	BuildTime string
	Dirty     bool
}

// Get возвращает информацию о сборке.
// В dev-сборке без VCS-тегов поля Commit/BuildTime могут быть пустыми.
func Get() BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return BuildInfo{GoVersion: "unknown"}
	}

	bi := BuildInfo{GoVersion: info.GoVersion}

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) >= 7 {
				bi.Commit = s.Value[:7]
			} else {
				bi.Commit = s.Value
			}
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				bi.BuildTime = t.Format("2006-01-02")
			}
		case "vcs.modified":
			bi.Dirty = s.Value == "true"
		}
	}

	return bi
}

// String возвращает строку вида "v0.1.0 (коммит abc1234, 2026-04-27)".
func String() string {
	bi := Get()
	sb := new(strings.Builder)
	sb.WriteString("v")
	sb.WriteString(Version)
	if bi.Commit != "" {
		sb.WriteString(" (коммит ")
		sb.WriteString(bi.Commit)
		if bi.Dirty {
			sb.WriteString("-dirty")
		}
		if bi.BuildTime != "" {
			sb.WriteString(", ")
			sb.WriteString(bi.BuildTime)
		}
		sb.WriteString(")")
	}
	return sb.String()
}

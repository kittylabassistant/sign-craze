package version

import (
	_ "embed"
	"runtime/debug"
	"strings"
	"time"
)

//go:embed ../../VERSION
var raw string

// Version — версия sign-craze из файла VERSION в корне репозитория.
// Ведущие/завершающие пробелы и переносы строк обрезаются.
var Version = strings.TrimSpace(raw)

// BuildInfo содержит метаданные сборки, извлечённые через runtime/debug.
type BuildInfo struct {
	GoVersion string
	Commit    string
	BuildTime string
	Dirty     bool
}

// Get возвращает информацию о сборке.
// При сборке через `go build` поля могут быть пустыми — это ожидаемое поведение в dev-среде.
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

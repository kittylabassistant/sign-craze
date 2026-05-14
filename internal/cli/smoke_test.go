package cli

import (
	"context"
	"strings"
	"testing"
)

// expectedCommands — все Long-флаги, которые должны быть зарегистрированы.
var expectedCommands = []string{
	"--ui", "--ui-daemon",
	"--install", "--install-auto", "--install-offline", "--reinstall",
	"--start", "--stop", "--restart", "--service-start",
	"--status", "--version",
	"--diag",
	"--update", "--update-geo", "--update-core", "--update-naive",
	"--uninstall",
	"--dpi", "--dpi-strategy", "--dpi-update",
	"--mode",
	"--port-add", "--port-del", "--port-list",
	"--exclude-add", "--exclude-del", "--exclude-list",
	"--backup", "--restore",
	"--reapply",
}

func TestRegistry_AllCommandsRegistered(t *testing.T) {
	registered := make(map[string]bool, len(registry))
	for _, c := range registry {
		registered[c.Long] = true
	}
	for _, want := range expectedCommands {
		if !registered[want] {
			t.Errorf("команда %q не зарегистрирована", want)
		}
	}
}

func TestRegistry_NoDuplicateLongFlags(t *testing.T) {
	seen := make(map[string]int, len(registry))
	for _, c := range registry {
		seen[c.Long]++
	}
	for k, v := range seen {
		if v > 1 {
			t.Errorf("дублирующая регистрация %q (%d раз)", k, v)
		}
	}
}

func TestDispatch_HelpDoesNotPanic(t *testing.T) {
	if err := Dispatch(context.Background(), []string{"--help"}); err != nil {
		t.Errorf("--help: %v", err)
	}
	if err := Dispatch(context.Background(), nil); err != nil {
		t.Errorf("пустой args: %v", err)
	}
}

func TestDispatch_UnknownCommand_Smoke(t *testing.T) {
	err := Dispatch(context.Background(), []string{"--no-such-thing"})
	if err == nil || !strings.Contains(err.Error(), "неизвестная команда") {
		t.Errorf("ожидалась ошибка про неизвестную команду, получено %v", err)
	}
}

func TestServiceStartIsHidden(t *testing.T) {
	for _, c := range registry {
		if c.Long == "--service-start" && !c.Hidden {
			t.Error("--service-start должна быть Hidden")
		}
	}
}

func TestReapplyIsHidden(t *testing.T) {
	for _, c := range registry {
		if c.Long == "--reapply" && !c.Hidden {
			t.Error("--reapply должна быть Hidden (вызывается из netfilter.d hook)")
		}
	}
}

func TestUIDaemonIsHidden(t *testing.T) {
	for _, c := range registry {
		if c.Long == "--ui-daemon" && !c.Hidden {
			t.Error("--ui-daemon должна быть Hidden (внутренний форк из --ui on)")
		}
	}
}

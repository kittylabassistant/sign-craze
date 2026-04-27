package service

import "os/exec"

// buildCmd создаёт *exec.Cmd без привязки к context — процесс должен пережить родителя.
func buildCmd(binPath string, args ...string) *exec.Cmd {
	return exec.Command(binPath, args...) //nolint:noctx // дочерний процесс должен пережить родителя
}

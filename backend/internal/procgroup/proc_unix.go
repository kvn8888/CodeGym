//go:build !windows

// Package procgroup runs a child process together with the descendants it
// spawns, and tears the whole tree down. It is the single place where the
// platform-specific process-tree behavior lives so callers stay portable:
// stage runners on every platform set up and kill trees identically.
package procgroup

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// Setup places cmd in its own process group so Kill can signal the entire
// tree — the shell and every descendant it spawned — rather than only the
// direct child, which would otherwise outlive a cancelled stage.
func Setup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// Kill terminates cmd's whole process group. It reports os.ErrProcessDone
// when the group is already gone, so a race with normal exit is not treated
// as a teardown failure, and passes any other error through unchanged.
func Kill(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

// Alive reports whether a pid still exists. Only "no such process" counts as
// dead: a permission error still proves the process is there.
func Alive(pid int) bool {
	return !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

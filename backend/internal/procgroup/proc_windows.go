//go:build windows

package procgroup

import (
	"os"
	"os/exec"
	"strconv"
)

// Setup needs no work on Windows: there is no process group to create, and
// Kill already reaches the whole child tree.
func Setup(cmd *exec.Cmd) {}

// Kill terminates cmd and every descendant it spawned. Windows has no
// signal-based group kill, so this shells out to taskkill's tree walk. A
// failure because the target is already gone is reported as
// os.ErrProcessDone so a race with normal exit is not a teardown failure,
// while a genuine failure (for example access denied) is passed through.
func Kill(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	pid := cmd.Process.Pid
	if err := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run(); err != nil {
		if !Alive(pid) {
			return os.ErrProcessDone
		}
		return err
	}
	return nil
}

// Alive reports whether a pid still exists. On Windows a process is alive
// exactly while its handle can still be opened.
func Alive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}

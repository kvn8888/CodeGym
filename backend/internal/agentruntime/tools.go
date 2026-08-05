package agentruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func readBounded(root *os.Root, path string, capBytes int) (string, bool, error) {
	path, err := confinedPath(path)
	if err != nil {
		return "", false, err
	}
	file, err := root.Open(path)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = file.Close() }()
	payload, err := io.ReadAll(io.LimitReader(file, int64(capBytes)+1))
	if err != nil {
		return "", false, err
	}
	if len(payload) > capBytes {
		return string(payload[:capBytes]), true, nil
	}
	return string(payload), false, nil
}

func writeBounded(root *os.Root, path string, content []byte) error {
	path, err := confinedPath(path)
	if err != nil {
		return err
	}
	parent := filepath.Dir(path)
	if parent != "." {
		if err := root.MkdirAll(parent, 0o700); err != nil {
			return err
		}
	}
	return root.WriteFile(path, content, 0o600)
}

func confinedPath(path string) (string, error) {
	path = filepath.FromSlash(strings.TrimSpace(path))
	if path == "" || filepath.IsAbs(path) || escapesRoot(path) {
		return "", fmt.Errorf("path %q escapes the working directory", path)
	}
	return filepath.Clean(path), nil
}

func runShell(ctx context.Context, workingDirectory, command string, environment []string, capBytes int) (string, int, bool, error) {
	if strings.TrimSpace(command) == "" {
		return "", -1, false, errors.New("shell command is required")
	}
	commandContext, cancel := context.WithCancel(ctx)
	defer cancel()
	buffer := newCappedBuffer(capBytes, cancel)
	cmd := exec.CommandContext(commandContext, "sh", "-c", command)
	cmd.Dir = workingDirectory
	cmd.Env = append([]string(nil), environment...)
	cmd.Stdout = buffer
	cmd.Stderr = buffer
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
			if ctx.Err() == nil && !buffer.Truncated() {
				err = nil
			}
		}
	}
	if buffer.Truncated() {
		return buffer.String(), exitCode, true, errors.New("shell output exceeded output ceiling")
	}
	if ctx.Err() != nil {
		return buffer.String(), exitCode, false, ctx.Err()
	}
	return buffer.String(), exitCode, false, err
}

func safeRuntimeEnvironment(home, tempRoot string) []string {
	environment := []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
		"TMPDIR=" + tempRoot,
		"CI=true",
	}
	for _, name := range []string{"GOCACHE", "GOMODCACHE", "GOPATH", "JAVA_HOME"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

type cappedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	remaining int
	truncated bool
	cancel    context.CancelFunc
}

func newCappedBuffer(limit int, cancel context.CancelFunc) *cappedBuffer {
	return &cappedBuffer{remaining: limit, cancel: cancel}
}

func (b *cappedBuffer) Write(payload []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(payload)
	overflow := len(payload) > b.remaining
	if b.remaining > 0 {
		keep := len(payload)
		if keep > b.remaining {
			keep = b.remaining
		}
		_, _ = b.buffer.Write(payload[:keep])
		b.remaining -= keep
	}
	if overflow && !b.truncated {
		b.truncated = true
		b.cancel()
	}
	return written, nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *cappedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

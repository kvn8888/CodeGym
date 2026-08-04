package execution

import (
	"fmt"
	"time"
)

const (
	GoToolchainVersion = "1.25.4"
	GoSnapshotName     = "codegym-go-1-25-4-v1"
)

// Language describes how to execute a submission for one runtime.
type Language struct {
	Name string
	// ChildCommand builds argv for the supervisor to execute without a shell.
	ChildCommand func(entrypoint string) []string
	// RunCommand builds the shell command executed inside the sandbox.
	// Kept for compatibility with callers outside DaytonaRunner.
	RunCommand func(entrypoint string) string
	// Snapshot names the Daytona snapshot to boot from; empty means the
	// platform default. This is the extension point for the hybrid flow:
	// runtimes scaffolded by the agent track get promoted to snapshots
	// (spikes/daytona Phase C) and registered here for deterministic reuse.
	Snapshot string
	// ExecTimeout bounds the run command itself; sandbox lifecycle overhead
	// is budgeted separately by the service.
	ExecTimeout time.Duration
}

var languages = map[string]Language{
	"python": {
		Name: "python",
		ChildCommand: func(entrypoint string) []string {
			return []string{"python3", entrypoint}
		},
		RunCommand: func(entrypoint string) string {
			return fmt.Sprintf("cd ~/work && python3 %s", entrypoint)
		},
		ExecTimeout: 30 * time.Second,
	},
	"go": {
		Name: "go",
		// The Go hidden entrypoint is a Python compile-and-run wrapper. Python
		// is part of the Daytona base image and remains outside user control.
		ChildCommand: func(entrypoint string) []string {
			return []string{"python3", entrypoint}
		},
		RunCommand: func(entrypoint string) string {
			return fmt.Sprintf("cd ~/work && python3 %s", entrypoint)
		},
		Snapshot:    GoSnapshotName,
		ExecTimeout: 30 * time.Second,
	},
}

// LanguageFor resolves a language by name.
func LanguageFor(name string) (Language, bool) {
	lang, ok := languages[name]
	return lang, ok
}

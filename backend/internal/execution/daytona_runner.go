package execution

// DaytonaRunner is the production Runner backed by Daytona sandboxes. It is
// the ONLY file in the backend that imports the Daytona SDK — everything
// else talks to the Runner interface.
//
// ── IMPLEMENT ME ─────────────────────────────────────────────────────────
// This is intentionally left as a scaffold. The working reference for every
// step is the validated spike at spikes/daytona/main.go (phaseA is the exact
// recipe; phaseC shows booting from a named snapshot). Your finish line is
// the gated integration suite in daytona_runner_test.go:
//
//	doppler run -p codegym -c dev -- go test ./internal/execution -run TestDaytonaRunner -count=1
//
// ─────────────────────────────────────────────────────────────────────────

import (
	"context"
	"errors"

	"github.com/daytona/clients/sdk-go/pkg/daytona"
)

var errNotImplemented = errors.New("daytona runner not implemented yet — see TODOs in daytona_runner.go and spikes/daytona/main.go")

type DaytonaRunner struct {
	client *daytona.Client
}

// NewDaytonaRunner builds a runner from explicit credentials (loaded from
// config, not raw env, so tests and main stay explicit).
func NewDaytonaRunner(apiKey, apiURL string) (*DaytonaRunner, error) {
	// TODO(kevin) 1: construct the SDK client with
	//   daytona.NewClientWithConfig(&types.DaytonaConfig{APIKey: apiKey, APIUrl: apiURL})
	// (import "github.com/daytona/clients/sdk-go/pkg/types"). Client lifetime:
	// it is fine to hold one client for the server's lifetime and never Close
	// it in v0 — note that client.Close(ctx) exists if we later add graceful
	// shutdown.
	_ = apiKey
	_ = apiURL
	return &DaytonaRunner{}, nil
}

// Run executes one submission in an ephemeral, fully network-blocked sandbox
// and reports the outcome. Remember the contract from runner.go: a nonzero
// exit code is a NORMAL result (return it in RunOutcome with a nil error);
// a returned error means the infrastructure itself failed.
func (r *DaytonaRunner) Run(ctx context.Context, spec RunSpec) (RunOutcome, error) {
	// TODO(kevin) 2: start timing here (time.Now) — RunOutcome.Duration is
	// the whole sandbox-side wall time, create through exec.

	// TODO(kevin) 3: create the sandbox (spike A1 / C2):
	//   r.client.Create(ctx, types.SnapshotParams{
	//       Snapshot: spec.Language.Snapshot, // "" = platform default
	//       SandboxBaseParams: types.SandboxBaseParams{
	//           Labels:          map[string]string{"codegym": "submission"},
	//           NetworkBlockAll: true,
	//           Ephemeral:       true,
	//       },
	//   })
	// Then `defer sb.Delete(ctx)` — Ephemeral auto-deletes on stop, but the
	// eager delete keeps billing tight and is what the spike does.

	// TODO(kevin) 4: upload the files (spike A2):
	//   sb.FileSystem.CreateFolder(ctx, "work")
	//   for each spec.Files: sb.FileSystem.UploadFile(ctx, []byte(f.Content), "work/"+f.Path)
	// GOTCHA: FileSystem paths are HOME-RELATIVE. No leading "/" and no "~/"
	// — absolute paths outside $HOME are rejected with a 400. (The service
	// already validated spec.Files paths as safe relative paths.)

	// TODO(kevin) 5: execute (spike A3):
	//   sb.Process.ExecuteCommand(ctx, spec.Language.RunCommand(spec.Entrypoint),
	//       options.WithExecuteTimeout(spec.Language.ExecTimeout))
	// (import "github.com/daytona/clients/sdk-go/pkg/options").
	// GOTCHA: unlike FileSystem paths, the shell command DOES address files
	// via ~/work — RunCommand already emits "cd ~/work && ...".

	// TODO(kevin) 6: map the response into RunOutcome:
	//   res.ExitCode → ExitCode (nonzero is a result, NOT an error)
	//   res.Result   → Output (combined stdout+stderr)
	//   time.Since(start) → Duration
	// Only return a non-nil error for SDK/sandbox failures (create, upload,
	// exec transport errors, ctx deadline).

	_ = ctx
	_ = spec
	return RunOutcome{}, errNotImplemented
}

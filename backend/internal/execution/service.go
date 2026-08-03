package execution

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

// ErrRunnerUnavailable is returned by SubmitRun when no runner is configured
// (DAYTONA_API_KEY unset); read endpoints keep working without one.
var ErrRunnerUnavailable = errors.New("execution runner not configured")

const (
	maxFiles          = 16
	maxTotalFileBytes = 256 * 1024
	// runnerOverhead budgets sandbox lifecycle (create, upload, delete) on
	// top of the language's own execution timeout.
	runnerOverhead = 60 * time.Second
)

type Clock func() time.Time

type Service struct {
	store  Store
	runner Runner
	now    Clock
}

// NewService builds the execution service. runner may be nil, which disables
// SubmitRun while leaving reads available.
func NewService(store Store, runner Runner, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: store, runner: runner, now: clock}
}

// SubmitRun validates the submission, persists it through its lifecycle
// (queued → running → terminal), and executes it synchronously.
func (s *Service) SubmitRun(ctx context.Context, input SubmitRunInput) (Run, error) {
	identity, err := identityFromContext(ctx)
	if err != nil {
		return Run{}, err
	}
	if s.runner == nil {
		return Run{}, ErrRunnerUnavailable
	}

	lang, ok := LanguageFor(strings.TrimSpace(input.Language))
	if !ok {
		return Run{}, fmt.Errorf("unsupported language %q", input.Language)
	}
	if err := validateFiles(input.Files, input.Entrypoint); err != nil {
		return Run{}, err
	}

	run := Run{
		ID:          newID("exec_run"),
		WorkspaceID: identity.workspaceID,
		UserID:      identity.userID,
		ProblemID:   strings.TrimSpace(input.ProblemID),
		Language:    lang.Name,
		Entrypoint:  input.Entrypoint,
		Files:       input.Files,
		Status:      StatusQueued,
		CreatedAt:   s.now().UTC(),
	}
	if err := s.store.CreateRun(ctx, run); err != nil {
		return Run{}, err
	}

	run.Status = StatusRunning
	if err := s.store.UpdateRun(ctx, run); err != nil {
		return Run{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, lang.ExecTimeout+runnerOverhead)
	defer cancel()

	outcome, runErr := s.runner.Run(runCtx, RunSpec{
		Language:   lang,
		Files:      input.Files,
		Entrypoint: input.Entrypoint,
	})

	completed := s.now().UTC()
	run.CompletedAt = &completed
	if runErr != nil {
		run.Status = StatusError
		run.Error = runErr.Error()
	} else {
		run.Output = outcome.Output
		run.DurationMs = outcome.Duration.Milliseconds()
		if outcome.Result.Schema == JudgeSchema {
			run.ExitCode = outcome.Result.ExitCode
			run.DurationMs = outcome.Result.DurationMs
			if outcome.Result.Status == JudgeStatusPassed {
				run.Status = StatusPassed
			} else {
				run.Status = StatusFailed
			}
		} else {
			exitCode := outcome.ExitCode
			run.ExitCode = &exitCode
			if exitCode == 0 {
				run.Status = StatusPassed
			} else {
				run.Status = StatusFailed
			}
		}
	}

	// A failed terminal update loses the outcome (the sandbox is already
	// gone); acceptable in v0 since the caller sees the error.
	if err := s.store.UpdateRun(ctx, run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *Service) GetRun(ctx context.Context, id string) (Run, error) {
	identity, err := identityFromContext(ctx)
	if err != nil {
		return Run{}, err
	}
	return s.store.GetRun(ctx, identity.workspaceID, identity.userID, id)
}

func (s *Service) ListRuns(ctx context.Context) ([]Run, error) {
	identity, err := identityFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return s.store.ListRuns(ctx, identity.workspaceID, identity.userID)
}

// validateFiles enforces that paths are safe to become home-relative sandbox
// uploads: relative, normalized, and confined to the work directory.
func validateFiles(files []File, entrypoint string) error {
	if len(files) == 0 {
		return errors.New("at least one file is required")
	}
	if len(files) > maxFiles {
		return fmt.Errorf("at most %d files are allowed", maxFiles)
	}

	totalBytes := 0
	entrypointFound := false
	seen := map[string]bool{}
	for _, f := range files {
		if err := validatePath(f.Path); err != nil {
			return err
		}
		if seen[f.Path] {
			return fmt.Errorf("duplicate file path %q", f.Path)
		}
		seen[f.Path] = true
		totalBytes += len(f.Content)
		if f.Path == entrypoint {
			entrypointFound = true
		}
	}
	if totalBytes > maxTotalFileBytes {
		return fmt.Errorf("total file size exceeds %d bytes", maxTotalFileBytes)
	}
	if strings.TrimSpace(entrypoint) == "" {
		return errors.New("entrypoint is required")
	}
	if !entrypointFound {
		return fmt.Errorf("entrypoint %q must match one of the submitted file paths", entrypoint)
	}
	return nil
}

func validatePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("file path is required")
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~") {
		return fmt.Errorf("file path %q must be relative", path)
	}
	parts := strings.Split(path, "/")
	if parts[0] == ".codegym" {
		return fmt.Errorf("file path %q uses the reserved judge protocol directory", path)
	}
	for _, part := range parts {
		if part == ".." || part == "." || part == "" {
			return fmt.Errorf("file path %q must not contain empty, '.' or '..' segments", path)
		}
	}
	return nil
}

type identity struct {
	workspaceID string
	userID      string
}

func identityFromContext(ctx context.Context) (identity, error) {
	principal, ok := auth.PrincipalFromContext(ctx)
	if !ok || principal.UserID == "" {
		return identity{}, errors.New("missing authenticated user")
	}

	scope, ok := workspace.ScopeFromContext(ctx)
	if !ok || scope.WorkspaceID == "" {
		return identity{}, errors.New("missing workspace scope")
	}

	return identity{workspaceID: scope.WorkspaceID, userID: principal.UserID}, nil
}

func newID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}

package execution

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
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
	if err := validateLimits(input.Limits); err != nil {
		return Run{}, err
	}

	lang, ok := LanguageFor(strings.TrimSpace(input.Language))
	if !ok {
		return Run{}, fmt.Errorf("unsupported language %q", input.Language)
	}
	if err := validateFiles(input.Files, input.Entrypoint); err != nil {
		return Run{}, err
	}
	strategy, err := normalizeTestStrategy(input.Strategy)
	if err != nil {
		return Run{}, err
	}
	mode, err := normalizeSubmissionMode(input.Mode)
	if err != nil {
		return Run{}, err
	}

	run := Run{
		ID:          newID("exec_run"),
		WorkspaceID: identity.workspaceID,
		UserID:      identity.userID,
		ProblemID:   strings.TrimSpace(input.ProblemID),
		Language:    lang.Name,
		Entrypoint:  input.Entrypoint,
		Mode:        mode,
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

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(input.Limits.TimeoutSeconds)*time.Second+runnerOverhead)
	defer cancel()

	outcome, runErr := s.runner.Run(runCtx, RunSpec{
		Language:   lang,
		Files:      input.Files,
		Entrypoint: input.Entrypoint,
		Strategy:   strategy,
		Limits:     input.Limits,
	})
	s.recordRunTimingBestEffort(ctx, run, lang, strategy, outcome.StageDurations)

	completed := s.now().UTC()
	run.CompletedAt = &completed
	if runErr != nil {
		run.Status = StatusError
		if errors.Is(runErr, ErrInfrastructureBusy) {
			run.Error = ErrInfrastructureBusy.Error()
		} else {
			run.Error = runErr.Error()
		}
	} else {
		run.Output = outcome.Output
		run.DurationMs = outcome.Duration.Milliseconds()
		if outcome.Result.Schema == JudgeSchema {
			judgeResult := outcome.Result
			run.JudgeResult = &judgeResult
			run.ExitCode = outcome.Result.ExitCode
			run.DurationMs = outcome.Result.DurationMs
			switch outcome.Result.Status {
			case JudgeStatusPassed:
				run.Status = StatusPassed
			case JudgeStatusFailed:
				run.Status = StatusFailed
			case JudgeStatusTimeout:
				run.Status = StatusTimeout
			case JudgeStatusOutOfMemory:
				run.Status = StatusOutOfMemory
			case JudgeStatusCrashed:
				run.Status = StatusCrashed
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

func normalizeSubmissionMode(value string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == "" {
		mode = "submit"
	}
	if mode != "run" && mode != "submit" {
		return "", fmt.Errorf("submission mode must be run or submit, got %q", value)
	}
	return mode, nil
}

func normalizeTestStrategy(value string) (TestStrategy, error) {
	strategy := TestStrategy(strings.ToLower(strings.TrimSpace(value)))
	if strategy == "" {
		strategy = TestStrategyUnit
	}
	switch strategy {
	case TestStrategyUnit, TestStrategyHTTP:
		return strategy, nil
	default:
		return "", fmt.Errorf("test strategy must be unit or http, got %q", value)
	}
}

func validateLimits(limits Limits) error {
	if limits.TimeoutSeconds <= 0 {
		return errors.New("limits.timeout_seconds must be positive")
	}
	if limits.MemoryMB <= 0 {
		return errors.New("limits.memory_mb must be positive")
	}
	if limits.NetworkMode != NetworkModeBlockAll {
		return fmt.Errorf("limits.network_mode must be %q", NetworkModeBlockAll)
	}
	return nil
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

// ListRunTimings returns the caller's structured execution-stage observations.
func (s *Service) ListRunTimings(ctx context.Context) ([]RunTiming, error) {
	identity, err := identityFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return s.store.ListRunTimings(ctx, identity.workspaceID, identity.userID)
}

func (s *Service) recordRunTimingBestEffort(
	ctx context.Context,
	run Run,
	language Language,
	strategy TestStrategy,
	stages StageDurations,
) {
	timing := RunTiming{
		ID:            newID("exec_timing"),
		RunID:         run.ID,
		WorkspaceID:   run.WorkspaceID,
		UserID:        run.UserID,
		Snapshot:      language.Snapshot,
		Language:      language.Name,
		Strategy:      strategy,
		CreateRetried: stages.CreateRetried,
		CreateMs:      stages.CreateMs,
		UploadMs:      stages.UploadMs,
		ExecMs:        stages.ExecMs,
		TotalMs:       stages.TotalMs,
		CreatedAt:     s.now().UTC(),
	}
	if err := s.store.AppendRunTiming(ctx, timing); err != nil {
		// Observability must never replace a valid learner verdict with a
		// platform error. Keep the failure loud and let the run finish.
		log.Printf("execution timing record failed run_id=%s workspace_id=%s language=%s err=%v",
			run.ID, run.WorkspaceID, language.Name, err)
	}
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

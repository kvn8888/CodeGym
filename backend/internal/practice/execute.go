package practice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/execution"
)

// Category is the safe public outcome of a practice execution. Values
// distinguish learner-visible check outcomes from backend faults without
// leaking hidden tests, reference code, paths, or provider internals.
type Category string

// Practice result categories.
const (
	// CategoryPassed reports every executed command succeeding.
	CategoryPassed Category = "passed"
	// CategoryBuildError reports a non-zero build command exit.
	CategoryBuildError Category = "build-error"
	// CategoryTestFailure reports a non-zero test (or run) command exit.
	CategoryTestFailure Category = "test-failure"
	// CategoryMalformedManifest reports the session manifest failing
	// structural validation at use time.
	CategoryMalformedManifest Category = "malformed-manifest"
	// CategoryValidationFailed reports the session overlay failing
	// backend file validation at use time.
	CategoryValidationFailed Category = "validation-failed"
	// CategoryTimeout reports a provider timeout or an expired deadline.
	CategoryTimeout Category = "timeout"
	// CategoryPlatformFault reports a provider or infrastructure failure.
	CategoryPlatformFault Category = "platform-fault"
)

// ExecutionStatus maps a practice category onto the existing execution
// status vocabulary, so record-adjacent consumers (logs, #191 views) can
// reuse established values. Identity does not map: execution records
// carry no artifact fields, which is the documented #184 addition.
func (c Category) ExecutionStatus() execution.Status {
	switch c {
	case CategoryPassed:
		return execution.StatusPassed
	case CategoryBuildError, CategoryTestFailure:
		return execution.StatusFailed
	case CategoryTimeout:
		return execution.StatusTimeout
	default:
		return execution.StatusError
	}
}

// StepSummary reports one executed stage by name and exit code. Command
// strings are deliberately absent: commands name protected files, so
// echoing them would leak hidden test names.
type StepSummary struct {
	Stage    string `json:"stage"`
	ExitCode int    `json:"exit_code"`
}

// Result is the safe public projection of Run or Submit. Output carries
// learner program output from run steps only. Build failures carry the
// category and fixed summary with stage and exit — no build output
// lines are forwarded, because a filename mention cannot authorize
// arbitrary trailing text (an editable filename prefixed to encoded
// hidden content would otherwise survive). Test-step output is never
// retained, because hidden assertions, expected answers, and protected
// source cannot be projected safely from raw text. Per-line build
// diagnostics await a structured provider contract (see doc.go).
// Identity fields associate the run with its verified artifact without
// trusting learner claims. Simulated marks development/testing-only
// sample execution: clients must label it explicitly and never present a
// simulated "passed" as verification evidence.
type Result struct {
	Category        Category      `json:"category"`
	Summary         string        `json:"summary"`
	Output          string        `json:"output"`
	OutputTruncated bool          `json:"output_truncated"`
	Steps           []StepSummary `json:"steps"`
	Simulated       bool          `json:"simulated"`

	SessionID       string `json:"session_id"`
	ArtifactID      string `json:"artifact_id"`
	ArtifactVersion string `json:"artifact_version"`
	Technology      string `json:"technology"`
	EnvironmentBase string `json:"environment_base"`
	NetworkPolicy   string `json:"network_policy"`
	// RecordID links to the persisted execution record when record
	// storage is configured and the save succeeded; empty otherwise.
	RecordID string `json:"record_id,omitempty"`
}

// Run executes the manifest's optional run command when declared;
// otherwise it executes the test command as the documented smoke check.
// Exactly one command runs; setup (dependency installation) never runs.
func (s *Service) Run(ctx context.Context, sessionID, owner string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	snap, err := s.snapshot(sessionID, owner)
	if err != nil {
		return Result{}, err
	}
	var stage, command string
	if snap.manifest.Commands != nil {
		command = strings.TrimSpace(snap.manifest.Commands.Run)
	}
	if command != "" {
		stage = environmentverify.StageRun
	} else if snap.manifest.Commands != nil {
		command = strings.TrimSpace(snap.manifest.Commands.Test)
		stage = environmentverify.StageTest
	}
	result, err := s.execute(ctx, snap, []stageCommand{{stage: stage, command: command}})
	if err != nil {
		return Result{}, err
	}
	s.storeResult(ctx, owner, &result)
	return result, nil
}

// Submit executes the manifest build command and then the test command
// within one isolated execution workspace, stopping at the first
// failure. Build outputs persist for the test step through the shared
// execution workspace. Setup never runs: dependencies are already
// available in the verified artifact.
func (s *Service) Submit(ctx context.Context, sessionID, owner string) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	snap, err := s.snapshot(sessionID, owner)
	if err != nil {
		return Result{}, err
	}
	var build, test string
	if snap.manifest.Commands != nil {
		build = strings.TrimSpace(snap.manifest.Commands.Build)
		test = strings.TrimSpace(snap.manifest.Commands.Test)
	}
	result, err := s.execute(ctx, snap, []stageCommand{
		{stage: environmentverify.StageBuild, command: build},
		{stage: environmentverify.StageTest, command: test},
	})
	if err != nil {
		return Result{}, err
	}
	s.storeResult(ctx, owner, &result)
	return result, nil
}

type stageCommand struct {
	stage   string
	command string
}

// sessionSnapshot is the authoritative execution view copied out of the
// session store. Files are re-backed so later writes cannot alter an
// in-flight operation; the manifest and redaction tokens are read-only
// after open and safely shared.
type sessionSnapshot struct {
	id              string
	artifactID      string
	artifactVersion string
	technology      string
	environmentBase string
	networkPolicy   string
	snapshot        string
	manifest        environmentverify.Manifest
	files           map[string]string
	redact          []string
}

// snapshot resolves one session for the requesting owner and copies the
// execution view out from under the service lock. Provider calls run
// without the lock held.
func (s *Service) snapshot(sessionID, owner string) (sessionSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.require(sessionID, owner)
	if err != nil {
		return sessionSnapshot{}, err
	}
	files := make(map[string]string, len(state.files))
	for name, content := range state.files {
		files[name] = content
	}
	return sessionSnapshot{
		id:              state.id,
		artifactID:      state.artifactID,
		artifactVersion: state.artifactVersion,
		technology:      state.technology,
		environmentBase: state.environmentBase,
		networkPolicy:   state.networkPolicy,
		snapshot:        state.snapshot,
		manifest:        state.manifest,
		files:           files,
		redact:          state.redact,
	}, nil
}

// execute runs trusted manifest commands in order against the isolated
// session overlay, short-circuiting at the first failure. Only manifest
// commands run; the learner overlay travels as data. Cancellation after a
// provider success supersedes it: dead contexts never advance outcomes.
func (s *Service) execute(ctx context.Context, snap sessionSnapshot, steps []stageCommand) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	base := Result{
		SessionID:       snap.id,
		ArtifactID:      snap.artifactID,
		ArtifactVersion: snap.artifactVersion,
		Technology:      snap.technology,
		EnvironmentBase: snap.environmentBase,
		NetworkPolicy:   snap.networkPolicy,
		Simulated:       s.Simulated,
	}
	if strings.TrimSpace(snap.id) == "" {
		return Result{}, errors.New("practice execute requires a session")
	}
	if s.Runner == nil {
		return Result{}, errors.New("practice runner is not configured")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("practice execute: %w", err)
	}
	if err := environmentverify.Validate(snap.manifest); err != nil {
		base.Category = CategoryMalformedManifest
		base.Summary = "The verified artifact manifest is malformed."
		return base, nil
	}
	if err := validateSessionFiles(snap.manifest.Workspace.LearnerEditable, snap.files); err != nil {
		base.Category = CategoryValidationFailed
		base.Summary = "The learner workspace failed backend validation."
		return base, nil
	}
	for _, step := range steps {
		if strings.TrimSpace(step.command) == "" {
			base.Category = CategoryValidationFailed
			base.Summary = "The verified artifact declares no command for this operation."
			return base, nil
		}
	}
	executionID := newExecutionID()
	files := overlayFiles(snap.files)
	protected := protectedPaths(snap.manifest)
	// runOutput carries the latest run-step program output; test steps
	// contribute nothing, so runs ending on test output retain none.
	runOutput := ""
	for _, step := range steps {
		stageCtx, cancel := context.WithTimeout(ctx, s.commandTimeout())
		outcome, err := s.Runner.RunCommand(stageCtx, RunSpec{
			ExecutionID:   executionID,
			SnapshotRef:   snap.snapshot,
			Stage:         step.stage,
			Command:       step.command,
			WorkDir:       workspaceRoot(snap.manifest),
			Files:         files,
			NetworkPolicy: snap.networkPolicy,
			Visibility:    scopeForStage(step.stage),
			Protected:     protected,
			Limits: execution.Limits{
				TimeoutSeconds: int(s.commandTimeout().Seconds()),
				MemoryMB:       s.memoryMB(),
				NetworkMode:    snap.networkPolicy,
			},
			MaxOutputBytes: s.maxOutputBytes(),
			MaxFileBytes:   s.maxFileBytes(),
		})
		// Read the command deadline before cancelling: nil-error success
		// after a per-command timeout must produce timeout, never
		// success, even with a live parent context.
		stageExpired := errors.Is(stageCtx.Err(), context.DeadlineExceeded)
		cancel()
		if err == nil {
			if stageExpired {
				return s.finished(base, CategoryTimeout,
					"The command exceeded its time limit.", step.stage, 0), nil
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = ctxErr
			}
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return s.finished(base, CategoryTimeout,
					"The command exceeded its time limit.", step.stage, 0), nil
			}
			return s.finished(base, CategoryPlatformFault,
				"The practice environment hit a platform fault.", step.stage, 0), nil
		}
		if outcome.TimedOut {
			return s.finished(base, CategoryTimeout,
				"The command exceeded its time limit.", step.stage, outcome.ExitCode), nil
		}
		base.Steps = append(base.Steps, StepSummary{Stage: step.stage, ExitCode: outcome.ExitCode})
		if outcome.ExitCode != 0 {
			category := CategoryTestFailure
			summary := "Tests failed."
			if step.stage == environmentverify.StageBuild {
				category = CategoryBuildError
				summary = "The build command failed."
			} else if step.stage == environmentverify.StageRun {
				runOutput, base.OutputTruncated = s.project(snap.redact, outcome.Output)
			}
			base.Output = runOutput
			return s.finished(base, category, summary, "", 0), nil
		}
		// A passing test command can still carry failing assertions for
		// harnesses that report through stdout: hidden tests speak the
		// CODEGYM_TEST protocol, and the backend evaluates the same lines
		// on Submit, reporting the intended failure instead of a false
		// pass. Only parsed verdict tokens surface; hidden-test source
		// never does. Output without protocol lines behaves exactly as
		// before.
		if step.stage == environmentverify.StageTest {
			if failed := environmentverify.FailedOutcomes(environmentverify.ParseTestOutcomes(outcome.Output)); len(failed) > 0 {
				return s.finished(base, CategoryTestFailure, testFailureSummary(failed), "", 0), nil
			}
		}
		if step.stage == environmentverify.StageRun {
			var truncated bool
			runOutput, truncated = s.project(snap.redact, outcome.Output)
			base.OutputTruncated = base.OutputTruncated || truncated
		}
	}
	runProjected, runTruncated := s.project(snap.redact, runOutput)
	base.Output = runProjected
	base.OutputTruncated = base.OutputTruncated || runTruncated
	base.Category = CategoryPassed
	base.Summary = "All checks passed."
	return base, nil
}

// scopeForStage assigns execution visibility: run steps execute learner
// code under learner scope, where protected, hidden, reference, and
// verifier-owned files are withheld; build and test steps run trusted
// commands under full scope with projected output.
func scopeForStage(stage string) FileScope {
	if stage == environmentverify.StageRun {
		return FileScopeLearner
	}
	return FileScopeFull
}

// protectedPaths lists manifest-declared sensitive paths in manifest
// order, deduplicated. It feeds both the provider visibility contract
// and the output boundary.
func protectedPaths(manifest environmentverify.Manifest) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(names []string) {
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	if manifest.Workspace != nil {
		add(manifest.Workspace.Protected)
	}
	if manifest.Assessment != nil {
		add(manifest.Assessment.HiddenTests)
	}
	return out
}

// reservedPathPattern matches path-like text through reserved verifier-owned
// locations (reference/solutions segments, dot-directories) that have no
// manifest presence and therefore no exact redaction token. It scrubs
// attempted-path echoes in transcripts; learner-attribution rules still
// decide what is published.
var reservedPathPattern = regexp.MustCompile(`[^\s'"]*(?:solutions/|reference/|\.codegym/)[^\s'"]*`)

// scrub replaces exact sensitive tokens and reserved-location paths with
// the redaction marker.
func scrub(redact []string, output string) string {
	for _, token := range redact {
		output = strings.ReplaceAll(output, token, "[redacted]")
	}
	return reservedPathPattern.ReplaceAllString(output, "[redacted]")
}

// testFailureSummary reports the intended failure from parsed protocol
// verdicts: test IDs plus the coded reason with expected/actual tokens.
// Hidden-test source, paths, and reference material never surface.
func testFailureSummary(failed []environmentverify.TestOutcome) string {
	parts := make([]string, 0, len(failed))
	for _, outcome := range failed {
		detail := strings.TrimSpace(outcome.TestID)
		if outcome.Assertion != nil {
			code := strings.TrimSpace(outcome.Assertion.Code)
			expected := strings.TrimSpace(outcome.Assertion.Expected)
			actual := strings.TrimSpace(outcome.Assertion.Actual)
			switch {
			case code != "" && expected != "" && actual != "":
				detail += fmt.Sprintf(" (%s: expected %q, got %q)", code, expected, actual)
			case code != "":
				detail += fmt.Sprintf(" (%s)", code)
			}
		}
		parts = append(parts, detail)
	}
	return "Tests failed: " + strings.Join(parts, "; ") + "."
}

// finished completes a terminal non-pass result, keeping recorded steps.
// Callers set Output beforehand; finished never touches provider output
// itself.
func (s *Service) finished(base Result, category Category, summary, stage string, exit int) Result {
	base.Category = category
	base.Summary = summary
	if stage != "" {
		base.Steps = append(base.Steps, StepSummary{Stage: stage, ExitCode: exit})
	}
	return base
}

// project applies the provider result boundary to decisive-step output:
// exact sensitive tokens and reserved-location paths are scrubbed first,
// then output is byte-capped. Scrubbing precedes capping so a cut can
// never leave a partial secret. Filename lists alone were insufficient —
// build tools echo protected paths and run output can exfiltrate
// protected source — so trusted contents are redacted exactly and
// unlisted verifier-owned locations scrubbed by pattern; fragments
// remain a documented residual limit.
func (s *Service) project(redact []string, output string) (string, bool) {
	output = scrub(redact, output)
	limit := s.maxOutputBytes()
	if len(output) <= limit {
		return output, false
	}
	return output[:limit] + "…[truncated]", true
}

// Record is the persisted practice execution record: artifact ID/version
// and available environment identity alongside the distinct practice
// category (plus its execution.Status mapping). It mirrors the existing
// execution store pattern without modifying existing contracts: identity
// fields the current records lack are carried here, and the #184
// proposal to add them to execution/submission records is documented in
// doc.go. Output is the already-projected safe output, never raw provider
// text.
type Record struct {
	ID              string
	SessionID       string
	Owner           string
	ArtifactID      string
	ArtifactVersion string
	Technology      string
	EnvironmentBase string
	NetworkPolicy   string
	Category        Category
	Status          execution.Status
	Summary         string
	Output          string
	OutputTruncated bool
	Steps           []StepSummary
	CreatedAt       time.Time
}

// ErrRecordNotFound reports a record lookup with no matching record.
var ErrRecordNotFound = errors.New("practice execution record not found")

// RecordStore persists practice execution records. The in-memory
// implementation below is the standing fake and test double; durable
// persistence is a production integration limit (see doc.go).
type RecordStore interface {
	SaveRecord(ctx context.Context, record Record) error
	GetRecord(ctx context.Context, id string) (Record, error)
}

// InMemoryRecordStore keeps execution records in process memory:
// mutex-guarded map, copies on write and read, explicit not-found
// errors — the same pattern as the execution and registry stores.
type InMemoryRecordStore struct {
	mu      sync.Mutex
	records map[string]Record
}

// NewInMemoryRecordStore creates an empty record store.
func NewInMemoryRecordStore() *InMemoryRecordStore {
	return &InMemoryRecordStore{records: map[string]Record{}}
}

// SaveRecord inserts or replaces the record for one execution ID. Steps
// are deep-copied so later caller mutations cannot alter the stored
// record.
func (s *InMemoryRecordStore) SaveRecord(_ context.Context, record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record.Steps = append([]StepSummary(nil), record.Steps...)
	s.records[record.ID] = record
	return nil
}

// GetRecord returns one stored record or ErrRecordNotFound, with an
// independent steps copy.
func (s *InMemoryRecordStore) GetRecord(_ context.Context, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return Record{}, ErrRecordNotFound
	}
	record.Steps = append([]StepSummary(nil), record.Steps...)
	return record, nil
}

// storeResult persists the terminal result best-effort: record failures
// never fail the learner result, mirroring the execution package's
// best-effort timing records. The result links back via RecordID only on
// a successful save. Unconfigured storage skips silently.
func (s *Service) storeResult(ctx context.Context, owner string, result *Result) {
	if s.Records == nil || result == nil {
		return
	}
	record := Record{
		ID:              newRecordID(),
		SessionID:       result.SessionID,
		Owner:           owner,
		ArtifactID:      result.ArtifactID,
		ArtifactVersion: result.ArtifactVersion,
		Technology:      result.Technology,
		EnvironmentBase: result.EnvironmentBase,
		NetworkPolicy:   result.NetworkPolicy,
		Category:        result.Category,
		Status:          result.Category.ExecutionStatus(),
		Summary:         result.Summary,
		Output:          result.Output,
		OutputTruncated: result.OutputTruncated,
		Steps:           append([]StepSummary(nil), result.Steps...),
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.Records.SaveRecord(context.WithoutCancel(ctx), record); err != nil {
		return
	}
	result.RecordID = record.ID
}

func newRecordID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "practice_rec_fallback"
	}
	return "practice_rec_" + hex.EncodeToString(raw[:])
}

// overlayFiles converts the isolated overlay to the existing execution
// file contract, sorted by path for deterministic provider input.
func overlayFiles(files map[string]string) []execution.File {
	out := make([]execution.File, 0, len(files))
	for name, content := range files {
		out = append(out, execution.File{Path: name, Content: content})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// workspaceRoot reports the manifest workspace root for the provider
// working directory, defaulting to "." for a valid manifest.
func workspaceRoot(manifest environmentverify.Manifest) string {
	if manifest.Workspace == nil {
		return "."
	}
	if root := strings.TrimSpace(manifest.Workspace.Root); root != "" {
		return root
	}
	return "."
}

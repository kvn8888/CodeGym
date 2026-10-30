package submission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/session"
)

var ErrInvalidInput = errors.New("invalid submission input")

const MaxRevealedFailureBytes = 2048

type Service struct {
	problems    *problems.Service
	executions  *execution.Service
	sessions    *session.Service
	memory      *memory.Service
	profiles    *generation.ProfileSynthesizer
	coaching    CoachingUsageLookup
	bindTimeout time.Duration
}

// CoachingUsageLookup reports whether the scoped learner used coding coaching
// for a session by the supplied cutoff. It must not return message contents.
type CoachingUsageLookup interface {
	CodingCoachUsed(context.Context, string, time.Time) (bool, error)
}

func NewService(
	problemService *problems.Service,
	executionService *execution.Service,
	sessionService *session.Service,
	memoryService *memory.Service,
	profiles *generation.ProfileSynthesizer,
) *Service {
	return &Service{
		problems:    problemService,
		executions:  executionService,
		sessions:    sessionService,
		memory:      memoryService,
		profiles:    profiles,
		bindTimeout: defaultSubmissionRequestBindTimeout,
	}
}

func (s *Service) WithCoachingUsageLookup(lookup CoachingUsageLookup) *Service {
	if s != nil {
		s.coaching = lookup
	}
	return s
}

// WithSubmissionRequestBindTimeout overrides how long a duplicate submit waits
// for the original request to bind its run before returning a retryable
// conflict. Non-positive values restore the default.
func (s *Service) WithSubmissionRequestBindTimeout(timeout time.Duration) *Service {
	if s != nil {
		if timeout <= 0 {
			timeout = defaultSubmissionRequestBindTimeout
		}
		s.bindTimeout = timeout
	}
	return s
}

func (s *Service) Submit(ctx context.Context, input SubmitInput) (Accepted, error) {
	mode, err := normalizeMode(input.Mode)
	if err != nil {
		return Accepted{}, err
	}
	problemID := strings.TrimSpace(input.ProblemID)
	if problemID == "" {
		return Accepted{}, fmt.Errorf("%w: problem_id is required", ErrInvalidInput)
	}

	definition, err := s.problems.GetDefinition(ctx, problemID)
	if err != nil {
		return Accepted{}, err
	}

	sessionID := strings.TrimSpace(input.SessionID)
	var practiceSession session.Session
	if sessionID != "" {
		practiceSession, err = s.submissionSession(ctx, sessionID, problemID)
		if err != nil {
			return Accepted{}, err
		}
	}

	testFiles := definition.HiddenTestFiles
	if mode == ModeRun {
		testFiles = definition.PublicTestFiles
		if len(testFiles) == 0 {
			return Accepted{}, fmt.Errorf("%w: problem has no public cases", ErrInvalidInput)
		}
	}
	files, err := AssembleFiles(input.Files, testFiles)
	if err != nil {
		return Accepted{}, err
	}

	var run execution.Run
	runID := ""
	replayed := false
	reservedInProgress := false
	token := strings.TrimSpace(input.ClientToken)
	if token != "" {
		reservation, reserveErr := s.executions.ReserveSubmissionRequest(ctx, token, submissionRequestFingerprint(input, string(mode)))
		if reserveErr != nil {
			return Accepted{}, reserveErr
		}
		if !reservation.Created {
			if reservation.Status == execution.SubmissionRequestStatusCompleted && reservation.SubmissionID != "" {
				// An uncertain-response retry of one logical submit returns the
				// original submission identity and does not start another run.
				replayed = true
				runID = reservation.SubmissionID
				fetched, fetchErr := s.executions.GetRun(ctx, reservation.SubmissionID)
				if fetchErr != nil {
					return Accepted{}, fetchErr
				}
				run = fetched
			} else if reservation.Status == execution.SubmissionRequestStatusInProgress {
				reservedInProgress = true
				// Another request owns this logical submission's execution and
				// learning evidence; wait for its run instead of starting a
				// second execution.
				replayed = true
				bound, bindErr := s.awaitSubmissionRequestRun(ctx, token, reservation)
				if bindErr != nil {
					return Accepted{}, bindErr
				}
				reservedRun, waitErr := s.waitForSubmissionRun(ctx, bound.RunID)
				if waitErr != nil {
					return Accepted{}, waitErr
				}
				runID = reservedRun.ID
				run = reservedRun
				if err := s.executions.CompleteSubmissionRequest(ctx, token, run.ID); err != nil {
					return Accepted{}, err
				}
			}
		}
	}
	if !reservedInProgress && !replayed {
		run, err = s.executions.SubmitRun(ctx, execution.SubmitRunInput{
			ProblemID:  problemID,
			Language:   definition.Language,
			Entrypoint: definition.Entrypoint,
			Strategy:   string(definition.TestConfig.Strategy),
			Mode:       string(mode),
			Files:      files,
			Limits: execution.Limits{
				TimeoutSeconds: definition.Runtime.TimeoutSeconds,
				MemoryMB:       definition.Runtime.MemoryMB,
				NetworkMode:    definition.Runtime.NetworkMode,
			},
		})
		if err != nil {
			return Accepted{}, err
		}
		runID = run.ID
		if token != "" {
			if err := s.executions.BindSubmissionRequestRun(ctx, token, runID); err != nil {
				return Accepted{}, err
			}
			if err := s.executions.CompleteSubmissionRequest(ctx, token, runID); err != nil {
				return Accepted{}, err
			}
		}
	}

	memoryStatus := ""
	if mode == ModeSubmit && sessionID != "" && isTerminal(run.Status) {
		sessionFiles := make([]session.FileInput, 0, len(input.Files))
		for _, file := range input.Files {
			sessionFiles = append(sessionFiles, session.FileInput{
				Path:    file.Path,
				Content: file.Content,
			})
		}
		if _, err := s.sessions.UpsertFiles(ctx, sessionID, session.UpsertFilesInput{Files: sessionFiles}); err != nil {
			return Accepted{}, fmt.Errorf("save submitted session files: %w", err)
		}

		view := viewFromRun(run)
		if isGradedAssessment(view) {
			memoryEventsSaved := true
			if !replayed {
				assistance := s.assistanceEvidence(ctx, practiceSession, run.CreatedAt)
				started := map[string]any{"submission_id": run.ID, "file_count": len(input.Files)}
				addAssistanceEvidence(started, assistance)
				memoryEventsSaved = s.recordEvent(ctx, memory.TypeAttemptStarted, "Started a coding attempt.", definition, sessionID, started)
				memoryEventsSaved = s.recordEvent(ctx, memory.TypeAttemptSubmitted, "Submitted a coding attempt.", definition, sessionID, map[string]any{
					"submission_id": run.ID,
				}) && memoryEventsSaved
				memoryEventsSaved = s.recordEvent(ctx, memory.TypeTestsRun, "Ran coding problem tests.", definition, sessionID, map[string]any{
					"submission_id": run.ID,
					"total":         view.Result.Total, "passed_count": view.Result.Passed,
					"failed_count": view.Result.Failed, "duration_ms": view.Result.DurationMs,
				}) && memoryEventsSaved
				outcomeType := memory.TypeAttemptFailed
				summary := "Coding attempt did not pass."
				if view.Result.Status == "pass" {
					outcomeType, summary = memory.TypeAttemptSolved, "Solved a coding problem."
				}
				outcome := map[string]any{
					"submission_id": run.ID, "graded": true,
					"passed": view.Result.Status == "pass", "total": view.Result.Total,
					"passed_count": view.Result.Passed, "failed_count": view.Result.Failed,
					"duration_ms": view.Result.DurationMs,
				}
				addAssistanceEvidence(outcome, assistance)
				memoryEventsSaved = s.recordEvent(ctx, outcomeType, summary, definition, sessionID, outcome) && memoryEventsSaved
			}
			if view.Result.Status == "pass" {
				completed := session.StatusCompleted
				if _, err := s.sessions.Update(ctx, sessionID, session.UpdateInput{Status: &completed}); err != nil {
					return Accepted{}, fmt.Errorf("complete passed session: %w", err)
				}
			}
			if replayed {
				memoryStatus = s.settledMemoryStatus(ctx, sessionID)
			} else {
				memoryStatus = s.refreshMemory(ctx, sessionID, problemID, run.ID, memoryEventsSaved)
			}
		}
	}

	return Accepted{SubmissionID: run.ID, MemoryUpdateStatus: memoryStatus}, nil
}

func normalizeMode(input Mode) (Mode, error) {
	mode := Mode(strings.ToLower(strings.TrimSpace(string(input))))
	if mode == "" {
		return ModeSubmit, nil
	}
	if mode != ModeRun && mode != ModeSubmit {
		return "", fmt.Errorf("%w: mode must be run or submit", ErrInvalidInput)
	}
	return mode, nil
}

func (s *Service) recordEvent(ctx context.Context, eventType, summary string, definition problems.Definition, sessionID string, extra map[string]any) bool {
	if s.memory == nil {
		return true
	}
	payload := map[string]any{
		"problem_id": definition.ID, "session_id": sessionID,
		"concept": definition.Subcategory, "difficulty": definition.Difficulty,
		"language": definition.Language, "tags": definition.Tags, "schema_version": 1,
	}
	for key, value := range extra {
		payload[key] = value
	}
	encoded, _ := json.Marshal(payload)
	if _, err := s.memory.RecordEvent(ctx, memory.RecordEventInput{
		Source: memory.SourceWorkspace, Type: eventType, Summary: summary, Payload: encoded,
	}); err != nil {
		log.Printf("could not record workspace.%s memory event: %v", eventType, err)
		return false
	}
	return true
}

// defaultSubmissionRequestBindTimeout bounds how long a duplicate submit waits
// for the original request to bind its run before surfacing a retryable
// conflict.
const defaultSubmissionRequestBindTimeout = 2 * time.Second

// awaitSubmissionRequestRun waits until the reservation records the run it
// bound, so a duplicate submit never starts a second execution.
func (s *Service) awaitSubmissionRequestRun(ctx context.Context, token string, reservation execution.SubmissionRequestReservation) (execution.SubmissionRequestReservation, error) {
	if strings.TrimSpace(reservation.RunID) != "" {
		return reservation, nil
	}
	timeout := s.bindTimeout
	if timeout <= 0 {
		timeout = defaultSubmissionRequestBindTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		current, err := s.executions.GetSubmissionRequest(ctx, token)
		if err != nil {
			if errors.Is(err, execution.ErrSubmissionRequestNotFound) {
				return execution.SubmissionRequestReservation{}, execution.ErrSubmissionRequestInProgress
			}
			return execution.SubmissionRequestReservation{}, err
		}
		if strings.TrimSpace(current.RunID) != "" || (current.Status == execution.SubmissionRequestStatusCompleted && current.SubmissionID != "") {
			return current, nil
		}
		if time.Now().After(deadline) {
			return execution.SubmissionRequestReservation{}, execution.ErrSubmissionRequestInProgress
		}
		select {
		case <-ctx.Done():
			return execution.SubmissionRequestReservation{}, execution.ErrSubmissionRequestInProgress
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (s *Service) waitForSubmissionRun(ctx context.Context, runID string) (execution.Run, error) {
	if strings.TrimSpace(runID) == "" {
		return execution.Run{}, execution.ErrSubmissionRequestInProgress
	}
	for {
		run, err := s.executions.GetRun(ctx, runID)
		if err == nil && isTerminal(run.Status) {
			return run, nil
		}
		if err != nil && !errors.Is(err, execution.ErrRunNotFound) {
			return execution.Run{}, err
		}
		if ctx.Err() != nil {
			return execution.Run{}, ctx.Err()
		}
		select {
		case <-ctx.Done():
			return execution.Run{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func submissionRequestFingerprint(input SubmitInput, mode string) string {
	filesJSON, err := json.Marshal(input.Files)
	if err != nil {
		filesJSON = json.RawMessage(`null`)
	}
	hashed := sha256.Sum256([]byte(strings.TrimSpace(mode) + "\x00" + strings.TrimSpace(input.ProblemID) + "\x00" + strings.TrimSpace(input.SessionID) + "\x00" + string(filesJSON)))
	return hex.EncodeToString(hashed[:])
}

func (s *Service) refreshMemory(ctx context.Context, sessionID, problemID, submissionID string, eventsSaved bool) string {
	if s.profiles == nil {
		_ = s.setMemoryUpdateStatus(ctx, sessionID, submissionID, "idle")
		return ""
	}
	if err := s.setMemoryUpdateStatus(ctx, sessionID, submissionID, "pending"); err != nil {
		log.Printf("could not claim memory update status for session %s: %v", sessionID, err)
	}
	if !eventsSaved {
		if applied, err := s.applyMemoryUpdateStatus(ctx, sessionID, submissionID, "failed"); err != nil {
			log.Printf("could not persist failed memory update status for session %s: %v", sessionID, err)
		} else if !applied {
			return ""
		}
		return "failed"
	}
	result, err := s.profiles.RefreshProfile(ctx, generation.ProfileRefreshInput{
		SessionID: sessionID, ProblemID: problemID, SubmissionID: submissionID, Trigger: "set-completion",
	})
	status := "synced"
	outcome, _ := generation.CompletionMaintenanceOutcome(result, err)
	if outcome == generation.CompletionOutcomeFailed {
		status = "failed"
		reason := "memory update failed"
		if err == nil && result.Skipped != "" {
			reason = result.Skipped
		}
		log.Printf("coding result saved but memory update failed: %s", reason)
	}
	if applied, updateErr := s.applyMemoryUpdateStatus(ctx, sessionID, submissionID, status); updateErr != nil {
		log.Printf("could not persist memory update status for session %s: %v", sessionID, updateErr)
		if status == "synced" {
			return "failed"
		}
	} else if !applied {
		// A newer completion already owns the session status. Do not report a
		// status that was not stored for this submission.
		log.Printf("discarded stale %s memory update status for session %s", status, sessionID)
		return ""
	}
	return status
}

// settledMemoryStatus reports a memory status that has already been decided by
// the request that owns the completion. Joined/duplicated submits never
// report a status for a refresh that is still running.
func (s *Service) settledMemoryStatus(ctx context.Context, sessionID string) string {
	if strings.TrimSpace(sessionID) == "" || s.sessions == nil {
		return ""
	}
	current, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return ""
	}
	state := map[string]any{}
	if len(current.State) == 0 || json.Unmarshal(current.State, &state) != nil {
		return ""
	}
	status, _ := state["memory_update_status"].(string)
	if status == "synced" || status == "failed" {
		return status
	}
	return ""
}

func (s *Service) setMemoryUpdateStatus(ctx context.Context, sessionID, submissionID, status string) error {
	_, err := s.sessions.SetMemoryUpdateStatusForSubmission(ctx, sessionID, submissionID, status)
	return err
}

func (s *Service) applyMemoryUpdateStatus(ctx context.Context, sessionID, submissionID, status string) (bool, error) {
	_, applied, err := s.sessions.SetMemoryUpdateStatusIfCurrent(ctx, sessionID, submissionID, status)
	return applied, err
}

func (s *Service) Get(ctx context.Context, id string) (View, error) {
	run, err := s.executions.GetRun(ctx, strings.TrimSpace(id))
	if err != nil {
		return View{}, err
	}
	return viewFromRun(run), nil
}

func (s *Service) submissionSession(ctx context.Context, sessionID, problemID string) (session.Session, error) {
	found, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return session.Session{}, err
	}
	if found.Kind != session.KindWorkspace {
		return session.Session{}, fmt.Errorf("%w: session_id must identify a workspace session", ErrInvalidInput)
	}
	if found.ProblemID != problemID {
		return session.Session{}, fmt.Errorf("%w: session problem_id does not match submission problem_id", ErrInvalidInput)
	}
	return found, nil
}

type assistanceEvidence struct {
	status        string
	hintCount     int
	hintsKnown    bool
	coachingUsed  bool
	coachingKnown bool
}

func (s *Service) assistanceEvidence(ctx context.Context, practiceSession session.Session, cutoff time.Time) assistanceEvidence {
	evidence := assistanceEvidence{status: "unknown"}
	var state map[string]any
	if len(practiceSession.State) > 0 && json.Unmarshal(practiceSession.State, &state) == nil {
		if count, ok := nonNegativeInt(state["hints_revealed"]); ok {
			evidence.hintCount = min(count, 100)
			evidence.hintsKnown = true
		}
	}
	if s.coaching != nil {
		used, err := s.coaching.CodingCoachUsed(ctx, practiceSession.ID, cutoff)
		if err == nil {
			evidence.coachingUsed = used
			evidence.coachingKnown = true
		}
	}
	if (evidence.hintsKnown && evidence.hintCount > 0) || (evidence.coachingKnown && evidence.coachingUsed) {
		evidence.status = "used"
	} else if evidence.hintsKnown && evidence.coachingKnown {
		evidence.status = "not_used"
	}
	return evidence
}

func addAssistanceEvidence(payload map[string]any, evidence assistanceEvidence) {
	payload["assistance_status"] = evidence.status
	if evidence.hintsKnown {
		payload["hints_used"] = evidence.hintCount > 0
		payload["hint_count"] = evidence.hintCount
	}
	if evidence.coachingKnown {
		payload["coaching_used"] = evidence.coachingUsed
	}
	if evidence.status != "unknown" {
		payload["used_help"] = evidence.status == "used"
	}
}

func nonNegativeInt(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if typed < 0 || typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case int:
		if typed < 0 {
			return 0, false
		}
		return typed, true
	default:
		return 0, false
	}
}

func isGradedAssessment(view View) bool {
	return view.Status == StatusCompleted && view.Result != nil &&
		(view.Result.Status == "pass" || view.Result.Status == "fail")
}

// AssembleFiles merges learner files with the selected server-owned judge
// bundle. Judge paths are reserved so user uploads cannot overwrite either the
// public-only or full-suite harness.
func AssembleFiles(userFiles []execution.File, testFiles []problems.File) ([]execution.File, error) {
	testPaths := make(map[string]struct{}, len(testFiles))
	for _, file := range testFiles {
		testPaths[file.Path] = struct{}{}
	}

	files := make([]execution.File, 0, len(userFiles)+len(testFiles))
	for _, file := range userFiles {
		if _, reserved := testPaths[file.Path]; reserved {
			return nil, fmt.Errorf("%w: file path %q is reserved by the problem", ErrInvalidInput, file.Path)
		}
		files = append(files, file)
	}
	for _, file := range testFiles {
		files = append(files, execution.File{Path: file.Path, Content: file.Content})
	}
	return files, nil
}

func isTerminal(status execution.Status) bool {
	switch status {
	case execution.StatusPassed, execution.StatusFailed, execution.StatusTimeout,
		execution.StatusOutOfMemory, execution.StatusCrashed, execution.StatusError:
		return true
	default:
		return false
	}
}

func viewFromRun(run execution.Run) View {
	mode, err := normalizeMode(Mode(run.Mode))
	if err != nil {
		mode = ModeSubmit
	}
	view := View{Mode: mode}
	if run.JudgeResult != nil {
		view.Stdout = run.JudgeResult.Stdout
		view.OutputTruncated = run.JudgeResult.OutputTruncated
		view.FailureDetail = truncateOptionalFailure(run.JudgeResult.FailureDetail)
		view.ExecutedCount = len(run.JudgeResult.Cases)
	}
	switch run.Status {
	case execution.StatusQueued:
		view.Status = StatusPending
		return view
	case execution.StatusRunning:
		view.Status = StatusRunning
		return view
	case execution.StatusError:
		view.Status = StatusError
		if run.Error == execution.ErrInfrastructureBusy.Error() {
			detail := run.Error
			view.FailureDetail = &detail
		}
		return view
	case execution.StatusTimeout:
		view.Status = StatusTimeout
		return view
	case execution.StatusOutOfMemory:
		view.Status = StatusOutOfMemory
		return view
	case execution.StatusCrashed:
		view.Status = StatusCrashed
		return view
	case execution.StatusPassed, execution.StatusFailed:
		var result TestResult
		var err error
		if run.JudgeResult != nil {
			result, err = TestResultFromJudge(*run.JudgeResult)
		} else {
			result, err = ParseTestResult(run.Output)
		}
		if err != nil {
			view.Status = StatusError
			return view
		}
		result.DurationMs = run.DurationMs
		truncateTestResultFailures(&result)
		view.Status = StatusCompleted
		view.Result = &result
		view.ExecutedCount = result.Total
		return view
	default:
		view.Status = StatusError
		return view
	}
}

func truncateTestResultFailures(result *TestResult) {
	if result == nil {
		return
	}
	result.CompileError = truncateOptionalFailure(result.CompileError)
	for index := range result.TestCases {
		result.TestCases[index].Error = truncateOptionalFailure(result.TestCases[index].Error)
	}
}

func truncateOptionalFailure(value *string) *string {
	if value == nil {
		return nil
	}
	truncated := truncateFailure(*value)
	return &truncated
}

func truncateFailure(value string) string {
	if len(value) <= MaxRevealedFailureBytes {
		return value
	}
	const suffix = "... [truncated]"
	limit := MaxRevealedFailureBytes - len(suffix)
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit] + suffix
}

package submission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/session"
)

var ErrInvalidInput = errors.New("invalid submission input")

type Service struct {
	problems   *problems.Service
	executions *execution.Service
	sessions   *session.Service
	memory     *memory.Service
	profiles   *generation.ProfileSynthesizer
}

func NewService(
	problemService *problems.Service,
	executionService *execution.Service,
	sessionService *session.Service,
	memoryService *memory.Service,
	profiles *generation.ProfileSynthesizer,
) *Service {
	return &Service{
		problems:   problemService,
		executions: executionService,
		sessions:   sessionService,
		memory:     memoryService,
		profiles:   profiles,
	}
}

func (s *Service) Submit(ctx context.Context, input SubmitInput) (Accepted, error) {
	problemID := strings.TrimSpace(input.ProblemID)
	if problemID == "" {
		return Accepted{}, fmt.Errorf("%w: problem_id is required", ErrInvalidInput)
	}

	definition, err := s.problems.GetDefinition(ctx, problemID)
	if err != nil {
		return Accepted{}, err
	}

	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID != "" {
		if err := s.validateSession(ctx, sessionID, problemID); err != nil {
			return Accepted{}, err
		}
	}

	s.recordEvent(ctx, memory.TypeAttemptStarted, "Started a coding attempt.", definition, sessionID, map[string]any{
		"file_count": len(input.Files),
	})

	files, err := AssembleFiles(input.Files, definition.HiddenTestFiles)
	if err != nil {
		return Accepted{}, err
	}
	s.recordEvent(ctx, memory.TypeAttemptSubmitted, "Submitted a coding attempt.", definition, sessionID, nil)
	run, err := s.executions.SubmitRun(ctx, execution.SubmitRunInput{
		ProblemID:  problemID,
		Language:   definition.Language,
		Entrypoint: definition.Entrypoint,
		Files:      files,
	})
	if err != nil {
		return Accepted{}, err
	}

	memoryStatus := ""
	if sessionID != "" && isTerminal(run.Status) {
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
		memoryEventsSaved := true
		if view.Result != nil {
			memoryEventsSaved = s.recordEvent(ctx, memory.TypeTestsRun, "Ran coding problem tests.", definition, sessionID, map[string]any{
				"total": view.Result.Total, "passed_count": view.Result.Passed,
				"failed_count": view.Result.Failed, "duration_ms": view.Result.DurationMs,
			})
			outcomeType := memory.TypeAttemptFailed
			summary := "Coding attempt did not pass."
			if view.Result.Status == "pass" {
				outcomeType, summary = memory.TypeAttemptSolved, "Solved a coding problem."
			}
			memoryEventsSaved = s.recordEvent(ctx, outcomeType, summary, definition, sessionID, map[string]any{
				"passed": view.Result.Status == "pass", "total": view.Result.Total,
				"passed_count": view.Result.Passed, "failed_count": view.Result.Failed,
				"duration_ms": view.Result.DurationMs,
			}) && memoryEventsSaved
		} else {
			memoryEventsSaved = s.recordEvent(ctx, memory.TypeAttemptFailed, "Coding attempt could not produce test results.", definition, sessionID, map[string]any{
				"passed": false, "total": 0, "passed_count": 0, "failed_count": 0,
				"duration_ms": run.DurationMs,
			})
		}
		if view.Result != nil && view.Result.Status == "pass" {
			completed := session.StatusCompleted
			if _, err := s.sessions.Update(ctx, sessionID, session.UpdateInput{Status: &completed}); err != nil {
				return Accepted{}, fmt.Errorf("complete passed session: %w", err)
			}
		}
		memoryStatus = s.refreshMemory(ctx, sessionID, run.ID, memoryEventsSaved)
	}

	return Accepted{SubmissionID: run.ID, MemoryUpdateStatus: memoryStatus}, nil
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

func (s *Service) refreshMemory(ctx context.Context, sessionID, submissionID string, eventsSaved bool) string {
	if s.profiles == nil {
		_ = s.setMemoryUpdateStatus(ctx, sessionID, submissionID, "idle")
		return ""
	}
	_ = s.setMemoryUpdateStatus(ctx, sessionID, submissionID, "pending")
	if !eventsSaved {
		_ = s.setMemoryUpdateStatus(ctx, sessionID, submissionID, "failed")
		return "failed"
	}
	result, err := s.profiles.RefreshProfile(ctx, generation.ProfileRefreshInput{
		SessionID: sessionID, Trigger: "set-completion",
	})
	status := "synced"
	if err != nil || strings.HasPrefix(result.Skipped, "profile generation failed") ||
		strings.HasPrefix(result.Skipped, "generated profile was invalid") ||
		result.Skipped == "generation is not configured" {
		status = "failed"
		if err != nil {
			log.Printf("coding result saved but memory update failed: %v", err)
		} else {
			log.Printf("coding result saved but memory update used failure fallback: %s", result.Skipped)
		}
	}
	if updateErr := s.setMemoryUpdateStatus(ctx, sessionID, submissionID, status); updateErr != nil {
		log.Printf("could not persist memory update status for session %s: %v", sessionID, updateErr)
		if status == "synced" {
			return "failed"
		}
	}
	return status
}

func (s *Service) setMemoryUpdateStatus(ctx context.Context, sessionID, submissionID, status string) error {
	current, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	state := map[string]any{}
	if len(current.State) > 0 {
		_ = json.Unmarshal(current.State, &state)
	}
	state["memory_update_status"] = status
	state["last_submission_id"] = submissionID
	encodedJSON, err := json.Marshal(state)
	if err != nil {
		return err
	}
	encoded := json.RawMessage(encodedJSON)
	_, err = s.sessions.Update(ctx, sessionID, session.UpdateInput{State: &encoded})
	return err
}

func (s *Service) Get(ctx context.Context, id string) (View, error) {
	run, err := s.executions.GetRun(ctx, strings.TrimSpace(id))
	if err != nil {
		return View{}, err
	}
	return viewFromRun(run), nil
}

func (s *Service) validateSession(ctx context.Context, sessionID, problemID string) error {
	found, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if found.Kind != session.KindWorkspace {
		return fmt.Errorf("%w: session_id must identify a workspace session", ErrInvalidInput)
	}
	if found.ProblemID != problemID {
		return fmt.Errorf("%w: session problem_id does not match submission problem_id", ErrInvalidInput)
	}
	return nil
}

// AssembleFiles merges learner files with server-owned hidden tests. Hidden
// paths are reserved so user uploads cannot overwrite the harness.
func AssembleFiles(userFiles []execution.File, hiddenFiles []problems.File) ([]execution.File, error) {
	hiddenPaths := make(map[string]struct{}, len(hiddenFiles))
	for _, file := range hiddenFiles {
		hiddenPaths[file.Path] = struct{}{}
	}

	files := make([]execution.File, 0, len(userFiles)+len(hiddenFiles))
	for _, file := range userFiles {
		if _, reserved := hiddenPaths[file.Path]; reserved {
			return nil, fmt.Errorf("%w: file path %q is reserved by the problem", ErrInvalidInput, file.Path)
		}
		files = append(files, file)
	}
	for _, file := range hiddenFiles {
		files = append(files, execution.File{Path: file.Path, Content: file.Content})
	}
	return files, nil
}

func isTerminal(status execution.Status) bool {
	switch status {
	case execution.StatusPassed, execution.StatusFailed, execution.StatusError:
		return true
	default:
		return false
	}
}

func viewFromRun(run execution.Run) View {
	switch run.Status {
	case execution.StatusQueued:
		return View{Status: StatusPending}
	case execution.StatusRunning:
		return View{Status: StatusRunning}
	case execution.StatusError:
		return View{Status: StatusError}
	case execution.StatusPassed, execution.StatusFailed:
		result, err := ParseTestResult(run.Output)
		if err != nil {
			return View{Status: StatusError}
		}
		result.DurationMs = run.DurationMs
		return View{Status: StatusCompleted, Result: &result}
	default:
		return View{Status: StatusError}
	}
}

package submission

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/session"
)

var ErrInvalidInput = errors.New("invalid submission input")

type Service struct {
	problems   *problems.Service
	executions *execution.Service
	sessions   *session.Service
}

func NewService(
	problemService *problems.Service,
	executionService *execution.Service,
	sessionService *session.Service,
) *Service {
	return &Service{
		problems:   problemService,
		executions: executionService,
		sessions:   sessionService,
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

	files, err := assembleFiles(input.Files, definition.HiddenTestFiles)
	if err != nil {
		return Accepted{}, err
	}
	run, err := s.executions.SubmitRun(ctx, execution.SubmitRunInput{
		ProblemID:  problemID,
		Language:   definition.Language,
		Entrypoint: definition.Entrypoint,
		Files:      files,
	})
	if err != nil {
		return Accepted{}, err
	}

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
		if view.Result != nil && view.Result.Status == "pass" {
			completed := session.StatusCompleted
			if _, err := s.sessions.Update(ctx, sessionID, session.UpdateInput{Status: &completed}); err != nil {
				return Accepted{}, fmt.Errorf("complete passed session: %w", err)
			}
		}
	}

	return Accepted{SubmissionID: run.ID}, nil
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

func assembleFiles(userFiles []execution.File, hiddenFiles []problems.File) ([]execution.File, error) {
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

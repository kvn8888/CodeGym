package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

type Clock func() time.Time

type Service struct {
	store Store
	now   Clock
}

func NewService(store Store, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: store, now: clock}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Session, error) {
	id, err := identityFromContext(ctx)
	if err != nil {
		return Session{}, err
	}

	kind, err := normalizeKind(input.Kind)
	if err != nil {
		return Session{}, err
	}

	state, err := normalizeState(input.State)
	if err != nil {
		return Session{}, err
	}

	now := s.now().UTC()
	session := Session{
		ID:              newID("sess"),
		WorkspaceID:     id.workspaceID,
		UserID:          id.userID,
		Kind:            kind,
		Status:          StatusActive,
		Title:           strings.TrimSpace(input.Title),
		ProblemID:       strings.TrimSpace(input.ProblemID),
		GenerationJobID: strings.TrimSpace(input.GenerationJobID),
		State:           state,
		CreatedAt:       now,
		UpdatedAt:       now,
		LastActivityAt:  now,
	}
	return s.store.Create(ctx, session)
}

func (s *Service) Get(ctx context.Context, sessionID string) (Session, error) {
	id, err := identityFromContext(ctx)
	if err != nil {
		return Session{}, err
	}
	return s.store.Get(ctx, id.workspaceID, id.userID, strings.TrimSpace(sessionID))
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]Summary, error) {
	id, err := identityFromContext(ctx)
	if err != nil {
		return nil, err
	}

	filter.Kind, err = normalizeOptionalKind(filter.Kind)
	if err != nil {
		return nil, err
	}
	filter.Status, err = normalizeOptionalStatus(filter.Status)
	if err != nil {
		return nil, err
	}
	filter.Limit = normalizeLimit(filter.Limit)

	return s.store.List(ctx, id.workspaceID, id.userID, filter)
}

func (s *Service) Update(ctx context.Context, sessionID string, input UpdateInput) (Session, error) {
	current, err := s.Get(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}

	if input.Title != nil {
		current.Title = strings.TrimSpace(*input.Title)
	}
	if input.Status != nil {
		status, err := normalizeStatus(*input.Status)
		if err != nil {
			return Session{}, err
		}
		current.Status = status
		if status == StatusCompleted {
			now := s.now().UTC()
			current.CompletedAt = &now
		} else {
			current.CompletedAt = nil
		}
	}
	if input.State != nil {
		state, err := normalizeState(*input.State)
		if err != nil {
			return Session{}, err
		}
		current.State = state
	}

	now := s.now().UTC()
	current.UpdatedAt = now
	current.LastActivityAt = now
	return s.store.Update(ctx, current)
}

func (s *Service) UpsertFiles(ctx context.Context, sessionID string, input UpsertFilesInput) (Session, error) {
	id, err := identityFromContext(ctx)
	if err != nil {
		return Session{}, err
	}
	sessionID = strings.TrimSpace(sessionID)

	now := s.now().UTC()
	files := make([]File, 0, len(input.Files))
	seen := map[string]struct{}{}
	for _, file := range input.Files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			return Session{}, errors.New("file_path is required")
		}
		if _, ok := seen[path]; ok {
			return Session{}, fmt.Errorf("duplicate file_path %q", path)
		}
		seen[path] = struct{}{}
		files = append(files, File{
			Path:      path,
			Content:   file.Content,
			UpdatedAt: now,
		})
	}
	if len(files) == 0 {
		return s.store.Get(ctx, id.workspaceID, id.userID, sessionID)
	}

	return s.store.UpsertFiles(ctx, id.workspaceID, id.userID, sessionID, files)
}

func (s *Service) HasPendingMemoryUpdate(ctx context.Context) (bool, error) {
	id, err := identityFromContext(ctx)
	if err != nil {
		return false, err
	}
	return s.store.HasPendingMemoryUpdate(ctx, id.workspaceID, id.userID)
}

func (s *Service) SetMemoryUpdateStatus(ctx context.Context, sessionID, status string) (Session, error) {
	switch status {
	case "idle", "pending", "synced", "failed":
	default:
		return Session{}, errors.New("invalid memory update status")
	}
	current, err := s.Get(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	state := map[string]any{}
	if len(current.State) > 0 {
		_ = json.Unmarshal(current.State, &state)
	}
	state["memory_update_status"] = status
	encoded, err := json.Marshal(state)
	if err != nil {
		return Session{}, err
	}
	raw := json.RawMessage(encoded)
	return s.Update(ctx, sessionID, UpdateInput{State: &raw})
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

func normalizeKind(kind Kind) (Kind, error) {
	switch Kind(strings.TrimSpace(string(kind))) {
	case KindWorkspace:
		return KindWorkspace, nil
	case KindMCQ:
		return KindMCQ, nil
	case KindInterview:
		return KindInterview, nil
	default:
		return "", errors.New("session kind must be workspace, mcq, or interview")
	}
}

func normalizeOptionalKind(kind Kind) (Kind, error) {
	if strings.TrimSpace(string(kind)) == "" {
		return "", nil
	}
	return normalizeKind(kind)
}

func normalizeStatus(status Status) (Status, error) {
	switch Status(strings.TrimSpace(string(status))) {
	case StatusActive:
		return StatusActive, nil
	case StatusCompleted:
		return StatusCompleted, nil
	case StatusAbandoned:
		return StatusAbandoned, nil
	default:
		return "", errors.New("session status must be active, completed, or abandoned")
	}
}

func normalizeOptionalStatus(status Status) (Status, error) {
	if strings.TrimSpace(string(status)) == "" {
		return "", nil
	}
	return normalizeStatus(status)
}

func normalizeState(state json.RawMessage) (json.RawMessage, error) {
	if len(state) == 0 {
		return json.RawMessage(`{"schema_version":1}`), nil
	}
	if !json.Valid(state) {
		return nil, errors.New("session state must be valid JSON")
	}
	copied := make(json.RawMessage, len(state))
	copy(copied, state)
	return copied, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func newID(prefix string) string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}

package workflow

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

const maxEventsPerRead = 500

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store, clock func() time.Time) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: store, now: clock}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (CreateResult, error) {
	scope, err := identityFromContext(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	kind := Kind(strings.TrimSpace(string(input.Kind)))
	steps, ok := kindSteps[kind]
	if !ok {
		return CreateResult{}, errors.New("kind must be mcq_generation, memory_reflection, or mcq_next_round")
	}
	now := s.now().UTC()
	operation := Operation{
		ID: newID("workflow"), WorkspaceID: scope.workspaceID, UserID: scope.userID,
		Kind: kind, Status: StatusQueued, CreatedAt: now, UpdatedAt: now,
	}
	operation, err = s.store.CreateOperation(ctx, operation)
	if err != nil {
		return CreateResult{}, err
	}
	events := make([]Event, 0, len(steps))
	for _, step := range steps {
		var event Event
		operation, event, err = s.appendScoped(
			ctx, scope, operation.ID, step.ID, StatusQueued, nil, false,
		)
		if err != nil {
			return CreateResult{}, err
		}
		events = append(events, event)
	}
	return CreateResult{Operation: operation, Events: events}, nil
}

func (s *Service) Get(ctx context.Context, operationID string) (Operation, error) {
	scope, err := identityFromContext(ctx)
	if err != nil {
		return Operation{}, err
	}
	return s.store.GetOperation(ctx, scope.workspaceID, scope.userID, strings.TrimSpace(operationID))
}

func (s *Service) Events(ctx context.Context, operationID string, afterSequence int64) ([]Event, error) {
	scope, err := identityFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if afterSequence < 0 {
		afterSequence = 0
	}
	return s.store.ListEvents(
		ctx, scope.workspaceID, scope.userID, strings.TrimSpace(operationID),
		afterSequence, maxEventsPerRead,
	)
}

func (s *Service) Attach(ctx context.Context, operationID string, allowedKinds ...Kind) (*Reporter, error) {
	scope, err := identityFromContext(ctx)
	if err != nil {
		return nil, err
	}
	operation, err := s.store.GetOperation(
		ctx, scope.workspaceID, scope.userID, strings.TrimSpace(operationID),
	)
	if err != nil {
		return nil, err
	}
	if operation.Status == StatusSucceeded || operation.Status == StatusFailed {
		return nil, ErrTerminal
	}
	if len(allowedKinds) > 0 {
		allowed := false
		for _, kind := range allowedKinds {
			if operation.Kind == kind {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("workflow operation kind %q is not valid for this request", operation.Kind)
		}
	}
	return &Reporter{service: s, scope: scope, operation: operation}, nil
}

func (s *Service) RequireSucceeded(ctx context.Context, operationID string, stepIDs ...string) error {
	scope, err := identityFromContext(ctx)
	if err != nil {
		return err
	}
	events, err := s.store.ListEvents(
		ctx, scope.workspaceID, scope.userID, strings.TrimSpace(operationID), 0, maxEventsPerRead,
	)
	if err != nil {
		return err
	}
	latest := make(map[string]Status, len(stepIDs))
	for _, event := range events {
		latest[event.StepID] = event.Status
	}
	for _, stepID := range stepIDs {
		if latest[stepID] != StatusSucceeded {
			return fmt.Errorf("%w: %s", ErrPrerequisite, stepID)
		}
	}
	return nil
}

func (s *Service) Cancel(ctx context.Context, operationID string) (Operation, Event, error) {
	scope, err := identityFromContext(ctx)
	if err != nil {
		return Operation{}, Event{}, err
	}
	operation, err := s.store.GetOperation(
		ctx, scope.workspaceID, scope.userID, strings.TrimSpace(operationID),
	)
	if err != nil {
		return Operation{}, Event{}, err
	}
	if operation.Status == StatusSucceeded || operation.Status == StatusFailed {
		return Operation{}, Event{}, ErrTerminal
	}
	events, err := s.store.ListEvents(ctx, scope.workspaceID, scope.userID, operation.ID, 0, maxEventsPerRead)
	if err != nil {
		return Operation{}, Event{}, err
	}
	return s.appendScoped(ctx, scope, operation.ID, firstActiveStep(events), StatusFailed, map[string]any{
		"reason_code": "cancelled",
		"retryable":   true,
	}, true)
}

func firstActiveStep(events []Event) string {
	latest := map[string]Event{}
	order := []string{}
	for _, event := range events {
		if _, seen := latest[event.StepID]; !seen {
			order = append(order, event.StepID)
		}
		latest[event.StepID] = event
	}
	for _, stepID := range order {
		if latest[stepID].Status == StatusRunning {
			return stepID
		}
	}
	for _, stepID := range order {
		if latest[stepID].Status == StatusQueued {
			return stepID
		}
	}
	if len(order) > 0 {
		return order[len(order)-1]
	}
	return ""
}

func (s *Service) appendScoped(
	ctx context.Context,
	scope identity,
	operationID, stepID string,
	status Status,
	metadata map[string]any,
	terminal bool,
) (Operation, Event, error) {
	operation, err := s.store.GetOperation(
		ctx, scope.workspaceID, scope.userID, strings.TrimSpace(operationID),
	)
	if err != nil {
		return Operation{}, Event{}, err
	}
	label, ok := labelFor(operation.Kind, stepID)
	if !ok {
		return Operation{}, Event{}, fmt.Errorf("unknown step %q for workflow kind %q", stepID, operation.Kind)
	}
	if status != StatusQueued && status != StatusRunning &&
		status != StatusSucceeded && status != StatusFailed {
		return Operation{}, Event{}, errors.New("invalid workflow event status")
	}
	safeMetadata, err := sanitizeMetadata(metadata)
	if err != nil {
		return Operation{}, Event{}, err
	}
	if terminal {
		if safeMetadata == nil {
			safeMetadata = map[string]any{}
		}
		safeMetadata["terminal"] = true
	}
	operationStatus := operation.Status
	if status == StatusRunning && operationStatus == StatusQueued {
		operationStatus = StatusRunning
	}
	if terminal {
		if status == StatusFailed {
			operationStatus = StatusFailed
		} else {
			operationStatus = StatusSucceeded
		}
	}
	event := Event{
		StepID: stepID, Label: label, Status: status,
		Timestamp: s.now().UTC(), Metadata: safeMetadata,
	}
	return s.store.AppendEvent(
		ctx, scope.workspaceID, scope.userID, operation.ID, event, operationStatus,
	)
}

func labelFor(kind Kind, stepID string) (string, bool) {
	for _, step := range kindSteps[kind] {
		if step.ID == stepID {
			return step.Label, true
		}
	}
	return "", false
}

var allowedMetadataKeys = map[string]struct{}{
	"attempt":        {},
	"max_attempts":   {},
	"question_count": {},
	"event_count":    {},
	"changed":        {},
	"skipped":        {},
	"reason_code":    {},
	"retryable":      {},
}

func sanitizeMetadata(metadata map[string]any) (map[string]any, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	safe := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if _, ok := allowedMetadataKeys[key]; !ok {
			return nil, fmt.Errorf("workflow metadata key %q is not allowed", key)
		}
		switch typed := value.(type) {
		case string:
			if len(typed) > 80 {
				return nil, fmt.Errorf("workflow metadata value %q is too long", key)
			}
			safe[key] = typed
		case bool, int, int32, int64, float64:
			safe[key] = typed
		default:
			return nil, fmt.Errorf("workflow metadata value %q must be a safe scalar", key)
		}
	}
	return safe, nil
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
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
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}

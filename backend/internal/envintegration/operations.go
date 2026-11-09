package envintegration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// OperationStatus names where an environment request stands. Terminal states
// are ready and failed; everything else is still working.
type OperationStatus string

const (
	OperationPending OperationStatus = "pending"
	OperationReady   OperationStatus = "ready"
	OperationFailed  OperationStatus = "failed"
)

// EnvironmentOperation is one learner technology request traveling toward a
// practice session. It records every stage transition so progress polling
// shows movement across minutes-long builds instead of silence.
type EnvironmentOperation struct {
	ID          string
	Owner       string
	WorkspaceID string
	Technology  string
	Objective   string
	Status      OperationStatus
	Stage       string
	SessionID   string
	Error       string
	UpdatedAt   time.Time
}

// PipelineFunc runs one operation to a practice session, reporting stages
// through report. Production resolves, builds, verifies, promotes, and
// opens the session; tests inject instant fakes.
type PipelineFunc func(ctx context.Context, operation *EnvironmentOperation, report func(stage string)) (sessionID string, err error)

// OperationStore tracks environment requests in process memory. Durable
// persistence across restarts is a production hardening item; a restart
// loses pending operations explicitly rather than resurrecting them wrong.
type OperationStore struct {
	mu         sync.Mutex
	operations map[string]*EnvironmentOperation
	Pipeline   PipelineFunc
}

func newOperationID() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("envreq_%d", time.Now().UnixNano())
	}
	return "envreq_" + hex.EncodeToString(raw)
}

// Start records a request and runs its pipeline in the background, returning
// immediately so minutes-long builds never hold HTTP connections.
func (s *OperationStore) Start(owner, workspaceID, technology, objective string) (*EnvironmentOperation, error) {
	if strings.TrimSpace(technology) == "" {
		return nil, fmt.Errorf("envintegration: technology is required")
	}
	if s == nil || s.Pipeline == nil {
		return nil, fmt.Errorf("envintegration: environment pipeline is required")
	}
	operation := &EnvironmentOperation{
		ID:          newOperationID(),
		Owner:       owner,
		WorkspaceID: workspaceID,
		Technology:  technology,
		Objective:   objective,
		Status:      OperationPending,
		Stage:       "queued",
		UpdatedAt:   time.Now().UTC(),
	}
	s.mu.Lock()
	if s.operations == nil {
		s.operations = map[string]*EnvironmentOperation{}
	}
	s.operations[operation.ID] = operation
	s.mu.Unlock()
	go s.run(operation)
	return operation, nil
}

func (s *OperationStore) report(id, stage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operation, ok := s.operations[id]
	if !ok {
		return
	}
	operation.Stage = stage
	operation.UpdatedAt = time.Now().UTC()
}

func (s *OperationStore) finish(id, sessionID, failure string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operation, ok := s.operations[id]
	if !ok {
		return
	}
	if failure != "" {
		operation.Status = OperationFailed
		operation.Error = failure
	} else {
		operation.Status = OperationReady
		operation.SessionID = sessionID
	}
	operation.UpdatedAt = time.Now().UTC()
}

func (s *OperationStore) run(operation *EnvironmentOperation) {
	ctx := context.Background()
	sessionID, err := s.Pipeline(ctx, operation, func(stage string) {
		s.report(operation.ID, stage)
	})
	if err != nil {
		s.finish(operation.ID, "", err.Error())
		return
	}
	s.finish(operation.ID, sessionID, "")
}

// Get returns one operation by ID, or false when unknown.
func (s *OperationStore) Get(id string) (*EnvironmentOperation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operation, ok := s.operations[id]
	if !ok {
		return nil, false
	}
	copied := *operation
	return &copied, true
}

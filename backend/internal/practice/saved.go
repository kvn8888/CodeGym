package practice

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// SavedExercise is the durable learner-owned resume record for one
// verified exercise. Identity comes from backend state at save time, never
// from client claims: artifact ID/version, environment identity, and the
// stable backend build/snapshot reference plus the exact backend-known
// editable set. Files hold only allowed learner edits.
type SavedExercise struct {
	ID              string `json:"id"`
	Owner           string `json:"owner"`
	WorkspaceID     string `json:"workspace_id"`
	ArtifactID      string `json:"artifact_id"`
	ArtifactVersion string `json:"artifact_version"`
	Technology      string `json:"technology"`
	EnvironmentBase string `json:"environment_base"`
	NetworkPolicy   string `json:"network_policy"`
	// SnapshotRef plus BuilderRunID/VerifierRunID pin the exact backend
	// build. Resume compares them against the live artifact; a
	// same-ID/version replacement reports an explicit mismatch instead of
	// silently restoring. No hashes are used.
	SnapshotRef   string            `json:"snapshot_ref"`
	BuilderRunID  string            `json:"builder_run_id"`
	VerifierRunID string            `json:"verifier_run_id"`
	Files         map[string]string `json:"files"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// SavedSummary is the list-safe projection of a saved exercise: identity
// plus timestamps, without file contents.
type SavedSummary struct {
	ID              string    `json:"id"`
	ArtifactID      string    `json:"artifact_id"`
	ArtifactVersion string    `json:"artifact_version"`
	Technology      string    `json:"technology"`
	EnvironmentBase string    `json:"environment_base"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Summary projects the list-safe fields.
func (s SavedExercise) Summary() SavedSummary {
	return SavedSummary{
		ID:              s.ID,
		ArtifactID:      s.ArtifactID,
		ArtifactVersion: s.ArtifactVersion,
		Technology:      s.Technology,
		EnvironmentBase: s.EnvironmentBase,
		UpdatedAt:       s.UpdatedAt,
	}
}

// SavedReceipt is the public save confirmation: identity plus timestamp
// only. Snapshot/build references and file contents stay server-side:
// the build reference is resume-check input, never a client display
// field, and the client already holds the files it just saved.
type SavedReceipt struct {
	ID              string    `json:"id"`
	ArtifactID      string    `json:"artifact_id"`
	ArtifactVersion string    `json:"artifact_version"`
	Technology      string    `json:"technology"`
	EnvironmentBase string    `json:"environment_base"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Receipt projects the public save confirmation.
func (s SavedExercise) Receipt() SavedReceipt {
	return SavedReceipt{
		ID:              s.ID,
		ArtifactID:      s.ArtifactID,
		ArtifactVersion: s.ArtifactVersion,
		Technology:      s.Technology,
		EnvironmentBase: s.EnvironmentBase,
		UpdatedAt:       s.UpdatedAt,
	}
}

// Saved-exercise errors. All are explicit: missing, incompatible, or
// replaced builds never silently substitute.
var (
	ErrSavedNotFound      = errors.New("practice saved exercise not found")
	ErrSavedOwnerMismatch = errors.New("practice saved exercise belongs to another owner")
	ErrArtifactMismatch   = errors.New("practice saved exercise artifact does not match the verified artifact")
	ErrBuildChanged       = errors.New("practice saved exercise build reference changed; the artifact was replaced")
	ErrSavedStoreMissing  = errors.New("practice saved exercise store is not configured")
)

// SavedExerciseStore persists saved exercises. The in-memory
// implementation below is the standing test/development double; durable
// history uses the Postgres implementation.
type SavedExerciseStore interface {
	SaveExercise(ctx context.Context, saved SavedExercise) error
	GetSavedExercise(ctx context.Context, id string) (SavedExercise, error)
	ListSavedExercises(ctx context.Context, owner, workspaceID string) ([]SavedExercise, error)
}

// InMemorySavedExerciseStore keeps saved exercises in process memory:
// mutex-guarded map, copies on write and read, explicit not-found
// errors — the same pattern as the execution and registry stores. For
// tests and development only; production uses Postgres.
type InMemorySavedExerciseStore struct {
	mu    sync.Mutex
	saved map[string]SavedExercise
}

// NewInMemorySavedExerciseStore creates an empty saved-exercise store.
func NewInMemorySavedExerciseStore() *InMemorySavedExerciseStore {
	return &InMemorySavedExerciseStore{saved: map[string]SavedExercise{}}
}

// SaveExercise inserts or replaces the saved record. Files are deep-copied
// so later caller mutations cannot alter the stored record.
func (s *InMemorySavedExerciseStore) SaveExercise(_ context.Context, saved SavedExercise) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := make(map[string]string, len(saved.Files))
	for name, content := range saved.Files {
		files[name] = content
	}
	saved.Files = files
	s.saved[saved.ID] = saved
	return nil
}

// GetSavedExercise returns one stored record or ErrSavedNotFound, with an
// independent files copy. Ownership is enforced by the service, not the
// store, so cross-owner checks stay in one place.
func (s *InMemorySavedExerciseStore) GetSavedExercise(_ context.Context, id string) (SavedExercise, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, ok := s.saved[id]
	if !ok {
		return SavedExercise{}, ErrSavedNotFound
	}
	files := make(map[string]string, len(saved.Files))
	for name, content := range saved.Files {
		files[name] = content
	}
	saved.Files = files
	return saved, nil
}

// ListSavedExercises returns saved records for one owner/workspace pair in
// most-recently-updated order, with independent files copies.
func (s *InMemorySavedExerciseStore) ListSavedExercises(_ context.Context, owner, workspaceID string) ([]SavedExercise, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []SavedExercise
	for _, saved := range s.saved {
		if saved.Owner != owner || saved.WorkspaceID != workspaceID {
			continue
		}
		files := make(map[string]string, len(saved.Files))
		for name, content := range saved.Files {
			files[name] = content
		}
		saved.Files = files
		out = append(out, saved)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

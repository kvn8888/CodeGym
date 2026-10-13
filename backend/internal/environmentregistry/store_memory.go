package environmentregistry

import (
	"context"
	"sort"
	"sync"
	"time"
)

// InMemoryStore keeps promoted artifact records in process memory,
// following the execution/usage store pattern: mutex-guarded maps, copies
// on read, explicit not-found errors. It is the standing fake and test
// double; durable persistence is a production integration limit.
type InMemoryStore struct {
	mu      sync.RWMutex
	records map[string]ArtifactRecord
}

// NewInMemoryStore creates an empty registry store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{records: map[string]ArtifactRecord{}}
}

func recordKey(artifactID, version string) string {
	return artifactID + "\x00" + version
}

// SaveArtifact inserts or replaces the record for one artifact version.
// Replacement (not forking) on same ID/version re-promotion is the
// standing compatibility decision, flagged for review in doc.go; storage
// is not redesigned in this pass. Slices are deep-copied so later caller
// mutations cannot alter the stored record.
func (s *InMemoryStore) SaveArtifact(_ context.Context, record ArtifactRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[recordKey(record.ArtifactID, record.ArtifactVersion)] = cloneRecord(record)
	return nil
}

// ListByTechnology returns promoted records for one technology, newest
// first. Callers apply tag and version matching.
func (s *InMemoryStore) ListByTechnology(_ context.Context, technology string) ([]ArtifactRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ArtifactRecord
	for _, record := range s.records {
		if record.Technology == technology {
			out = append(out, cloneRecord(record))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// GetArtifact returns one promoted record or ErrArtifactNotFound.
func (s *InMemoryStore) GetArtifact(_ context.Context, artifactID, version string) (ArtifactRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[recordKey(artifactID, version)]
	if !ok {
		return ArtifactRecord{}, ErrArtifactNotFound
	}
	return cloneRecord(record), nil
}

// UpdateValidation advances one record's validation state and boot-check
// timestamp, or returns ErrArtifactNotFound.
func (s *InMemoryStore) UpdateValidation(_ context.Context, artifactID, version string, state ValidationState, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := recordKey(artifactID, version)
	record, ok := s.records[key]
	if !ok {
		return ErrArtifactNotFound
	}
	record.ValidationState = state
	record.LastBootCheckAt = at
	s.records[key] = record
	return nil
}

// cloneRecord deep-copies every slice a record owns so store writes and
// reads never alias caller memory.
func cloneRecord(record ArtifactRecord) ArtifactRecord {
	record.DependencyFiles = append([]string(nil), record.DependencyFiles...)
	record.Lockfiles = append([]string(nil), record.Lockfiles...)
	record.Compatibility.ReuseTags = append([]string(nil), record.Compatibility.ReuseTags...)
	return record
}

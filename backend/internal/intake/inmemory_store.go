package intake

import (
	"context"
	"sort"
	"sync"

	"github.com/kvn8888/codegym/backend/internal/generation"
)

type InMemoryStore struct {
	mu      sync.RWMutex
	records map[string]Record
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{records: map[string]Record{}}
}

func (s *InMemoryStore) Create(_ context.Context, record Record) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.ID] = cloneRecord(record)
	return cloneRecord(record), nil
}

func (s *InMemoryStore) Get(_ context.Context, workspaceID, userID, id string) (Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[id]
	if !ok || record.WorkspaceID != workspaceID || record.UserID != userID {
		return Record{}, ErrNotFound
	}
	return cloneRecord(record), nil
}

func (s *InMemoryStore) FindLatestByTopic(_ context.Context, workspaceID, userID, topic string) (Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var found *Record
	for _, record := range s.records {
		if record.WorkspaceID != workspaceID || record.UserID != userID || record.NormalizedTopic != topic {
			continue
		}
		if found == nil || record.UpdatedAt.After(found.UpdatedAt) {
			copy := cloneRecord(record)
			found = &copy
		}
	}
	if found == nil {
		return Record{}, ErrNotFound
	}
	return *found, nil
}

func (s *InMemoryStore) ListPending(_ context.Context, workspaceID, userID string, limit int) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := []Record{}
	for _, record := range s.records {
		if record.WorkspaceID == workspaceID && record.UserID == userID && record.Status == StatusPending {
			records = append(records, cloneRecord(record))
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].UpdatedAt.After(records[j].UpdatedAt) })
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (s *InMemoryStore) Update(_ context.Context, record Record) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.records[record.ID]
	if !ok || existing.WorkspaceID != record.WorkspaceID || existing.UserID != record.UserID {
		return Record{}, ErrNotFound
	}
	s.records[record.ID] = cloneRecord(record)
	return cloneRecord(record), nil
}

func cloneRecord(record Record) Record {
	record.PracticeSeed = append([]byte(nil), record.PracticeSeed...)
	record.Questions = append([]generation.IntakeQuestion(nil), record.Questions...)
	record.Answers = cloneAnswers(record.Answers)
	return record
}

func cloneAnswers(input map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

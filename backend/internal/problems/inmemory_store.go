package problems

import (
	"context"
	"sort"
	"sync"
)

type InMemoryStore struct {
	mu          sync.RWMutex
	definitions map[string]Definition
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{definitions: map[string]Definition{}}
}

func (s *InMemoryStore) EnsureSchema(_ context.Context) error {
	return nil
}

func (s *InMemoryStore) Upsert(_ context.Context, definition Definition) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.definitions[definition.ID] = cloneDefinition(definition)
	return nil
}

func (s *InMemoryStore) Get(_ context.Context, id string) (Definition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	definition, ok := s.definitions[id]
	if !ok {
		return Definition{}, ErrNotFound
	}
	return cloneDefinition(definition), nil
}

func (s *InMemoryStore) List(_ context.Context) ([]Summary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summaries := make([]Summary, 0, len(s.definitions))
	for _, definition := range s.definitions {
		summaries = append(summaries, cloneSummary(definition.Summary))
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Title < summaries[j].Title
	})
	return summaries, nil
}

func cloneDefinition(definition Definition) Definition {
	definition.Tags = append([]string(nil), definition.Tags...)
	definition.Hints = append([]Hint(nil), definition.Hints...)
	definition.Files.Skeleton = append([]FileRef(nil), definition.Files.Skeleton...)
	definition.SkeletonFiles = append([]File(nil), definition.SkeletonFiles...)
	definition.HiddenTestFiles = append([]File(nil), definition.HiddenTestFiles...)
	return definition
}

func cloneSummary(summary Summary) Summary {
	summary.Tags = append([]string(nil), summary.Tags...)
	return summary
}

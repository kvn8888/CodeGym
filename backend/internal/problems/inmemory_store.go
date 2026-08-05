package problems

import (
	"context"
	"encoding/json"
	"maps"
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

func (s *InMemoryStore) Get(_ context.Context, id, workspaceID, userID string) (Definition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	definition, ok := s.definitions[id]
	if !ok || !visibleTo(definition, workspaceID, userID) {
		return Definition{}, ErrNotFound
	}
	return cloneDefinition(definition), nil
}

func (s *InMemoryStore) List(_ context.Context, workspaceID, userID string) ([]Summary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summaries := make([]Summary, 0, len(s.definitions))
	for _, definition := range s.definitions {
		if !visibleTo(definition, workspaceID, userID) {
			continue
		}
		summaries = append(summaries, cloneSummary(definition.Summary))
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Title < summaries[j].Title
	})
	return summaries, nil
}

func visibleTo(definition Definition, workspaceID, userID string) bool {
	if definition.Visibility == "" || definition.Visibility == VisibilityGlobal {
		return true
	}
	return definition.Visibility == VisibilityWorkspace &&
		definition.WorkspaceID == workspaceID && definition.UserID == userID
}

func cloneDefinition(definition Definition) Definition {
	definition.Tags = append([]string(nil), definition.Tags...)
	definition.Hints = append([]Hint(nil), definition.Hints...)
	definition.PublicCases = clonePublicCases(definition.PublicCases)
	definition.Files.Skeleton = append([]FileRef(nil), definition.Files.Skeleton...)
	definition.SkeletonFiles = append([]File(nil), definition.SkeletonFiles...)
	definition.PublicTestFiles = append([]File(nil), definition.PublicTestFiles...)
	definition.HiddenTestFiles = append([]File(nil), definition.HiddenTestFiles...)
	return definition
}

func clonePublicCases(cases []PublicCase) []PublicCase {
	cloned := make([]PublicCase, len(cases))
	for index, testCase := range cases {
		cloned[index] = testCase
		cloned[index].Args = append([]json.RawMessage(nil), testCase.Args...)
		if testCase.Request != nil {
			request := *testCase.Request
			request.Headers = maps.Clone(testCase.Request.Headers)
			request.Body = cloneRawMessage(testCase.Request.Body)
			cloned[index].Request = &request
		}
	}
	return cloned
}

func cloneSummary(summary Summary) Summary {
	summary.Tags = append([]string(nil), summary.Tags...)
	return summary
}

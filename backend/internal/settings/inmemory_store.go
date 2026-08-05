package settings

import (
	"context"
	"sync"
)

// InMemoryStore keeps runtime settings in process memory.
type InMemoryStore struct {
	mu       sync.RWMutex
	settings map[string]Setting
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{settings: make(map[string]Setting)}
}

func (s *InMemoryStore) EnsureSchema(context.Context) error { return nil }

func (s *InMemoryStore) Get(_ context.Context, workspaceID, key string) (Setting, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	setting, ok := s.settings[cacheKey(workspaceID, key)]
	if !ok {
		return Setting{}, ErrSettingNotFound
	}
	return setting, nil
}

func (s *InMemoryStore) Upsert(_ context.Context, setting Setting) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[cacheKey(setting.WorkspaceID, setting.Key)] = setting
	return nil
}

func cacheKey(workspaceID, key string) string {
	return workspaceID + "\x00" + key
}

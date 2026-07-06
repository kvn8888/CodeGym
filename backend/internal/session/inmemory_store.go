package session

import (
	"context"
	"sort"
	"sync"
	"time"
)

type InMemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]Session
	files    map[string]map[string]File
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		sessions: map[string]Session{},
		files:    map[string]map[string]File{},
	}
}

func (s *InMemoryStore) EnsureSchema(_ context.Context) error {
	return nil
}

func (s *InMemoryStore) Create(_ context.Context, session Session) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[session.ID] = copySession(session)
	return copySession(session), nil
}

func (s *InMemoryStore) Get(_ context.Context, tenantID, userID, id string) (Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok || session.TenantID != tenantID || session.UserID != userID {
		return Session{}, ErrNotFound
	}

	session = copySession(session)
	session.Files = s.filesFor(id)
	return session, nil
}

func (s *InMemoryStore) List(_ context.Context, tenantID, userID string, filter ListFilter) ([]Summary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summaries := []Summary{}
	for _, session := range s.sessions {
		if session.TenantID != tenantID || session.UserID != userID {
			continue
		}
		if filter.Kind != "" && session.Kind != filter.Kind {
			continue
		}
		if filter.Status != "" && session.Status != filter.Status {
			continue
		}
		if !filter.Before.IsZero() && !session.LastActivityAt.Before(filter.Before) {
			continue
		}
		summaries = append(summaries, session.Summary())
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastActivityAt.After(summaries[j].LastActivityAt)
	})
	if filter.Limit > 0 && len(summaries) > filter.Limit {
		summaries = summaries[:filter.Limit]
	}
	return summaries, nil
}

func (s *InMemoryStore) Update(_ context.Context, session Session) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.sessions[session.ID]
	if !ok || current.TenantID != session.TenantID || current.UserID != session.UserID {
		return Session{}, ErrNotFound
	}
	s.sessions[session.ID] = copySession(session)
	session = copySession(session)
	session.Files = s.filesFor(session.ID)
	return session, nil
}

func (s *InMemoryStore) UpsertFiles(_ context.Context, tenantID, userID, sessionID string, files []File) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok || session.TenantID != tenantID || session.UserID != userID {
		return Session{}, ErrNotFound
	}

	if s.files[sessionID] == nil {
		s.files[sessionID] = map[string]File{}
	}
	var lastUpdate time.Time
	for _, file := range files {
		copied := file
		s.files[sessionID][file.Path] = copied
		if copied.UpdatedAt.After(lastUpdate) {
			lastUpdate = copied.UpdatedAt
		}
	}
	if lastUpdate.IsZero() {
		lastUpdate = session.UpdatedAt
	}
	session.UpdatedAt = lastUpdate
	session.LastActivityAt = lastUpdate
	s.sessions[sessionID] = copySession(session)
	session.Files = s.filesFor(sessionID)
	return copySession(session), nil
}

func (s *InMemoryStore) filesFor(sessionID string) []File {
	byPath := s.files[sessionID]
	files := make([]File, 0, len(byPath))
	for _, file := range byPath {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

func copySession(session Session) Session {
	if len(session.State) > 0 {
		state := make([]byte, len(session.State))
		copy(state, session.State)
		session.State = state
	}
	if session.CompletedAt != nil {
		completedAt := *session.CompletedAt
		session.CompletedAt = &completedAt
	}
	if len(session.Files) > 0 {
		files := make([]File, len(session.Files))
		copy(files, session.Files)
		session.Files = files
	}
	return session
}

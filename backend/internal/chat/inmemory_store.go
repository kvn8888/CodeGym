package chat

import (
	"context"
	"sort"
	"sync"
)

type InMemoryStore struct {
	mu       sync.RWMutex
	threads  map[string]Thread
	messages map[string][]Message
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		threads:  map[string]Thread{},
		messages: map[string][]Message{},
	}
}

func (s *InMemoryStore) EnsureSchema(context.Context) error { return nil }

func (s *InMemoryStore) CreateThread(_ context.Context, thread Thread) (Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.threads[thread.ID]; exists {
		return Thread{}, ErrConflict
	}
	s.threads[thread.ID] = thread
	return thread, nil
}

func (s *InMemoryStore) GetThread(_ context.Context, workspaceID, userID, threadID string) (Thread, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	thread, ok := s.threads[threadID]
	if !ok || thread.WorkspaceID != workspaceID || thread.UserID != userID {
		return Thread{}, ErrNotFound
	}
	return thread, nil
}

func (s *InMemoryStore) ListThreads(_ context.Context, workspaceID, userID string, filter ThreadFilter) ([]Thread, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Thread{}
	for _, thread := range s.threads {
		if thread.WorkspaceID != workspaceID || thread.UserID != userID {
			continue
		}
		if filter.Kind != "" && thread.Kind != filter.Kind {
			continue
		}
		if filter.Status != "" && thread.Status != filter.Status {
			continue
		}
		if filter.SessionID != "" && thread.SessionID != filter.SessionID {
			continue
		}
		out = append(out, thread)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (s *InMemoryStore) UpdateThread(_ context.Context, thread Thread) (Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.threads[thread.ID]
	if !ok || current.WorkspaceID != thread.WorkspaceID || current.UserID != thread.UserID {
		return Thread{}, ErrNotFound
	}
	s.threads[thread.ID] = thread
	return thread, nil
}

func (s *InMemoryStore) ListMessages(_ context.Context, workspaceID, userID, threadID string) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	thread, ok := s.threads[threadID]
	if !ok || thread.WorkspaceID != workspaceID || thread.UserID != userID {
		return nil, ErrNotFound
	}
	return append([]Message(nil), s.messages[threadID]...), nil
}

func (s *InMemoryStore) AppendMessage(_ context.Context, workspaceID, userID string, message Message) (Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	thread, ok := s.threads[message.ThreadID]
	if !ok || thread.WorkspaceID != workspaceID || thread.UserID != userID {
		return Message{}, false, ErrNotFound
	}
	if message.ClientMessageID != "" {
		for _, existing := range s.messages[message.ThreadID] {
			if existing.ClientMessageID == message.ClientMessageID {
				return existing, false, nil
			}
		}
	}
	s.messages[message.ThreadID] = append(s.messages[message.ThreadID], message)
	return message, true, nil
}

func (s *InMemoryStore) FindAssistantReply(_ context.Context, workspaceID, userID, threadID, replyToMessageID string) (Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	thread, ok := s.threads[threadID]
	if !ok || thread.WorkspaceID != workspaceID || thread.UserID != userID {
		return Message{}, ErrNotFound
	}
	for index := len(s.messages[threadID]) - 1; index >= 0; index-- {
		message := s.messages[threadID][index]
		if message.Role == "assistant" && message.ReplyToMessageID == replyToMessageID {
			return message, nil
		}
	}
	return Message{}, ErrNotFound
}

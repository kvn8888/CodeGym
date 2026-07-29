package chat

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("chat record not found")
	ErrConflict = errors.New("chat record conflict")
)

type Store interface {
	EnsureSchema(ctx context.Context) error
	CreateThread(ctx context.Context, thread Thread) (Thread, error)
	GetThread(ctx context.Context, workspaceID, userID, threadID string) (Thread, error)
	ListThreads(ctx context.Context, workspaceID, userID string, filter ThreadFilter) ([]Thread, error)
	UpdateThread(ctx context.Context, thread Thread) (Thread, error)
	ListMessages(ctx context.Context, workspaceID, userID, threadID string) ([]Message, error)
	AppendMessage(ctx context.Context, workspaceID, userID string, message Message) (Message, bool, error)
	FindAssistantReply(ctx context.Context, workspaceID, userID, threadID, replyToMessageID string) (Message, error)
}

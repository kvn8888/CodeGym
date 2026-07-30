package chat

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS chat_threads (
			id text PRIMARY KEY,
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			session_id text NOT NULL REFERENCES practice_sessions (id) ON DELETE CASCADE,
			kind text NOT NULL,
			mode text NOT NULL DEFAULT '',
			status text NOT NULL DEFAULT 'active',
			context jsonb NOT NULL,
			successor_id text REFERENCES chat_threads (id) ON DELETE SET NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			closed_at timestamptz,
			FOREIGN KEY (workspace_id, user_id)
				REFERENCES workspace_memberships (workspace_id, user_id)
				ON DELETE CASCADE,
			CONSTRAINT chk_chat_threads_kind CHECK (kind IN ('interview', 'coach')),
			CONSTRAINT chk_chat_threads_status CHECK (status IN ('active', 'closed', 'completed'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_threads_scope_session
			ON chat_threads (workspace_id, user_id, session_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_threads_scope_active
			ON chat_threads (workspace_id, user_id, kind, updated_at DESC)
			WHERE status = 'active'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_threads_one_active_session_kind
			ON chat_threads (workspace_id, user_id, session_id, kind)
			WHERE status = 'active'`,
		`CREATE TABLE IF NOT EXISTS session_messages (
			id text PRIMARY KEY,
			thread_id text NOT NULL REFERENCES chat_threads (id) ON DELETE CASCADE,
			role text NOT NULL,
			content text NOT NULL,
			status text NOT NULL DEFAULT 'complete',
			client_message_id text NOT NULL DEFAULT '',
			reply_to_message_id text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT now(),
			CONSTRAINT chk_session_messages_role CHECK (role IN ('user', 'assistant')),
			CONSTRAINT chk_session_messages_status CHECK (status IN ('complete', 'interrupted'))
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_messages_client_id
			ON session_messages (thread_id, client_message_id)
			WHERE client_message_id <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_session_messages_thread_created
			ON session_messages (thread_id, created_at ASC, id ASC)`,
	}
	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) CreateThread(ctx context.Context, thread Thread) (Thread, error) {
	contextJSON, err := json.Marshal(thread.Context)
	if err != nil {
		return Thread{}, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO chat_threads (
			id, workspace_id, user_id, session_id, kind, mode, status, context,
			successor_id, created_at, updated_at, closed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,NULLIF($9,''),$10,$11,$12)
	`, thread.ID, thread.WorkspaceID, thread.UserID, thread.SessionID, thread.Kind,
		thread.Mode, thread.Status, contextJSON, thread.SuccessorID,
		thread.CreatedAt.UTC(), thread.UpdatedAt.UTC(), nullableTime(thread.ClosedAt))
	if err != nil {
		return Thread{}, err
	}
	return s.GetThread(ctx, thread.WorkspaceID, thread.UserID, thread.ID)
}

func (s *PostgresStore) GetThread(ctx context.Context, workspaceID, userID, threadID string) (Thread, error) {
	thread, err := scanThread(s.pool.QueryRow(ctx, `
		SELECT id, workspace_id, user_id, session_id, kind, mode, status, context::text,
			COALESCE(successor_id,''), created_at, updated_at, closed_at
		FROM chat_threads
		WHERE id=$1 AND workspace_id=$2 AND user_id=$3
	`, threadID, workspaceID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Thread{}, ErrNotFound
	}
	return thread, err
}

func (s *PostgresStore) ListThreads(ctx context.Context, workspaceID, userID string, filter ThreadFilter) ([]Thread, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, workspace_id, user_id, session_id, kind, mode, status, context::text,
			COALESCE(successor_id,''), created_at, updated_at, closed_at
		FROM chat_threads
		WHERE workspace_id=$1 AND user_id=$2
			AND ($3='' OR kind=$3)
			AND ($4='' OR status=$4)
			AND ($5='' OR session_id=$5)
		ORDER BY updated_at DESC
		LIMIT $6
	`, workspaceID, userID, filter.Kind, filter.Status, filter.SessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Thread{}
	for rows.Next() {
		thread, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, thread)
	}
	return out, rows.Err()
}

func (s *PostgresStore) UpdateThread(ctx context.Context, thread Thread) (Thread, error) {
	contextJSON, err := json.Marshal(thread.Context)
	if err != nil {
		return Thread{}, err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE chat_threads
		SET mode=$4, status=$5, context=$6::jsonb, successor_id=NULLIF($7,''),
			updated_at=$8, closed_at=$9
		WHERE id=$1 AND workspace_id=$2 AND user_id=$3
	`, thread.ID, thread.WorkspaceID, thread.UserID, thread.Mode, thread.Status,
		contextJSON, thread.SuccessorID, thread.UpdatedAt.UTC(), nullableTime(thread.ClosedAt))
	if err != nil {
		return Thread{}, err
	}
	if tag.RowsAffected() == 0 {
		return Thread{}, ErrNotFound
	}
	return s.GetThread(ctx, thread.WorkspaceID, thread.UserID, thread.ID)
}

func (s *PostgresStore) ListMessages(ctx context.Context, workspaceID, userID, threadID string) ([]Message, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.thread_id, m.role, m.content, m.status,
			m.client_message_id, m.reply_to_message_id, m.created_at
		FROM session_messages m
		JOIN chat_threads t ON t.id=m.thread_id
		WHERE m.thread_id=$1 AND t.workspace_id=$2 AND t.user_id=$3
		ORDER BY m.created_at ASC, m.id ASC
	`, threadID, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	if len(out) == 0 {
		if _, err := s.GetThread(ctx, workspaceID, userID, threadID); err != nil {
			return nil, err
		}
	}
	return out, rows.Err()
}

func (s *PostgresStore) AppendMessage(ctx context.Context, workspaceID, userID string, message Message) (Message, bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO session_messages (
			id, thread_id, role, content, status, client_message_id,
			reply_to_message_id, created_at
		)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8
		FROM chat_threads
		WHERE id=$2 AND workspace_id=$9 AND user_id=$10
		ON CONFLICT (thread_id, client_message_id)
			WHERE client_message_id <> '' DO NOTHING
	`, message.ID, message.ThreadID, message.Role, message.Content, message.Status,
		message.ClientMessageID, message.ReplyToMessageID, message.CreatedAt.UTC(),
		workspaceID, userID)
	if err != nil {
		return Message{}, false, err
	}
	if tag.RowsAffected() > 0 {
		return message, true, nil
	}
	if _, err := s.GetThread(ctx, workspaceID, userID, message.ThreadID); err != nil {
		return Message{}, false, err
	}
	if message.ClientMessageID == "" {
		return Message{}, false, ErrConflict
	}
	existing, err := scanMessage(s.pool.QueryRow(ctx, `
		SELECT m.id, m.thread_id, m.role, m.content, m.status,
			m.client_message_id, m.reply_to_message_id, m.created_at
		FROM session_messages m
		JOIN chat_threads t ON t.id=m.thread_id
		WHERE m.thread_id=$1 AND m.client_message_id=$2
			AND t.workspace_id=$3 AND t.user_id=$4
	`, message.ThreadID, message.ClientMessageID, workspaceID, userID))
	return existing, false, err
}

func (s *PostgresStore) FindAssistantReply(ctx context.Context, workspaceID, userID, threadID, replyToMessageID string) (Message, error) {
	message, err := scanMessage(s.pool.QueryRow(ctx, `
		SELECT m.id, m.thread_id, m.role, m.content, m.status,
			m.client_message_id, m.reply_to_message_id, m.created_at
		FROM session_messages m
		JOIN chat_threads t ON t.id=m.thread_id
		WHERE m.thread_id=$1 AND m.reply_to_message_id=$2 AND m.role='assistant'
			AND t.workspace_id=$3 AND t.user_id=$4
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT 1
	`, threadID, replyToMessageID, workspaceID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrNotFound
	}
	return message, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanThread(row rowScanner) (Thread, error) {
	var thread Thread
	var contextJSON string
	var closedAt *time.Time
	err := row.Scan(&thread.ID, &thread.WorkspaceID, &thread.UserID, &thread.SessionID,
		&thread.Kind, &thread.Mode, &thread.Status, &contextJSON, &thread.SuccessorID,
		&thread.CreatedAt, &thread.UpdatedAt, &closedAt)
	if err != nil {
		return Thread{}, err
	}
	if err := json.Unmarshal([]byte(contextJSON), &thread.Context); err != nil {
		return Thread{}, err
	}
	thread.ClosedAt = closedAt
	return thread, nil
}

func scanMessage(row rowScanner) (Message, error) {
	var message Message
	err := row.Scan(&message.ID, &message.ThreadID, &message.Role, &message.Content,
		&message.Status, &message.ClientMessageID, &message.ReplyToMessageID,
		&message.CreatedAt)
	return message, err
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
		// Legacy rename: tenant_id → workspace_id on existing session tables.
		`DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'practice_sessions' AND column_name = 'tenant_id'
			) THEN
				ALTER TABLE practice_sessions RENAME COLUMN tenant_id TO workspace_id;
			END IF;
		END $$`,
		`CREATE TABLE IF NOT EXISTS practice_sessions (
			id text PRIMARY KEY,
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			kind text NOT NULL,
			status text NOT NULL DEFAULT 'active',
			title text NOT NULL DEFAULT '',
			problem_id text NOT NULL DEFAULT '',
			generation_job_id text NOT NULL DEFAULT '',
			state jsonb NOT NULL DEFAULT '{"schema_version":1}'::jsonb,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			last_activity_at timestamptz NOT NULL DEFAULT now(),
			completed_at timestamptz,
			FOREIGN KEY (workspace_id, user_id)
				REFERENCES workspace_memberships (workspace_id, user_id)
				ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS session_files (
			session_id text NOT NULL REFERENCES practice_sessions (id) ON DELETE CASCADE,
			file_path text NOT NULL,
			content text NOT NULL,
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (session_id, file_path)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_practice_sessions_scope_activity ON practice_sessions (workspace_id, user_id, last_activity_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_practice_sessions_scope_active ON practice_sessions (workspace_id, user_id) WHERE status = 'active'`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'chk_practice_sessions_kind'
					AND conrelid = 'practice_sessions'::regclass
			) THEN
				ALTER TABLE practice_sessions
				ADD CONSTRAINT chk_practice_sessions_kind
				CHECK (kind IN ('workspace', 'mcq', 'interview'));
			END IF;
		END $$`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'chk_practice_sessions_status'
					AND conrelid = 'practice_sessions'::regclass
			) THEN
				ALTER TABLE practice_sessions
				ADD CONSTRAINT chk_practice_sessions_status
				CHECK (status IN ('active', 'completed', 'abandoned'));
			END IF;
		END $$`,
	}

	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) Create(ctx context.Context, session Session) (Session, error) {
	state, err := encodeState(session.State)
	if err != nil {
		return Session{}, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO practice_sessions (
			id,
			workspace_id,
			user_id,
			kind,
			status,
			title,
			problem_id,
			generation_job_id,
			state,
			created_at,
			updated_at,
			last_activity_at,
			completed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11, $12, $13)
	`, session.ID, session.WorkspaceID, session.UserID, session.Kind, session.Status, session.Title, session.ProblemID, session.GenerationJobID, state, session.CreatedAt.UTC(), session.UpdatedAt.UTC(), session.LastActivityAt.UTC(), nullableTime(session.CompletedAt))
	if err != nil {
		return Session{}, err
	}
	return s.Get(ctx, session.WorkspaceID, session.UserID, session.ID)
}

func (s *PostgresStore) Get(ctx context.Context, workspaceID, userID, id string) (Session, error) {
	session, err := s.scanSession(s.pool.QueryRow(ctx, `
		SELECT
			id,
			workspace_id,
			user_id,
			kind,
			status,
			title,
			problem_id,
			generation_job_id,
			state::text,
			created_at,
			updated_at,
			last_activity_at,
			completed_at
		FROM practice_sessions
		WHERE id = $1 AND workspace_id = $2 AND user_id = $3
	`, id, workspaceID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}

	files, err := s.listFiles(ctx, id)
	if err != nil {
		return Session{}, err
	}
	session.Files = files
	return session, nil
}

func (s *PostgresStore) List(ctx context.Context, workspaceID, userID string, filter ListFilter) ([]Summary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id,
			workspace_id,
			user_id,
			kind,
			status,
			title,
			problem_id,
			generation_job_id,
			state::text,
			created_at,
			updated_at,
			last_activity_at,
			completed_at
		FROM practice_sessions
		WHERE workspace_id = $1
			AND user_id = $2
			AND ($3 = '' OR kind = $3)
			AND ($4 = '' OR status = $4)
			AND ($5::timestamptz IS NULL OR last_activity_at < $5)
		ORDER BY last_activity_at DESC
		LIMIT $6
	`, workspaceID, userID, string(filter.Kind), string(filter.Status), nullableFilterTime(filter.Before), filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := []Summary{}
	for rows.Next() {
		session, err := s.scanSession(rows)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, session.Summary())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return summaries, nil
}

func (s *PostgresStore) Update(ctx context.Context, session Session) (Session, error) {
	state, err := encodeState(session.State)
	if err != nil {
		return Session{}, err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE practice_sessions
		SET status = $4,
			title = $5,
			state = $6::jsonb,
			updated_at = $7,
			last_activity_at = $8,
			completed_at = $9
		WHERE id = $1 AND workspace_id = $2 AND user_id = $3
	`, session.ID, session.WorkspaceID, session.UserID, session.Status, session.Title, state, session.UpdatedAt.UTC(), session.LastActivityAt.UTC(), nullableTime(session.CompletedAt))
	if err != nil {
		return Session{}, err
	}
	if tag.RowsAffected() == 0 {
		return Session{}, ErrNotFound
	}
	return s.Get(ctx, session.WorkspaceID, session.UserID, session.ID)
}

func (s *PostgresStore) UpsertFiles(ctx context.Context, workspaceID, userID, sessionID string, files []File) (Session, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var exists int
	if err := tx.QueryRow(ctx, `
		SELECT 1
		FROM practice_sessions
		WHERE id = $1 AND workspace_id = $2 AND user_id = $3
	`, sessionID, workspaceID, userID).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	} else if err != nil {
		return Session{}, err
	}

	var lastUpdate sql.NullTime
	for _, file := range files {
		if !lastUpdate.Valid || file.UpdatedAt.After(lastUpdate.Time) {
			lastUpdate = sql.NullTime{Time: file.UpdatedAt.UTC(), Valid: true}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO session_files (session_id, file_path, content, updated_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (session_id, file_path) DO UPDATE
			SET content = EXCLUDED.content,
				updated_at = EXCLUDED.updated_at
		`, sessionID, file.Path, file.Content, file.UpdatedAt.UTC()); err != nil {
			return Session{}, err
		}
	}
	if !lastUpdate.Valid {
		lastUpdate = sql.NullTime{Time: timeNowUTC(), Valid: true}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE practice_sessions
		SET updated_at = $4,
			last_activity_at = $4
		WHERE id = $1 AND workspace_id = $2 AND user_id = $3
	`, sessionID, workspaceID, userID, lastUpdate.Time); err != nil {
		return Session{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Session{}, err
	}
	return s.Get(ctx, workspaceID, userID, sessionID)
}

type scanner interface {
	Scan(dest ...any) error
}

func (s *PostgresStore) scanSession(row scanner) (Session, error) {
	var session Session
	var kind string
	var status string
	var state string
	var completedAt sql.NullTime
	if err := row.Scan(
		&session.ID,
		&session.WorkspaceID,
		&session.UserID,
		&kind,
		&status,
		&session.Title,
		&session.ProblemID,
		&session.GenerationJobID,
		&state,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.LastActivityAt,
		&completedAt,
	); err != nil {
		return Session{}, err
	}
	session.Kind = Kind(kind)
	session.Status = Status(status)
	session.State = json.RawMessage(state)
	if completedAt.Valid {
		completed := completedAt.Time
		session.CompletedAt = &completed
	}
	return session, nil
}

func (s *PostgresStore) listFiles(ctx context.Context, sessionID string) ([]File, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT file_path, content, updated_at
		FROM session_files
		WHERE session_id = $1
		ORDER BY file_path ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	files := []File{}
	for rows.Next() {
		var file File
		if err := rows.Scan(&file.Path, &file.Content, &file.UpdatedAt); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return files, nil
}

func encodeState(state json.RawMessage) (string, error) {
	if len(state) == 0 {
		return `{"schema_version":1}`, nil
	}
	if !json.Valid(state) {
		return "", fmt.Errorf("encode session state: invalid JSON")
	}
	return string(state), nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func nullableFilterTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

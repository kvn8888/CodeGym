package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists memory profiles and events in Postgres.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a Postgres-backed memory store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// EnsureSchema idempotently creates the tables and constraints used by memory
// persistence. It also renames legacy tenant_id columns from earlier schema versions.
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		// Legacy rename: tenant_id → workspace_id on existing memory tables.
		`DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'user_memory_profiles' AND column_name = 'tenant_id'
			) THEN
				ALTER TABLE user_memory_profiles RENAME COLUMN tenant_id TO workspace_id;
			END IF;
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'memory_events' AND column_name = 'tenant_id'
			) THEN
				ALTER TABLE memory_events RENAME COLUMN tenant_id TO workspace_id;
			END IF;
		END $$`,
		`CREATE TABLE IF NOT EXISTS user_memory_profiles (
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			profile jsonb NOT NULL,
			updated_at timestamptz NOT NULL,
			next_review_at timestamptz NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (workspace_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS memory_events (
			id text PRIMARY KEY,
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			source text NOT NULL,
			type text NOT NULL,
			summary text NOT NULL DEFAULT '',
			payload jsonb,
			occurred_at timestamptz NOT NULL,
			created_at timestamptz NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS memory_profile_completions (
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			submission_id text NOT NULL,
			session_id text NOT NULL,
			problem_id text NOT NULL,
			source_event_id text NOT NULL REFERENCES memory_events (id) ON DELETE CASCADE,
			refresh_id text NOT NULL,
			profile_version bigint NOT NULL,
			processed_at timestamptz NOT NULL,
			PRIMARY KEY (workspace_id, user_id, session_id, problem_id, submission_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_events_scope_created_at ON memory_events (workspace_id, user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_memory_events_scope_occurred_at ON memory_events (workspace_id, user_id, occurred_at DESC)`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'fk_user_memory_profiles_membership'
					AND conrelid = 'user_memory_profiles'::regclass
			) THEN
				ALTER TABLE user_memory_profiles
				ADD CONSTRAINT fk_user_memory_profiles_membership
				FOREIGN KEY (workspace_id, user_id)
				REFERENCES workspace_memberships (workspace_id, user_id)
				ON DELETE CASCADE
				NOT VALID;
			END IF;
		END $$`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'fk_memory_profile_completions_membership'
					AND conrelid = 'memory_profile_completions'::regclass
			) THEN
				ALTER TABLE memory_profile_completions
				ADD CONSTRAINT fk_memory_profile_completions_membership
				FOREIGN KEY (workspace_id, user_id)
				REFERENCES workspace_memberships (workspace_id, user_id)
				ON DELETE CASCADE
				NOT VALID;
			END IF;
		END $$`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'fk_memory_events_membership'
					AND conrelid = 'memory_events'::regclass
			) THEN
				ALTER TABLE memory_events
				ADD CONSTRAINT fk_memory_events_membership
				FOREIGN KEY (workspace_id, user_id)
				REFERENCES workspace_memberships (workspace_id, user_id)
				ON DELETE CASCADE
				NOT VALID;
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

func (s *PostgresStore) GetProfile(ctx context.Context, workspaceID, userID string) (Profile, error) {
	var profileJSON []byte
	err := s.pool.QueryRow(ctx, `
		SELECT profile
		FROM user_memory_profiles
		WHERE workspace_id = $1 AND user_id = $2
	`, workspaceID, userID).Scan(&profileJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrProfileNotFound
	}
	if err != nil {
		return Profile{}, err
	}

	var profile Profile
	if err := json.Unmarshal(profileJSON, &profile); err != nil {
		return Profile{}, fmt.Errorf("decode memory profile: %w", err)
	}
	return profile, nil
}

func (s *PostgresStore) UpsertProfile(ctx context.Context, workspaceID, userID string, profile Profile) error {
	current, err := s.GetProfile(ctx, workspaceID, userID)
	if errors.Is(err, ErrProfileNotFound) {
		if profile.Version == 0 {
			profile.Version = 1
		}
	} else if err != nil {
		return err
	} else {
		profile.Version = current.Version + 1
	}
	profile.UpdatedAt = profile.UpdatedAt.UTC()
	profile.NextReviewAt = profile.NextReviewAt.UTC()

	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("encode memory profile: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO user_memory_profiles (workspace_id, user_id, profile, updated_at, next_review_at)
		VALUES ($1, $2, $3::jsonb, $4, $5)
		ON CONFLICT (workspace_id, user_id) DO UPDATE
		SET profile = EXCLUDED.profile,
			updated_at = EXCLUDED.updated_at,
			next_review_at = EXCLUDED.next_review_at
	`, workspaceID, userID, string(profileJSON), profile.UpdatedAt, profile.NextReviewAt)
	return err
}

func (s *PostgresStore) ReplaceProfileIfVersion(ctx context.Context, workspaceID, userID string, profile Profile, expectedVersion int64) error {
	profile.Version = expectedVersion + 1
	profile.UpdatedAt = profile.UpdatedAt.UTC()
	profile.NextReviewAt = profile.NextReviewAt.UTC()

	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("encode memory profile: %w", err)
	}

	var tag pgconn.CommandTag
	var execErr error
	if expectedVersion == 0 {
		tag, execErr = s.pool.Exec(ctx, `
			INSERT INTO user_memory_profiles (workspace_id, user_id, profile, updated_at, next_review_at)
			VALUES ($1, $2, $3::jsonb, $4, $5)
			ON CONFLICT (workspace_id, user_id) DO NOTHING
		`, workspaceID, userID, string(profileJSON), profile.UpdatedAt, profile.NextReviewAt)
	} else {
		tag, execErr = s.pool.Exec(ctx, `
			UPDATE user_memory_profiles
			SET profile = $3::jsonb,
				updated_at = $4,
				next_review_at = $5
			WHERE workspace_id = $1
				AND user_id = $2
				AND COALESCE((profile->>'version')::bigint, 0) = $6
		`, workspaceID, userID, string(profileJSON), profile.UpdatedAt, profile.NextReviewAt, expectedVersion)
	}
	if execErr != nil {
		return execErr
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		err := s.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM user_memory_profiles
				WHERE workspace_id = $1 AND user_id = $2
			)
		`, workspaceID, userID).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			return ErrProfileNotFound
		}
		return ErrStaleProfile
	}
	return nil
}

func (s *PostgresStore) ReplaceProfileAndMarkCompletionIfVersion(ctx context.Context, workspaceID, userID string, profile Profile, expectedVersion int64, completion ProcessedCompletion) error {
	profile.Version = expectedVersion + 1
	profile.UpdatedAt = profile.UpdatedAt.UTC()
	profile.NextReviewAt = profile.NextReviewAt.UTC()
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("encode memory profile: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	markerTag, err := tx.Exec(ctx, `
		INSERT INTO memory_profile_completions (
			workspace_id, user_id, submission_id, session_id, problem_id,
			source_event_id, refresh_id, profile_version, processed_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (workspace_id, user_id, session_id, problem_id, submission_id) DO NOTHING
	`, workspaceID, userID, completion.SubmissionID, completion.SessionID, completion.ProblemID,
		completion.SourceEventID, completion.RefreshID, completion.ProfileVersion, completion.ProcessedAt.UTC())
	if err != nil {
		return err
	}
	if markerTag.RowsAffected() == 0 {
		return ErrCompletionAlreadyProcessed
	}

	var profileTag pgconn.CommandTag
	if expectedVersion == 0 {
		profileTag, err = tx.Exec(ctx, `
			INSERT INTO user_memory_profiles (workspace_id, user_id, profile, updated_at, next_review_at)
			VALUES ($1, $2, $3::jsonb, $4, $5)
			ON CONFLICT (workspace_id, user_id) DO NOTHING
		`, workspaceID, userID, string(profileJSON), profile.UpdatedAt, profile.NextReviewAt)
	} else {
		profileTag, err = tx.Exec(ctx, `
			UPDATE user_memory_profiles
			SET profile = $3::jsonb, updated_at = $4, next_review_at = $5
			WHERE workspace_id = $1 AND user_id = $2
				AND COALESCE((profile->>'version')::bigint, 0) = $6
		`, workspaceID, userID, string(profileJSON), profile.UpdatedAt, profile.NextReviewAt, expectedVersion)
	}
	if err != nil {
		return err
	}
	if profileTag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM user_memory_profiles WHERE workspace_id=$1 AND user_id=$2
		)`, workspaceID, userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrProfileNotFound
		}
		return ErrStaleProfile
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) IsCompletionProcessed(ctx context.Context, workspaceID, userID, sessionID, problemID, submissionID string) (bool, error) {
	var processed bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM memory_profile_completions
		WHERE workspace_id=$1 AND user_id=$2 AND session_id=$3 AND problem_id=$4 AND submission_id=$5
	)`, workspaceID, userID, sessionID, problemID, submissionID).Scan(&processed)
	return processed, err
}

func (s *PostgresStore) AppendEvent(ctx context.Context, event Event) error {
	payload := "null"
	if len(event.Payload) > 0 {
		if !json.Valid(event.Payload) {
			return errors.New("memory event payload must be valid JSON")
		}
		payload = string(event.Payload)
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO memory_events (
			id,
			workspace_id,
			user_id,
			source,
			type,
			summary,
			payload,
			occurred_at,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9)
	`, event.ID, event.WorkspaceID, event.UserID, event.Source, event.Type, event.Summary, payload, event.OccurredAt.UTC(), event.CreatedAt.UTC())
	return err
}

func (s *PostgresStore) ListEvents(ctx context.Context, workspaceID, userID string) ([]Event, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id,
			workspace_id,
			user_id,
			source,
			type,
			summary,
			COALESCE(payload, 'null'::jsonb)::text,
			occurred_at,
			created_at
		FROM memory_events
		WHERE workspace_id = $1 AND user_id = $2
		ORDER BY created_at ASC
	`, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		var event Event
		var payload string
		if err := rows.Scan(
			&event.ID,
			&event.WorkspaceID,
			&event.UserID,
			&event.Source,
			&event.Type,
			&event.Summary,
			&payload,
			&event.OccurredAt,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		if payload != "null" {
			event.Payload = json.RawMessage(payload)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *PostgresStore) ListEventScopes(ctx context.Context) ([]Scope, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT workspace_id, user_id
		FROM memory_events
		GROUP BY workspace_id, user_id
		ORDER BY workspace_id ASC, user_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scopes := []Scope{}
	for rows.Next() {
		var scope Scope
		if err := rows.Scan(&scope.WorkspaceID, &scope.UserID); err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return scopes, nil
}

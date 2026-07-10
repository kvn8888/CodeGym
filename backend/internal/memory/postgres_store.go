package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
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

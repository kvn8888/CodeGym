package usage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists GenAI usage in Postgres/Neon.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a Postgres-backed usage store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// EnsureSchema creates the genai_usage table if needed.
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS genai_usage (
			id text PRIMARY KEY,
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			provider text NOT NULL,
			model text NOT NULL DEFAULT '',
			kind text NOT NULL DEFAULT '',
			tokens_in integer NOT NULL DEFAULT 0,
			tokens_out integer NOT NULL DEFAULT 0,
			cost_usd_micros bigint NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create genai_usage: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_genai_usage_scope_created
		ON genai_usage (workspace_id, user_id, created_at DESC)`)
	if err != nil {
		return fmt.Errorf("index genai_usage: %w", err)
	}
	return nil
}

// Append inserts a usage record.
func (s *PostgresStore) Append(ctx context.Context, record Record) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO genai_usage (
			id, workspace_id, user_id, provider, model, kind,
			tokens_in, tokens_out, cost_usd_micros, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		record.ID,
		record.WorkspaceID,
		record.UserID,
		record.Provider,
		record.Model,
		record.Kind,
		record.TokensIn,
		record.TokensOut,
		record.CostUSDMicros,
		record.CreatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert genai_usage: %w", err)
	}
	return nil
}

// List returns usage for a workspace user scope.
func (s *PostgresStore) List(ctx context.Context, workspaceID, userID string) ([]Record, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, workspace_id, user_id, provider, model, kind,
		       tokens_in, tokens_out, cost_usd_micros, created_at
		FROM genai_usage
		WHERE workspace_id = $1 AND user_id = $2
		ORDER BY created_at ASC`, workspaceID, userID)
	if err != nil {
		return nil, fmt.Errorf("list genai_usage: %w", err)
	}
	defer rows.Close()

	out := make([]Record, 0)
	for rows.Next() {
		var record Record
		var created time.Time
		if err := rows.Scan(
			&record.ID,
			&record.WorkspaceID,
			&record.UserID,
			&record.Provider,
			&record.Model,
			&record.Kind,
			&record.TokensIn,
			&record.TokensOut,
			&record.CostUSDMicros,
			&created,
		); err != nil {
			return nil, fmt.Errorf("scan genai_usage: %w", err)
		}
		record.CreatedAt = created.UTC()
		out = append(out, record)
	}
	return out, rows.Err()
}

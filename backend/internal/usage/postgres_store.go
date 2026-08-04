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
			operation_id text NOT NULL DEFAULT '',
			provider text NOT NULL,
			model text NOT NULL DEFAULT '',
			kind text NOT NULL DEFAULT '',
			total_tokens integer NOT NULL DEFAULT 0,
			tokens_in integer NOT NULL DEFAULT 0,
			tokens_out integer NOT NULL DEFAULT 0,
			reasoning_tokens integer NOT NULL DEFAULT 0,
			cache_read_tokens integer NOT NULL DEFAULT 0,
			cache_write_tokens integer NOT NULL DEFAULT 0,
			cost_usd_micros bigint NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create genai_usage: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		ALTER TABLE genai_usage
			ADD COLUMN IF NOT EXISTS operation_id text NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS total_tokens integer NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS reasoning_tokens integer NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS cache_read_tokens integer NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS cache_write_tokens integer NOT NULL DEFAULT 0`)
	if err != nil {
		return fmt.Errorf("extend genai_usage token categories: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_genai_usage_scope_created
		ON genai_usage (workspace_id, user_id, created_at DESC)`)
	if err != nil {
		return fmt.Errorf("index genai_usage: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_genai_usage_operation
		ON genai_usage (workspace_id, user_id, operation_id, created_at ASC)
		WHERE operation_id <> ''`)
	if err != nil {
		return fmt.Errorf("index genai_usage operation: %w", err)
	}
	return nil
}

// Append inserts a usage record.
func (s *PostgresStore) Append(ctx context.Context, record Record) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO genai_usage (
			id, workspace_id, user_id, operation_id, provider, model, kind,
			total_tokens, tokens_in, tokens_out, reasoning_tokens,
			cache_read_tokens, cache_write_tokens, cost_usd_micros, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		record.ID,
		record.WorkspaceID,
		record.UserID,
		record.OperationID,
		record.Provider,
		record.Model,
		record.Kind,
		record.TotalTokens,
		record.TokensIn,
		record.TokensOut,
		record.ReasoningTokens,
		record.CacheReadTokens,
		record.CacheWriteTokens,
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
		SELECT id, workspace_id, user_id, operation_id, provider, model, kind,
		       total_tokens, tokens_in, tokens_out, reasoning_tokens,
		       cache_read_tokens, cache_write_tokens, cost_usd_micros, created_at
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
			&record.OperationID,
			&record.Provider,
			&record.Model,
			&record.Kind,
			&record.TotalTokens,
			&record.TokensIn,
			&record.TokensOut,
			&record.ReasoningTokens,
			&record.CacheReadTokens,
			&record.CacheWriteTokens,
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

package agentrelay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists relay operation budgets in Postgres/Neon.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// EnsureSchema idempotently creates relay state and guards constraints so the
// schema can be applied to both new and existing databases.
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS agent_relay_operation_budgets (
			id text PRIMARY KEY,
			operation_id text NOT NULL,
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			max_total_tokens bigint NOT NULL,
			max_cost_usd_micros bigint NOT NULL,
			deadline timestamptz NOT NULL,
			used_total_tokens bigint NOT NULL DEFAULT 0,
			input_tokens bigint NOT NULL DEFAULT 0,
			output_tokens bigint NOT NULL DEFAULT 0,
			reasoning_tokens bigint NOT NULL DEFAULT 0,
			cache_read_tokens bigint NOT NULL DEFAULT 0,
			cache_write_tokens bigint NOT NULL DEFAULT 0,
			used_cost_usd_micros bigint NOT NULL DEFAULT 0,
			revoked_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_relay_budget_operation
			ON agent_relay_operation_budgets (operation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_relay_budget_scope
			ON agent_relay_operation_budgets (workspace_id, user_id, operation_id)`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'fk_agent_relay_budget_membership'
					AND conrelid = 'agent_relay_operation_budgets'::regclass
			) THEN
				ALTER TABLE agent_relay_operation_budgets
					ADD CONSTRAINT fk_agent_relay_budget_membership
					FOREIGN KEY (workspace_id, user_id)
					REFERENCES workspace_memberships (workspace_id, user_id)
					ON DELETE CASCADE;
			END IF;
		END $$`,
		`DO $$
		BEGIN
			IF to_regclass('public.workflow_operations') IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'fk_agent_relay_budget_operation'
					AND conrelid = 'agent_relay_operation_budgets'::regclass
			) THEN
				ALTER TABLE agent_relay_operation_budgets
					ADD CONSTRAINT fk_agent_relay_budget_operation
					FOREIGN KEY (operation_id)
					REFERENCES workflow_operations (id)
					ON DELETE CASCADE;
			END IF;
		END $$`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'chk_agent_relay_budget_nonnegative'
					AND conrelid = 'agent_relay_operation_budgets'::regclass
			) THEN
				ALTER TABLE agent_relay_operation_budgets
					ADD CONSTRAINT chk_agent_relay_budget_nonnegative CHECK (
						max_total_tokens > 0 AND max_cost_usd_micros > 0
						AND used_total_tokens >= 0 AND input_tokens >= 0
						AND output_tokens >= 0 AND reasoning_tokens >= 0
						AND cache_read_tokens >= 0 AND cache_write_tokens >= 0
						AND used_cost_usd_micros >= 0
					);
			END IF;
		END $$`,
	}
	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("ensure agent relay schema: %w", err)
		}
	}
	return nil
}

func (s *PostgresStore) Create(ctx context.Context, budget OperationBudget) (OperationBudget, error) {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO agent_relay_operation_budgets (
			id, operation_id, workspace_id, user_id,
			max_total_tokens, max_cost_usd_micros, deadline,
			used_total_tokens, input_tokens, output_tokens, reasoning_tokens,
			cache_read_tokens, cache_write_tokens, used_cost_usd_micros,
			revoked_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		budget.ID, budget.OperationID, budget.WorkspaceID, budget.UserID,
		budget.MaxTotalTokens, budget.MaxCostUSDMicros, budget.Deadline.UTC(),
		budget.UsedTotalTokens, budget.InputTokens, budget.OutputTokens,
		budget.ReasoningTokens, budget.CacheReadTokens, budget.CacheWriteTokens,
		budget.UsedCostUSDMicros, nullableTime(budget.RevokedAt),
		budget.CreatedAt.UTC(), budget.UpdatedAt.UTC(),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return OperationBudget{}, ErrConflict
		}
		return OperationBudget{}, fmt.Errorf("create agent relay budget: %w", err)
	}
	return s.Get(ctx, budget.WorkspaceID, budget.UserID, budget.OperationID)
}

func (s *PostgresStore) Get(ctx context.Context, workspaceID, userID, operationID string) (OperationBudget, error) {
	budget, err := scanBudget(s.pool.QueryRow(ctx, `
		SELECT id, operation_id, workspace_id, user_id,
			max_total_tokens, max_cost_usd_micros, deadline,
			used_total_tokens, input_tokens, output_tokens, reasoning_tokens,
			cache_read_tokens, cache_write_tokens, used_cost_usd_micros,
			revoked_at, created_at, updated_at
		FROM agent_relay_operation_budgets
		WHERE operation_id = $1 AND workspace_id = $2 AND user_id = $3`,
		operationID, workspaceID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return OperationBudget{}, ErrNotFound
	}
	if err != nil {
		return OperationBudget{}, fmt.Errorf("get agent relay budget: %w", err)
	}
	return budget, nil
}

func (s *PostgresStore) Revoke(ctx context.Context, workspaceID, userID, operationID string, at time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE agent_relay_operation_budgets
		SET revoked_at = COALESCE(revoked_at, $4), updated_at = $4
		WHERE operation_id = $1 AND workspace_id = $2 AND user_id = $3`,
		operationID, workspaceID, userID, at.UTC())
	if err != nil {
		return fmt.Errorf("revoke agent relay budget: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) AddUsage(
	ctx context.Context,
	workspaceID, userID, operationID string,
	delta UsageDelta,
	at time.Time,
) (OperationBudget, error) {
	budget, err := scanBudget(s.pool.QueryRow(ctx, `
		UPDATE agent_relay_operation_budgets
		SET used_total_tokens = used_total_tokens + $4,
			input_tokens = input_tokens + $5,
			output_tokens = output_tokens + $6,
			reasoning_tokens = reasoning_tokens + $7,
			cache_read_tokens = cache_read_tokens + $8,
			cache_write_tokens = cache_write_tokens + $9,
			used_cost_usd_micros = used_cost_usd_micros + $10,
			updated_at = $11
		WHERE operation_id = $1 AND workspace_id = $2 AND user_id = $3
		RETURNING id, operation_id, workspace_id, user_id,
			max_total_tokens, max_cost_usd_micros, deadline,
			used_total_tokens, input_tokens, output_tokens, reasoning_tokens,
			cache_read_tokens, cache_write_tokens, used_cost_usd_micros,
			revoked_at, created_at, updated_at`,
		operationID, workspaceID, userID, delta.TotalTokens, delta.InputTokens,
		delta.OutputTokens, delta.ReasoningTokens, delta.CacheReadTokens,
		delta.CacheWriteTokens, delta.CostUSDMicros, at.UTC()))
	if errors.Is(err, pgx.ErrNoRows) {
		return OperationBudget{}, ErrNotFound
	}
	if err != nil {
		return OperationBudget{}, fmt.Errorf("add agent relay usage: %w", err)
	}
	return budget, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBudget(row rowScanner) (OperationBudget, error) {
	var budget OperationBudget
	err := row.Scan(
		&budget.ID, &budget.OperationID, &budget.WorkspaceID, &budget.UserID,
		&budget.MaxTotalTokens, &budget.MaxCostUSDMicros, &budget.Deadline,
		&budget.UsedTotalTokens, &budget.InputTokens, &budget.OutputTokens,
		&budget.ReasoningTokens, &budget.CacheReadTokens, &budget.CacheWriteTokens,
		&budget.UsedCostUSDMicros, &budget.RevokedAt, &budget.CreatedAt,
		&budget.UpdatedAt,
	)
	return budget, err
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

package execution

import (
	"context"
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
		`CREATE TABLE IF NOT EXISTS execution_runs (
			id text PRIMARY KEY,
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			problem_id text NOT NULL DEFAULT '',
			language text NOT NULL,
			entrypoint text NOT NULL,
			files jsonb NOT NULL,
			status text NOT NULL,
			exit_code int,
			output text NOT NULL DEFAULT '',
			error text NOT NULL DEFAULT '',
			duration_ms bigint NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL,
			completed_at timestamptz
		)`,
		// Legacy shim: the pre-rebase Daytona spike created execution_runs with a
		// tenant_id column and a tenant_memberships FK on shared dev databases,
		// before the tenant→workspace rename. Migrate that column once so the
		// workspace_id index/FK below apply cleanly. No-op on fresh databases.
		`DO $$
		BEGIN
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'execution_runs' AND column_name = 'tenant_id'
			) AND NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'execution_runs' AND column_name = 'workspace_id'
			) THEN
				ALTER TABLE execution_runs RENAME COLUMN tenant_id TO workspace_id;
				ALTER TABLE execution_runs DROP CONSTRAINT IF EXISTS fk_execution_runs_membership;
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_execution_runs_scope_created_at ON execution_runs (workspace_id, user_id, created_at DESC)`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'fk_execution_runs_membership'
					AND conrelid = 'execution_runs'::regclass
			) THEN
				ALTER TABLE execution_runs
				ADD CONSTRAINT fk_execution_runs_membership
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
				WHERE conname = 'chk_execution_runs_status'
					AND conrelid = 'execution_runs'::regclass
			) THEN
				ALTER TABLE execution_runs
				ADD CONSTRAINT chk_execution_runs_status
				CHECK (status IN ('queued', 'running', 'passed', 'failed', 'error'));
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

func (s *PostgresStore) CreateRun(ctx context.Context, run Run) error {
	filesJSON, err := json.Marshal(run.Files)
	if err != nil {
		return fmt.Errorf("encode execution files: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO execution_runs (
			id,
			workspace_id,
			user_id,
			problem_id,
			language,
			entrypoint,
			files,
			status,
			exit_code,
			output,
			error,
			duration_ms,
			created_at,
			completed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $13, $14)
	`, run.ID, run.WorkspaceID, run.UserID, run.ProblemID, run.Language, run.Entrypoint,
		string(filesJSON), string(run.Status), run.ExitCode, run.Output, run.Error,
		run.DurationMs, run.CreatedAt.UTC(), completedAtUTC(run.CompletedAt))
	return err
}

func (s *PostgresStore) UpdateRun(ctx context.Context, run Run) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE execution_runs
		SET status = $4,
			exit_code = $5,
			output = $6,
			error = $7,
			duration_ms = $8,
			completed_at = $9
		WHERE id = $1 AND workspace_id = $2 AND user_id = $3
	`, run.ID, run.WorkspaceID, run.UserID, string(run.Status), run.ExitCode,
		run.Output, run.Error, run.DurationMs, completedAtUTC(run.CompletedAt))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrRunNotFound
	}
	return nil
}

func (s *PostgresStore) GetRun(ctx context.Context, workspaceID, userID, id string) (Run, error) {
	row := s.pool.QueryRow(ctx, selectRunColumns+`
		FROM execution_runs
		WHERE workspace_id = $1 AND user_id = $2 AND id = $3
	`, workspaceID, userID, id)

	run, err := scanRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrRunNotFound
	}
	return run, err
}

func (s *PostgresStore) ListRuns(ctx context.Context, workspaceID, userID string) ([]Run, error) {
	rows, err := s.pool.Query(ctx, selectRunColumns+`
		FROM execution_runs
		WHERE workspace_id = $1 AND user_id = $2
		ORDER BY created_at DESC
	`, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

const selectRunColumns = `
	SELECT
		id,
		workspace_id,
		user_id,
		problem_id,
		language,
		entrypoint,
		files::text,
		status,
		exit_code,
		output,
		error,
		duration_ms,
		created_at,
		completed_at
`

func scanRun(row pgx.Row) (Run, error) {
	var run Run
	var filesJSON string
	var status string
	if err := row.Scan(
		&run.ID,
		&run.WorkspaceID,
		&run.UserID,
		&run.ProblemID,
		&run.Language,
		&run.Entrypoint,
		&filesJSON,
		&status,
		&run.ExitCode,
		&run.Output,
		&run.Error,
		&run.DurationMs,
		&run.CreatedAt,
		&run.CompletedAt,
	); err != nil {
		return Run{}, err
	}
	run.Status = Status(status)
	if err := json.Unmarshal([]byte(filesJSON), &run.Files); err != nil {
		return Run{}, fmt.Errorf("decode execution files: %w", err)
	}
	return run, nil
}

func completedAtUTC(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

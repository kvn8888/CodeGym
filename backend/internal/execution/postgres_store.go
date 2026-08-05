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
				mode text NOT NULL DEFAULT 'submit',
				language text NOT NULL,
			entrypoint text NOT NULL,
			files jsonb NOT NULL,
			status text NOT NULL,
			exit_code int,
			output text NOT NULL DEFAULT '',
			judge_result jsonb,
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
		`ALTER TABLE execution_runs ADD COLUMN IF NOT EXISTS judge_result jsonb`,
		`ALTER TABLE execution_runs ADD COLUMN IF NOT EXISTS mode text NOT NULL DEFAULT 'submit'`,
		`CREATE INDEX IF NOT EXISTS idx_execution_runs_scope_created_at ON execution_runs (workspace_id, user_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS execution_run_timings (
			id text PRIMARY KEY,
			run_id text NOT NULL,
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			snapshot text NOT NULL DEFAULT '',
			language text NOT NULL,
			strategy text NOT NULL,
			create_retried boolean NOT NULL DEFAULT false,
			create_ms bigint NOT NULL DEFAULT 0,
			upload_ms bigint NOT NULL DEFAULT 0,
			exec_ms bigint NOT NULL DEFAULT 0,
			total_ms bigint NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_run_timings_scope_created_at
			ON execution_run_timings (workspace_id, user_id, created_at DESC)`,
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
				WHERE conname = 'fk_execution_run_timings_membership'
					AND conrelid = 'execution_run_timings'::regclass
			) THEN
				ALTER TABLE execution_run_timings
					ADD CONSTRAINT fk_execution_run_timings_membership
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
				WHERE conname = 'fk_execution_run_timings_run'
					AND conrelid = 'execution_run_timings'::regclass
			) THEN
				ALTER TABLE execution_run_timings
					ADD CONSTRAINT fk_execution_run_timings_run
					FOREIGN KEY (run_id)
					REFERENCES execution_runs (id)
					ON DELETE CASCADE
					NOT VALID;
			END IF;
		END $$`,
		`DO $$
		DECLARE
			definition text;
		BEGIN
			SELECT pg_get_constraintdef(oid)
			INTO definition
			FROM pg_constraint
			WHERE conname = 'chk_execution_runs_status'
				AND conrelid = 'execution_runs'::regclass;

			IF definition IS NULL OR definition NOT LIKE '%out_of_memory%' THEN
				ALTER TABLE execution_runs DROP CONSTRAINT IF EXISTS chk_execution_runs_status;
				ALTER TABLE execution_runs
					ADD CONSTRAINT chk_execution_runs_status
					CHECK (status IN ('queued', 'running', 'passed', 'failed', 'timeout', 'out_of_memory', 'crashed', 'error'));
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
	judgeResultJSON, err := encodeJudgeResult(run.JudgeResult)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO execution_runs (
			id,
			workspace_id,
			user_id,
				problem_id,
				mode,
				language,
			entrypoint,
			files,
			status,
			exit_code,
			output,
			judge_result,
			error,
			duration_ms,
			created_at,
			completed_at
		)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11, $12::jsonb, $13, $14, $15, $16)
		`, run.ID, run.WorkspaceID, run.UserID, run.ProblemID, run.Mode, run.Language, run.Entrypoint,
		string(filesJSON), string(run.Status), run.ExitCode, run.Output, judgeResultJSON,
		run.Error, run.DurationMs, run.CreatedAt.UTC(), completedAtUTC(run.CompletedAt))
	return err
}

func (s *PostgresStore) UpdateRun(ctx context.Context, run Run) error {
	judgeResultJSON, err := encodeJudgeResult(run.JudgeResult)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE execution_runs
		SET status = $4,
			exit_code = $5,
			output = $6,
			judge_result = $7::jsonb,
			error = $8,
			duration_ms = $9,
			completed_at = $10
		WHERE id = $1 AND workspace_id = $2 AND user_id = $3
	`, run.ID, run.WorkspaceID, run.UserID, string(run.Status), run.ExitCode,
		run.Output, judgeResultJSON, run.Error, run.DurationMs, completedAtUTC(run.CompletedAt))
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

func (s *PostgresStore) AppendRunTiming(ctx context.Context, timing RunTiming) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO execution_run_timings (
			id, run_id, workspace_id, user_id, snapshot, language, strategy,
			create_retried, create_ms, upload_ms, exec_ms, total_ms, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		timing.ID,
		timing.RunID,
		timing.WorkspaceID,
		timing.UserID,
		timing.Snapshot,
		timing.Language,
		string(timing.Strategy),
		timing.CreateRetried,
		timing.CreateMs,
		timing.UploadMs,
		timing.ExecMs,
		timing.TotalMs,
		timing.CreatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert execution_run_timings: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListRunTimings(ctx context.Context, workspaceID, userID string) ([]RunTiming, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, run_id, workspace_id, user_id, snapshot, language, strategy,
		       create_retried, create_ms, upload_ms, exec_ms, total_ms, created_at
		FROM execution_run_timings
		WHERE workspace_id = $1 AND user_id = $2
		ORDER BY created_at ASC`, workspaceID, userID)
	if err != nil {
		return nil, fmt.Errorf("list execution_run_timings: %w", err)
	}
	defer rows.Close()

	timings := make([]RunTiming, 0)
	for rows.Next() {
		var timing RunTiming
		var strategy string
		if err := rows.Scan(
			&timing.ID,
			&timing.RunID,
			&timing.WorkspaceID,
			&timing.UserID,
			&timing.Snapshot,
			&timing.Language,
			&strategy,
			&timing.CreateRetried,
			&timing.CreateMs,
			&timing.UploadMs,
			&timing.ExecMs,
			&timing.TotalMs,
			&timing.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan execution_run_timings: %w", err)
		}
		timing.Strategy = TestStrategy(strategy)
		timing.CreatedAt = timing.CreatedAt.UTC()
		timings = append(timings, timing)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate execution_run_timings: %w", err)
	}
	return timings, nil
}

const selectRunColumns = `
	SELECT
		id,
		workspace_id,
		user_id,
			problem_id,
			mode,
			language,
		entrypoint,
		files::text,
		status,
		exit_code,
		output,
		judge_result::text,
		error,
		duration_ms,
		created_at,
		completed_at
`

func scanRun(row pgx.Row) (Run, error) {
	var run Run
	var filesJSON string
	var judgeResultJSON *string
	var status string
	if err := row.Scan(
		&run.ID,
		&run.WorkspaceID,
		&run.UserID,
		&run.ProblemID,
		&run.Mode,
		&run.Language,
		&run.Entrypoint,
		&filesJSON,
		&status,
		&run.ExitCode,
		&run.Output,
		&judgeResultJSON,
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
	if judgeResultJSON != nil {
		var judgeResult JudgeResult
		if err := json.Unmarshal([]byte(*judgeResultJSON), &judgeResult); err != nil {
			return Run{}, fmt.Errorf("decode judge result: %w", err)
		}
		run.JudgeResult = &judgeResult
	}
	return run, nil
}

func encodeJudgeResult(result *JudgeResult) (*string, error) {
	if result == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode judge result: %w", err)
	}
	value := string(encoded)
	return &value, nil
}

func completedAtUTC(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

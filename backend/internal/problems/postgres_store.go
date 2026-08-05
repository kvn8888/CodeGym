package problems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
		`CREATE TABLE IF NOT EXISTS problems (
			id text PRIMARY KEY,
				public_spec jsonb NOT NULL,
				skeleton_files jsonb NOT NULL,
				public_test_files jsonb NOT NULL DEFAULT '[]'::jsonb,
				hidden_test_files jsonb NOT NULL,
			reference_solution text NOT NULL,
			entrypoint text NOT NULL,
			visibility text NOT NULL DEFAULT 'global',
			workspace_id text,
			user_id text,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`ALTER TABLE problems ADD COLUMN IF NOT EXISTS visibility text NOT NULL DEFAULT 'global'`,
		`ALTER TABLE problems ADD COLUMN IF NOT EXISTS workspace_id text`,
		`ALTER TABLE problems ADD COLUMN IF NOT EXISTS user_id text`,
		`ALTER TABLE problems ADD COLUMN IF NOT EXISTS public_test_files jsonb NOT NULL DEFAULT '[]'::jsonb`,
		`UPDATE problems SET visibility = 'global' WHERE visibility IS NULL OR visibility = ''`,
		`CREATE INDEX IF NOT EXISTS idx_problems_scope ON problems (workspace_id, user_id) WHERE visibility = 'workspace'`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'chk_problems_visibility'
				  AND conrelid = 'problems'::regclass
			) THEN
				ALTER TABLE problems
					ADD CONSTRAINT chk_problems_visibility
					CHECK (
						visibility = 'global'
						OR (visibility = 'workspace' AND workspace_id IS NOT NULL AND user_id IS NOT NULL)
					);
			END IF;
		END $$`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'fk_problems_workspace_owner'
				  AND conrelid = 'problems'::regclass
			) THEN
				ALTER TABLE problems
					ADD CONSTRAINT fk_problems_workspace_owner
					FOREIGN KEY (workspace_id, user_id)
					REFERENCES workspace_memberships (workspace_id, user_id)
					ON DELETE CASCADE;
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_problems_title ON problems ((public_spec->>'title'))`,
	}
	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) Upsert(ctx context.Context, definition Definition) error {
	publicSpec, err := json.Marshal(definition.Problem)
	if err != nil {
		return fmt.Errorf("encode problem public spec: %w", err)
	}
	skeletonFiles, err := json.Marshal(definition.SkeletonFiles)
	if err != nil {
		return fmt.Errorf("encode problem skeleton files: %w", err)
	}
	publicTestFiles, err := json.Marshal(definition.PublicTestFiles)
	if err != nil {
		return fmt.Errorf("encode problem public test files: %w", err)
	}
	hiddenFiles, err := json.Marshal(definition.HiddenTestFiles)
	if err != nil {
		return fmt.Errorf("encode problem hidden files: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO problems (
			id,
			public_spec,
				skeleton_files,
				public_test_files,
				hidden_test_files,
			reference_solution,
			entrypoint,
			visibility,
			workspace_id,
			user_id,
			updated_at
		)
			VALUES ($1, $2::jsonb, $3::jsonb, $4::jsonb, $5::jsonb, $6, $7, $8, $9, $10, now())
		ON CONFLICT (id) DO UPDATE
			SET public_spec = EXCLUDED.public_spec,
				skeleton_files = EXCLUDED.skeleton_files,
				public_test_files = EXCLUDED.public_test_files,
				hidden_test_files = EXCLUDED.hidden_test_files,
			reference_solution = EXCLUDED.reference_solution,
			entrypoint = EXCLUDED.entrypoint,
			visibility = EXCLUDED.visibility,
			workspace_id = EXCLUDED.workspace_id,
			user_id = EXCLUDED.user_id,
			updated_at = now()
		`, definition.ID, string(publicSpec), string(skeletonFiles), string(publicTestFiles), string(hiddenFiles), definition.ReferenceSolution, definition.Entrypoint, normalizeVisibility(definition.Visibility), nullableString(definition.WorkspaceID), nullableString(definition.UserID))
	return err
}

func (s *PostgresStore) Get(ctx context.Context, id, workspaceID, userID string) (Definition, error) {
	var publicSpec []byte
	var skeletonFiles []byte
	var publicTestFiles []byte
	var hiddenFiles []byte
	var definition Definition
	err := s.pool.QueryRow(ctx, `
		SELECT
			public_spec,
				skeleton_files,
				public_test_files,
				hidden_test_files,
			reference_solution,
			entrypoint,
			visibility,
			COALESCE(workspace_id, ''),
			COALESCE(user_id, '')
		FROM problems
		WHERE id = $1
		  AND (visibility = 'global' OR (visibility = 'workspace' AND workspace_id = $2 AND user_id = $3))
	`, id, workspaceID, userID).Scan(
		&publicSpec,
		&skeletonFiles,
		&publicTestFiles,
		&hiddenFiles,
		&definition.ReferenceSolution,
		&definition.Entrypoint,
		&definition.Visibility,
		&definition.WorkspaceID,
		&definition.UserID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Definition{}, ErrNotFound
	}
	if err != nil {
		return Definition{}, err
	}
	if err := json.Unmarshal(publicSpec, &definition.Problem); err != nil {
		return Definition{}, fmt.Errorf("decode problem public spec: %w", err)
	}
	if err := json.Unmarshal(skeletonFiles, &definition.SkeletonFiles); err != nil {
		return Definition{}, fmt.Errorf("decode problem skeleton files: %w", err)
	}
	if err := json.Unmarshal(publicTestFiles, &definition.PublicTestFiles); err != nil {
		return Definition{}, fmt.Errorf("decode problem public test files: %w", err)
	}
	if err := json.Unmarshal(hiddenFiles, &definition.HiddenTestFiles); err != nil {
		return Definition{}, fmt.Errorf("decode problem hidden files: %w", err)
	}
	return definition, nil
}

func (s *PostgresStore) List(ctx context.Context, workspaceID, userID string) ([]Summary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT public_spec
		FROM problems
		WHERE visibility = 'global' OR (visibility = 'workspace' AND workspace_id = $1 AND user_id = $2)
		ORDER BY public_spec->>'title' ASC
	`, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := []Summary{}
	for rows.Next() {
		var publicSpec []byte
		if err := rows.Scan(&publicSpec); err != nil {
			return nil, err
		}
		var problem Problem
		if err := json.Unmarshal(publicSpec, &problem); err != nil {
			return nil, fmt.Errorf("decode problem public spec: %w", err)
		}
		summaries = append(summaries, problem.Summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return summaries, nil
}

func normalizeVisibility(value Visibility) Visibility {
	if value == VisibilityWorkspace {
		return value
	}
	return VisibilityGlobal
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

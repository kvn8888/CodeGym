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
			hidden_test_files jsonb NOT NULL,
			reference_solution text NOT NULL,
			entrypoint text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
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
	hiddenFiles, err := json.Marshal(definition.HiddenTestFiles)
	if err != nil {
		return fmt.Errorf("encode problem hidden files: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO problems (
			id,
			public_spec,
			skeleton_files,
			hidden_test_files,
			reference_solution,
			entrypoint,
			updated_at
		)
		VALUES ($1, $2::jsonb, $3::jsonb, $4::jsonb, $5, $6, now())
		ON CONFLICT (id) DO UPDATE
		SET public_spec = EXCLUDED.public_spec,
			skeleton_files = EXCLUDED.skeleton_files,
			hidden_test_files = EXCLUDED.hidden_test_files,
			reference_solution = EXCLUDED.reference_solution,
			entrypoint = EXCLUDED.entrypoint,
			updated_at = now()
	`, definition.ID, string(publicSpec), string(skeletonFiles), string(hiddenFiles), definition.ReferenceSolution, definition.Entrypoint)
	return err
}

func (s *PostgresStore) Get(ctx context.Context, id string) (Definition, error) {
	var publicSpec []byte
	var skeletonFiles []byte
	var hiddenFiles []byte
	var definition Definition
	err := s.pool.QueryRow(ctx, `
		SELECT
			public_spec,
			skeleton_files,
			hidden_test_files,
			reference_solution,
			entrypoint
		FROM problems
		WHERE id = $1
	`, id).Scan(
		&publicSpec,
		&skeletonFiles,
		&hiddenFiles,
		&definition.ReferenceSolution,
		&definition.Entrypoint,
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
	if err := json.Unmarshal(hiddenFiles, &definition.HiddenTestFiles); err != nil {
		return Definition{}, fmt.Errorf("decode problem hidden files: %w", err)
	}
	return definition, nil
}

func (s *PostgresStore) List(ctx context.Context) ([]Summary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT public_spec
		FROM problems
		ORDER BY public_spec->>'title' ASC
	`)
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

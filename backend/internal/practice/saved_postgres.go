package practice

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresSavedExerciseStore persists saved exercises durably. It follows
// the session/execution Postgres conventions: workspace-scoped rows,
// explicit not-found mapping, and JSON file blobs.
type PostgresSavedExerciseStore struct {
	pool *pgxpool.Pool
}

// NewPostgresSavedExerciseStore creates a durable saved-exercise store.
func NewPostgresSavedExerciseStore(pool *pgxpool.Pool) *PostgresSavedExerciseStore {
	return &PostgresSavedExerciseStore{pool: pool}
}

// EnsureSchema creates the durable saved-exercise table.
func (s *PostgresSavedExerciseStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS practice_saved_exercises (
			id text PRIMARY KEY,
			workspace_id text NOT NULL,
			user_id text NOT NULL,
			artifact_id text NOT NULL,
			artifact_version text NOT NULL,
			technology text NOT NULL DEFAULT '',
			environment_base text NOT NULL DEFAULT '',
			network_policy text NOT NULL DEFAULT '',
			snapshot_ref text NOT NULL,
			builder_run_id text NOT NULL DEFAULT '',
			verifier_run_id text NOT NULL DEFAULT '',
			files jsonb NOT NULL DEFAULT '{}'::jsonb,
			updated_at timestamptz NOT NULL DEFAULT now(),
			FOREIGN KEY (workspace_id, user_id)
				REFERENCES workspace_memberships (workspace_id, user_id)
				ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_practice_saved_scope_updated ON practice_saved_exercises (workspace_id, user_id, updated_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// SaveExercise inserts or replaces one saved record.
func (s *PostgresSavedExerciseStore) SaveExercise(ctx context.Context, saved SavedExercise) error {
	encoded, err := json.Marshal(saved.Files)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO practice_saved_exercises (
			id, workspace_id, user_id, artifact_id, artifact_version,
			technology, environment_base, network_policy,
			snapshot_ref, builder_run_id, verifier_run_id, files, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13)
		ON CONFLICT (id) DO UPDATE SET
			workspace_id = EXCLUDED.workspace_id,
			user_id = EXCLUDED.user_id,
			artifact_id = EXCLUDED.artifact_id,
			artifact_version = EXCLUDED.artifact_version,
			technology = EXCLUDED.technology,
			environment_base = EXCLUDED.environment_base,
			network_policy = EXCLUDED.network_policy,
			snapshot_ref = EXCLUDED.snapshot_ref,
			builder_run_id = EXCLUDED.builder_run_id,
			verifier_run_id = EXCLUDED.verifier_run_id,
			files = EXCLUDED.files,
			updated_at = EXCLUDED.updated_at
	`, saved.ID, saved.WorkspaceID, saved.Owner, saved.ArtifactID, saved.ArtifactVersion,
		saved.Technology, saved.EnvironmentBase, saved.NetworkPolicy,
		saved.SnapshotRef, saved.BuilderRunID, saved.VerifierRunID, string(encoded), saved.UpdatedAt.UTC())
	return err
}

// GetSavedExercise returns one saved record or ErrSavedNotFound.
func (s *PostgresSavedExerciseStore) GetSavedExercise(ctx context.Context, id string) (SavedExercise, error) {
	var saved SavedExercise
	var files string
	err := s.pool.QueryRow(ctx, `
		SELECT id, workspace_id, user_id, artifact_id, artifact_version,
			technology, environment_base, network_policy,
			snapshot_ref, builder_run_id, verifier_run_id, files::text, updated_at
		FROM practice_saved_exercises
		WHERE id = $1
	`, id).Scan(
		&saved.ID, &saved.WorkspaceID, &saved.Owner, &saved.ArtifactID, &saved.ArtifactVersion,
		&saved.Technology, &saved.EnvironmentBase, &saved.NetworkPolicy,
		&saved.SnapshotRef, &saved.BuilderRunID, &saved.VerifierRunID, &files, &saved.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SavedExercise{}, ErrSavedNotFound
	}
	if err != nil {
		return SavedExercise{}, err
	}
	saved.Files = map[string]string{}
	if err := json.Unmarshal([]byte(files), &saved.Files); err != nil {
		return SavedExercise{}, err
	}
	return saved, nil
}

// ListSavedExercises returns saved records for one owner/workspace pair in
// most-recently-updated order.
func (s *PostgresSavedExerciseStore) ListSavedExercises(ctx context.Context, owner, workspaceID string) ([]SavedExercise, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, workspace_id, user_id, artifact_id, artifact_version,
			technology, environment_base, network_policy,
			snapshot_ref, builder_run_id, verifier_run_id, files::text, updated_at
		FROM practice_saved_exercises
		WHERE workspace_id = $1 AND user_id = $2
		ORDER BY updated_at DESC
	`, workspaceID, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedExercise
	for rows.Next() {
		var saved SavedExercise
		var files string
		if err := rows.Scan(
			&saved.ID, &saved.WorkspaceID, &saved.Owner, &saved.ArtifactID, &saved.ArtifactVersion,
			&saved.Technology, &saved.EnvironmentBase, &saved.NetworkPolicy,
			&saved.SnapshotRef, &saved.BuilderRunID, &saved.VerifierRunID, &files, &saved.UpdatedAt,
		); err != nil {
			return nil, err
		}
		saved.Files = map[string]string{}
		if err := json.Unmarshal([]byte(files), &saved.Files); err != nil {
			return nil, err
		}
		out = append(out, saved)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

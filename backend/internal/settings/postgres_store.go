package settings

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists runtime settings in Postgres/Neon.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// EnsureSchema idempotently creates the workspace-scoped runtime settings
// table. Values remain text so readers can detect and default malformed rows.
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS runtime_settings (
			workspace_id text NOT NULL,
			key text NOT NULL,
			value_type text NOT NULL,
			value text NOT NULL,
			updated_by_user_id text NOT NULL,
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (workspace_id, key)
		)`)
	if err != nil {
		return fmt.Errorf("create runtime_settings: %w", err)
	}
	return nil
}

func (s *PostgresStore) Get(ctx context.Context, workspaceID, key string) (Setting, error) {
	var setting Setting
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT workspace_id, key, value_type, value, updated_by_user_id, updated_at
		FROM runtime_settings
		WHERE workspace_id = $1 AND key = $2`, workspaceID, key).Scan(
		&setting.WorkspaceID,
		&setting.Key,
		&setting.Type,
		&setting.RawValue,
		&setting.UpdatedByUserID,
		&updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Setting{}, ErrSettingNotFound
	}
	if err != nil {
		return Setting{}, fmt.Errorf("get runtime setting: %w", err)
	}
	setting.UpdatedAt = updatedAt.UTC()
	return setting, nil
}

func (s *PostgresStore) Upsert(ctx context.Context, setting Setting) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO runtime_settings (
			workspace_id, key, value_type, value, updated_by_user_id, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (workspace_id, key) DO UPDATE SET
			value_type = EXCLUDED.value_type,
			value = EXCLUDED.value,
			updated_by_user_id = EXCLUDED.updated_by_user_id,
			updated_at = EXCLUDED.updated_at`,
		setting.WorkspaceID,
		setting.Key,
		setting.Type,
		setting.RawValue,
		setting.UpdatedByUserID,
		setting.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert runtime setting: %w", err)
	}
	return nil
}

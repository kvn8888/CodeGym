package identity

import (
	"context"
	"strings"

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
		`CREATE TABLE IF NOT EXISTS app_users (
			id text PRIMARY KEY,
			display_name text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS tenants (
			id text PRIMARY KEY,
			name text NOT NULL,
			tenant_type text NOT NULL DEFAULT 'personal',
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_memberships (
			tenant_id text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
			user_id text NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
			role text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (tenant_id, user_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_memberships_user_id ON tenant_memberships (user_id)`,
	}

	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) EnsurePersonalTenant(ctx context.Context, tenant PersonalTenant) error {
	role := strings.TrimSpace(tenant.Role)
	if role == "" {
		role = "owner"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO app_users (id, display_name)
		VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE
		SET updated_at = now()
	`, tenant.UserID, tenant.UserID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO tenants (id, name, tenant_type)
		VALUES ($1, $2, 'personal')
		ON CONFLICT (id) DO UPDATE
		SET updated_at = now()
	`, tenant.TenantID, tenant.TenantID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO tenant_memberships (tenant_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, user_id) DO UPDATE
		SET role = EXCLUDED.role,
			updated_at = now()
	`, tenant.TenantID, tenant.UserID, role); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

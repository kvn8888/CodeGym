package identity

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists identity and tenant membership bootstrap data.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a Postgres-backed identity store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// EnsureSchema idempotently creates identity and tenancy tables.
func (s *PostgresStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS app_users (
			id text PRIMARY KEY,
			email text NOT NULL DEFAULT '',
			display_name text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`ALTER TABLE app_users ADD COLUMN IF NOT EXISTS email text NOT NULL DEFAULT ''`,
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
		`ALTER TABLE tenants ALTER COLUMN tenant_type SET DEFAULT 'personal'`,
		`ALTER TABLE tenant_memberships ALTER COLUMN role SET DEFAULT 'owner'`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'chk_tenants_tenant_type'
					AND conrelid = 'tenants'::regclass
			) THEN
				ALTER TABLE tenants
				ADD CONSTRAINT chk_tenants_tenant_type
				CHECK (tenant_type IN ('personal', 'team'));
			END IF;
		END $$`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'chk_tenant_memberships_role'
					AND conrelid = 'tenant_memberships'::regclass
			) THEN
				ALTER TABLE tenant_memberships
				ADD CONSTRAINT chk_tenant_memberships_role
				CHECK (role IN ('owner', 'admin', 'member'));
			END IF;
		END $$`,
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
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	metadataDisplayName := strings.TrimSpace(tenant.DisplayName)
	displayName := metadataDisplayName
	if displayName == "" {
		displayName = tenant.UserID
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO app_users (id, email, display_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE
		SET email = CASE
				WHEN EXCLUDED.email <> '' THEN EXCLUDED.email
				ELSE app_users.email
			END,
			display_name = CASE
				WHEN $4 THEN EXCLUDED.display_name
				ELSE app_users.display_name
			END,
			updated_at = now()
	`, tenant.UserID, strings.TrimSpace(tenant.Email), displayName, metadataDisplayName != ""); err != nil {
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

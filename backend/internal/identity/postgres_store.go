package identity

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
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
		`ALTER TABLE app_users ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT ''`,
		`ALTER TABLE app_users ADD COLUMN IF NOT EXISTS display_name_source text NOT NULL DEFAULT 'fallback'`,
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
				WHERE conname = 'chk_app_users_display_name_source'
					AND conrelid = 'app_users'::regclass
			) THEN
				ALTER TABLE app_users
				ADD CONSTRAINT chk_app_users_display_name_source
				CHECK (display_name_source IN ('oauth', 'user', 'fallback'));
			END IF;
		END $$`,
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
	displayNameSource := normalizeDisplayNameSource(tenant.DisplayNameSource)
	if displayName == "" {
		displayName = tenant.UserID
		displayNameSource = "fallback"
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO app_users (id, email, display_name, display_name_source)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET email = CASE
				WHEN EXCLUDED.email <> '' THEN EXCLUDED.email
				ELSE app_users.email
			END,
			display_name = CASE
				WHEN app_users.display_name_source = 'user' THEN app_users.display_name
				WHEN EXCLUDED.display_name_source = 'oauth' THEN EXCLUDED.display_name
				WHEN app_users.display_name = '' THEN EXCLUDED.display_name
				ELSE app_users.display_name
			END,
			display_name_source = CASE
				WHEN app_users.display_name_source = 'user' THEN app_users.display_name_source
				WHEN EXCLUDED.display_name_source = 'oauth' THEN EXCLUDED.display_name_source
				WHEN app_users.display_name_source = '' THEN EXCLUDED.display_name_source
				ELSE app_users.display_name_source
			END,
			updated_at = now()
	`, tenant.UserID, strings.TrimSpace(tenant.Email), displayName, displayNameSource); err != nil {
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

func (s *PostgresStore) GetUserProfile(ctx context.Context, userID string) (UserProfile, error) {
	var profile UserProfile
	err := s.pool.QueryRow(ctx, `
		SELECT
			u.id,
			u.email,
			u.display_name,
			u.display_name_source,
			COALESCE((
				SELECT m.tenant_id
				FROM tenant_memberships m
				JOIN tenants t ON t.id = m.tenant_id
				WHERE m.user_id = u.id
					AND t.tenant_type = 'personal'
				ORDER BY m.created_at ASC, m.tenant_id ASC
				LIMIT 1
			), '')
		FROM app_users u
		WHERE u.id = $1
	`, userID).Scan(
		&profile.UserID,
		&profile.Email,
		&profile.DisplayName,
		&profile.DisplayNameSource,
		&profile.DefaultTenantID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserProfile{}, ErrUserNotFound
	}
	if err != nil {
		return UserProfile{}, err
	}

	profile.DisplayNameSource = normalizeDisplayNameSource(profile.DisplayNameSource)
	return profile, nil
}

func (s *PostgresStore) UpdateDisplayName(ctx context.Context, userID, displayName string) (UserProfile, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE app_users
		SET display_name = $2,
			display_name_source = 'user',
			updated_at = now()
		WHERE id = $1
	`, userID, strings.TrimSpace(displayName))
	if err != nil {
		return UserProfile{}, err
	}
	if tag.RowsAffected() == 0 {
		return UserProfile{}, ErrUserNotFound
	}

	return s.GetUserProfile(ctx, userID)
}

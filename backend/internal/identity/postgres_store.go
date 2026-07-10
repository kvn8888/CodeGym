package identity

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists identity and workspace membership bootstrap data.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a Postgres-backed identity store.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// EnsureSchema idempotently creates identity and workspace tables.
// It also renames legacy tenant_* tables/columns from earlier schema versions.
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
		// Legacy rename: tenants / tenant_memberships → workspaces / workspace_memberships
		`DO $$
		BEGIN
			IF to_regclass('public.tenants') IS NOT NULL AND to_regclass('public.workspaces') IS NULL THEN
				ALTER TABLE tenants RENAME TO workspaces;
			END IF;
			IF to_regclass('public.tenant_memberships') IS NOT NULL AND to_regclass('public.workspace_memberships') IS NULL THEN
				ALTER TABLE tenant_memberships RENAME TO workspace_memberships;
			END IF;
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'workspaces' AND column_name = 'tenant_type'
			) THEN
				ALTER TABLE workspaces RENAME COLUMN tenant_type TO workspace_type;
			END IF;
			IF EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'workspace_memberships' AND column_name = 'tenant_id'
			) THEN
				ALTER TABLE workspace_memberships RENAME COLUMN tenant_id TO workspace_id;
			END IF;
			IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_tenants_tenant_type') THEN
				ALTER TABLE workspaces RENAME CONSTRAINT chk_tenants_tenant_type TO chk_workspaces_workspace_type;
			END IF;
			IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_tenant_memberships_role') THEN
				ALTER TABLE workspace_memberships RENAME CONSTRAINT chk_tenant_memberships_role TO chk_workspace_memberships_role;
			END IF;
			IF to_regclass('public.idx_tenant_memberships_user_id') IS NOT NULL THEN
				ALTER INDEX idx_tenant_memberships_user_id RENAME TO idx_workspace_memberships_user_id;
			END IF;
		END $$`,
		`CREATE TABLE IF NOT EXISTS workspaces (
			id text PRIMARY KEY,
			name text NOT NULL,
			workspace_type text NOT NULL DEFAULT 'personal',
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS workspace_memberships (
			workspace_id text NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
			user_id text NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
			role text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (workspace_id, user_id)
		)`,
		`ALTER TABLE workspaces ALTER COLUMN workspace_type SET DEFAULT 'personal'`,
		`ALTER TABLE workspace_memberships ALTER COLUMN role SET DEFAULT 'owner'`,
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
				WHERE conname = 'chk_workspaces_workspace_type'
					AND conrelid = 'workspaces'::regclass
			) THEN
				ALTER TABLE workspaces
				ADD CONSTRAINT chk_workspaces_workspace_type
				CHECK (workspace_type IN ('personal', 'team'));
			END IF;
		END $$`,
		`DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'chk_workspace_memberships_role'
					AND conrelid = 'workspace_memberships'::regclass
			) THEN
				ALTER TABLE workspace_memberships
				ADD CONSTRAINT chk_workspace_memberships_role
				CHECK (role IN ('owner', 'admin', 'member'));
			END IF;
		END $$`,
		`CREATE INDEX IF NOT EXISTS idx_workspace_memberships_user_id ON workspace_memberships (user_id)`,
	}

	for _, statement := range statements {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *PostgresStore) EnsurePersonalWorkspace(ctx context.Context, workspace PersonalWorkspace) error {
	role := strings.TrimSpace(workspace.Role)
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

	metadataDisplayName := strings.TrimSpace(workspace.DisplayName)
	displayName := metadataDisplayName
	displayNameSource := normalizeDisplayNameSource(workspace.DisplayNameSource)
	if displayName == "" {
		displayName = workspace.UserID
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
	`, workspace.UserID, strings.TrimSpace(workspace.Email), displayName, displayNameSource); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO workspaces (id, name, workspace_type)
		VALUES ($1, $2, 'personal')
		ON CONFLICT (id) DO UPDATE
		SET updated_at = now()
	`, workspace.WorkspaceID, workspace.WorkspaceID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO workspace_memberships (workspace_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id, user_id) DO UPDATE
		SET role = EXCLUDED.role,
			updated_at = now()
	`, workspace.WorkspaceID, workspace.UserID, role); err != nil {
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
				SELECT m.workspace_id
				FROM workspace_memberships m
				JOIN workspaces w ON w.id = m.workspace_id
				WHERE m.user_id = u.id
					AND w.workspace_type = 'personal'
				ORDER BY m.created_at ASC, m.workspace_id ASC
				LIMIT 1
			), '')
		FROM app_users u
		WHERE u.id = $1
	`, userID).Scan(
		&profile.UserID,
		&profile.Email,
		&profile.DisplayName,
		&profile.DisplayNameSource,
		&profile.DefaultWorkspaceID,
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

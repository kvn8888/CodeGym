package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/tenant"
)

func TestNeonIdentityAndMemoryBootstrap(t *testing.T) {
	databaseURL := os.Getenv("CODEGYM_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("NEON_CONNECTION_STRING")
	}
	if databaseURL == "" {
		t.Skip("set CODEGYM_TEST_DATABASE_URL or NEON_CONNECTION_STRING to run Neon integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("configure Postgres pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping Postgres: %v", err)
	}

	identityStore := identity.NewPostgresStore(pool)
	memoryStore := memory.NewPostgresStore(pool)
	if err := identityStore.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure identity schema: %v", err)
	}
	if err := memoryStore.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure memory schema: %v", err)
	}
	assertConstraints(t, ctx, pool)

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	userID := "test-user-" + suffix
	tenantID := "test-tenant-" + suffix
	defer cleanupRows(t, pool, tenantID, userID)

	identityService := identity.NewService(identityStore)
	principal := auth.Principal{
		UserID:          userID,
		DefaultTenantID: tenantID,
		TenantIDs:       []string{tenantID},
		UserMetadata: auth.UserMetadata{
			Email:       userID + "@example.com",
			DisplayName: "Test User " + suffix,
		},
	}
	if err := identityService.EnsurePersonalTenant(ctx, principal); err != nil {
		t.Fatalf("ensure personal tenant: %v", err)
	}
	assertIdentityRows(t, ctx, pool, tenantID, userID, principal.UserMetadata.Email, principal.UserMetadata.DisplayName)

	repeatedPrincipal := principal
	repeatedPrincipal.UserMetadata = auth.UserMetadata{}
	if err := identityService.EnsurePersonalTenant(ctx, repeatedPrincipal); err != nil {
		t.Fatalf("ensure personal tenant without metadata: %v", err)
	}
	assertIdentityRows(t, ctx, pool, tenantID, userID, principal.UserMetadata.Email, principal.UserMetadata.DisplayName)

	requestCtx := auth.WithPrincipal(ctx, principal)
	requestCtx = tenant.WithScope(requestCtx, tenant.Scope{TenantID: tenantID})

	memoryService := memory.NewService(memoryStore, func() time.Time {
		return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	})
	payload := json.RawMessage(`{"check":"neon-bootstrap"}`)
	event, err := memoryService.RecordEvent(requestCtx, memory.RecordEventInput{
		Source:  "system",
		Type:    "memory_api_checked",
		Summary: "Verified Neon identity and memory schema bootstrap.",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("record memory event: %v", err)
	}

	events, err := memoryService.ListEvents(requestCtx)
	if err != nil {
		t.Fatalf("list memory events: %v", err)
	}
	if !containsEvent(events, event.ID) {
		t.Fatalf("expected event %s in list, got %d events", event.ID, len(events))
	}

	profile, err := memoryService.GetProfile(requestCtx)
	if err != nil {
		t.Fatalf("get memory profile: %v", err)
	}
	if profile.Summary == "" {
		t.Fatal("expected default memory profile summary")
	}

	assertMembershipConstraint(t, ctx, pool)
}

func assertConstraints(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	for _, constraint := range []struct {
		name  string
		table string
	}{
		{name: "chk_app_users_display_name_source", table: "app_users"},
		{name: "chk_tenants_tenant_type", table: "tenants"},
		{name: "chk_tenant_memberships_role", table: "tenant_memberships"},
		{name: "fk_user_memory_profiles_membership", table: "user_memory_profiles"},
		{name: "fk_memory_events_membership", table: "memory_events"},
	} {
		var exists bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = $1
					AND conrelid = $2::regclass
			)
		`, constraint.name, constraint.table).Scan(&exists)
		if err != nil {
			t.Fatalf("check constraint %s: %v", constraint.name, err)
		}
		if !exists {
			t.Fatalf("expected constraint %s to exist", constraint.name)
		}
	}
}

func assertIdentityRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, userID, email, displayName string) {
	t.Helper()

	var count int
	err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM app_users u
		JOIN tenant_memberships m ON m.user_id = u.id
		JOIN tenants t ON t.id = m.tenant_id
		WHERE u.id = $1
			AND t.id = $2
			AND u.email = $3
			AND u.display_name = $4
			AND t.tenant_type = 'personal'
			AND m.role = 'owner'
	`, userID, tenantID, email, displayName).Scan(&count)
	if err != nil {
		t.Fatalf("query identity rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one user/tenant membership row, got %d", count)
	}
}

func assertMembershipConstraint(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		INSERT INTO memory_events (
			id,
			tenant_id,
			user_id,
			source,
			type,
			summary,
			payload,
			occurred_at,
			created_at
		)
		VALUES ($1, $2, $3, 'system', 'memory_api_checked', '', '{}'::jsonb, now(), now())
	`, "mem_evt_orphan_"+fmt.Sprint(time.Now().UnixNano()), "missing-tenant", "missing-user")
	if err == nil {
		t.Fatal("expected orphan memory event insert to fail")
	}
}

func containsEvent(events []memory.Event, id string) bool {
	for _, event := range events {
		if event.ID == id {
			return true
		}
	}
	return false
}

func cleanupRows(t *testing.T, pool *pgxpool.Pool, tenantID, userID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type cleanupStatement struct {
		sql  string
		args []any
	}
	statements := []cleanupStatement{
		{sql: `DELETE FROM memory_events WHERE tenant_id = $1 AND user_id = $2`, args: []any{tenantID, userID}},
		{sql: `DELETE FROM user_memory_profiles WHERE tenant_id = $1 AND user_id = $2`, args: []any{tenantID, userID}},
		{sql: `DELETE FROM tenant_memberships WHERE tenant_id = $1 AND user_id = $2`, args: []any{tenantID, userID}},
		{sql: `DELETE FROM tenants WHERE id = $1`, args: []any{tenantID}},
		{sql: `DELETE FROM app_users WHERE id = $1`, args: []any{userID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Logf("cleanup failed for %q: %v", statement.sql, err)
		}
	}
}

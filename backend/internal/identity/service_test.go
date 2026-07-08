package identity

import (
	"context"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
)

func TestServiceEnsuresPersonalTenant(t *testing.T) {
	store := NewInMemoryStore()
	service := NewService(store)

	err := service.EnsurePersonalTenant(context.Background(), auth.Principal{
		UserID:          "kevin",
		DefaultTenantID: "personal-kevin",
		TenantIDs:       []string{"personal-kevin"},
		UserMetadata: auth.UserMetadata{
			Email:       "kevin@example.com",
			DisplayName: "Kevin Chen",
		},
	})
	if err != nil {
		t.Fatalf("EnsurePersonalTenant returned error: %v", err)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	if _, ok := store.users["kevin"]; !ok {
		t.Fatal("expected user to be bootstrapped")
	}
	user := store.users["kevin"]
	if user.Email != "kevin@example.com" {
		t.Fatalf("expected email metadata, got %q", user.Email)
	}
	if user.DisplayName != "Kevin Chen" {
		t.Fatalf("expected display name metadata, got %q", user.DisplayName)
	}
	if _, ok := store.tenants["personal-kevin"]; !ok {
		t.Fatal("expected tenant to be bootstrapped")
	}

	membership := store.memberships[membershipKey("personal-kevin", "kevin")]
	if membership.Role != "owner" {
		t.Fatalf("expected owner role, got %q", membership.Role)
	}
}

func TestServiceUsesStableFallbackDisplayName(t *testing.T) {
	store := NewInMemoryStore()
	service := NewService(store)

	err := service.EnsurePersonalTenant(context.Background(), auth.Principal{
		UserID:          "auth0|user_123",
		DefaultTenantID: "personal-auth0-user-123",
		TenantIDs:       []string{"personal-auth0-user-123"},
	})
	if err != nil {
		t.Fatalf("EnsurePersonalTenant returned error: %v", err)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	user := store.users["auth0|user_123"]
	if user.DisplayName != "auth0|user_123" {
		t.Fatalf("expected user id fallback display name, got %q", user.DisplayName)
	}
}

func TestServiceKeepsExistingMetadataWhenClaimsAreOmitted(t *testing.T) {
	store := NewInMemoryStore()
	service := NewService(store)
	principal := auth.Principal{
		UserID:          "auth0|user_123",
		DefaultTenantID: "personal-auth0-user-123",
		TenantIDs:       []string{"personal-auth0-user-123"},
		UserMetadata: auth.UserMetadata{
			Email:       "kevin@example.com",
			DisplayName: "Kevin Chen",
		},
	}

	if err := service.EnsurePersonalTenant(context.Background(), principal); err != nil {
		t.Fatalf("first EnsurePersonalTenant returned error: %v", err)
	}
	principal.UserMetadata = auth.UserMetadata{}
	if err := service.EnsurePersonalTenant(context.Background(), principal); err != nil {
		t.Fatalf("second EnsurePersonalTenant returned error: %v", err)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	user := store.users["auth0|user_123"]
	if user.Email != "kevin@example.com" {
		t.Fatalf("expected email to be preserved, got %q", user.Email)
	}
	if user.DisplayName != "Kevin Chen" {
		t.Fatalf("expected display name to be preserved, got %q", user.DisplayName)
	}
	if len(store.memberships) != 1 {
		t.Fatalf("expected one idempotent membership, got %d", len(store.memberships))
	}
}

func TestServiceRejectsMissingUser(t *testing.T) {
	service := NewService(NewInMemoryStore())

	err := service.EnsurePersonalTenant(context.Background(), auth.Principal{
		DefaultTenantID: "personal-kevin",
	})
	if err == nil {
		t.Fatal("expected missing user error")
	}
}

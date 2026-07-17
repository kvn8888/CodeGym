package identity

import (
	"context"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
)

func TestServiceEnsuresPersonalWorkspace(t *testing.T) {
	store := NewInMemoryStore()
	service := NewService(store)

	err := service.EnsurePersonalWorkspace(context.Background(), auth.Principal{
		UserID:             "kevin",
		DefaultWorkspaceID: "personal-kevin",
		WorkspaceIDs:       []string{"personal-kevin"},
		UserMetadata: auth.UserMetadata{
			Email:       "kevin@example.com",
			DisplayName: "Kevin Chen",
		},
	})
	if err != nil {
		t.Fatalf("EnsurePersonalWorkspace returned error: %v", err)
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
	if _, ok := store.workspaces["personal-kevin"]; !ok {
		t.Fatal("expected workspace to be bootstrapped")
	}

	membership := store.memberships[membershipKey("personal-kevin", "kevin")]
	if membership.Role != "owner" {
		t.Fatalf("expected owner role, got %q", membership.Role)
	}
}

func TestServiceUsesStableFallbackDisplayName(t *testing.T) {
	store := NewInMemoryStore()
	service := NewService(store)

	err := service.EnsurePersonalWorkspace(context.Background(), auth.Principal{
		UserID:             "auth0|user_123",
		DefaultWorkspaceID: "personal-auth0-user-123",
		WorkspaceIDs:       []string{"personal-auth0-user-123"},
	})
	if err != nil {
		t.Fatalf("EnsurePersonalWorkspace returned error: %v", err)
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
		UserID:             "auth0|user_123",
		DefaultWorkspaceID: "personal-auth0-user-123",
		WorkspaceIDs:       []string{"personal-auth0-user-123"},
		UserMetadata: auth.UserMetadata{
			Email:       "kevin@example.com",
			DisplayName: "Kevin Chen",
		},
	}

	if err := service.EnsurePersonalWorkspace(context.Background(), principal); err != nil {
		t.Fatalf("first EnsurePersonalWorkspace returned error: %v", err)
	}
	principal.UserMetadata = auth.UserMetadata{}
	if err := service.EnsurePersonalWorkspace(context.Background(), principal); err != nil {
		t.Fatalf("second EnsurePersonalWorkspace returned error: %v", err)
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

func TestServicePreservesUserDisplayNameOverride(t *testing.T) {
	store := NewInMemoryStore()
	service := NewService(store)
	principal := auth.Principal{
		UserID:             "auth0|user_123",
		DefaultWorkspaceID: "personal-auth0-user-123",
		WorkspaceIDs:       []string{"personal-auth0-user-123"},
		UserMetadata: auth.UserMetadata{
			Email:       "kevin@example.com",
			DisplayName: "Kevin From Google",
		},
	}

	if err := service.EnsurePersonalWorkspace(context.Background(), principal); err != nil {
		t.Fatalf("EnsurePersonalWorkspace returned error: %v", err)
	}
	updated, err := service.UpdateDisplayName(context.Background(), principal, "Kevin Chen")
	if err != nil {
		t.Fatalf("UpdateDisplayName returned error: %v", err)
	}
	if updated.DisplayName != "Kevin Chen" || updated.DisplayNameSource != "user" {
		t.Fatalf("updated profile = %#v", updated)
	}

	principal.UserMetadata.DisplayName = "Kevin From Google Again"
	if err := service.EnsurePersonalWorkspace(context.Background(), principal); err != nil {
		t.Fatalf("second EnsurePersonalWorkspace returned error: %v", err)
	}

	profile, err := service.GetUserProfile(context.Background(), principal)
	if err != nil {
		t.Fatalf("GetUserProfile returned error: %v", err)
	}
	if profile.DisplayName != "Kevin Chen" {
		t.Fatalf("expected manual display name to survive OAuth metadata, got %q", profile.DisplayName)
	}
	if profile.DisplayNameSource != "user" {
		t.Fatalf("expected user display name source, got %q", profile.DisplayNameSource)
	}
}

func TestServiceRejectsMissingUser(t *testing.T) {
	service := NewService(NewInMemoryStore())

	err := service.EnsurePersonalWorkspace(context.Background(), auth.Principal{
		DefaultWorkspaceID: "personal-kevin",
	})
	if err == nil {
		t.Fatal("expected missing user error")
	}
}

func TestServiceRejectsInvalidDisplayName(t *testing.T) {
	service := NewService(NewInMemoryStore())
	principal := auth.Principal{
		UserID:             "kevin",
		DefaultWorkspaceID: "personal-kevin",
		WorkspaceIDs:       []string{"personal-kevin"},
	}
	if err := service.EnsurePersonalWorkspace(context.Background(), principal); err != nil {
		t.Fatalf("EnsurePersonalWorkspace returned error: %v", err)
	}

	if _, err := service.UpdateDisplayName(context.Background(), principal, "  "); err == nil {
		t.Fatal("expected blank display name to be rejected")
	}
}

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
	})
	if err != nil {
		t.Fatalf("EnsurePersonalTenant returned error: %v", err)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	if _, ok := store.users["kevin"]; !ok {
		t.Fatal("expected user to be bootstrapped")
	}
	if _, ok := store.tenants["personal-kevin"]; !ok {
		t.Fatal("expected tenant to be bootstrapped")
	}

	membership := store.memberships[membershipKey("personal-kevin", "kevin")]
	if membership.Role != "owner" {
		t.Fatalf("expected owner role, got %q", membership.Role)
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

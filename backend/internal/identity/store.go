package identity

import (
	"context"
	"errors"
)

// ErrUserNotFound indicates the authenticated user has not been bootstrapped.
var ErrUserNotFound = errors.New("identity user not found")

// ErrInvalidProfile indicates user-editable profile input failed validation.
var ErrInvalidProfile = errors.New("invalid profile")

// PersonalTenant defines the minimum fields required to ensure a personal
// tenant membership for an authenticated user.
type PersonalTenant struct {
	UserID            string
	TenantID          string
	Role              string
	Email             string
	DisplayName       string
	DisplayNameSource string
}

// UserProfile is the editable profile surface for the authenticated user.
type UserProfile struct {
	UserID            string `json:"user_id"`
	Email             string `json:"email"`
	DisplayName       string `json:"display_name"`
	DisplayNameSource string `json:"display_name_source"`
	DefaultTenantID   string `json:"default_tenant_id"`
}

// Store defines persistence operations for identity bootstrap behavior.
type Store interface {
	EnsurePersonalTenant(ctx context.Context, tenant PersonalTenant) error
	GetUserProfile(ctx context.Context, userID string) (UserProfile, error)
	UpdateDisplayName(ctx context.Context, userID, displayName string) (UserProfile, error)
}

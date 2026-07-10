package identity

import (
	"context"
	"errors"
)

// ErrUserNotFound indicates the authenticated user has not been bootstrapped.
var ErrUserNotFound = errors.New("identity user not found")

// ErrInvalidProfile indicates user-editable profile input failed validation.
var ErrInvalidProfile = errors.New("invalid profile")

// PersonalWorkspace defines the minimum fields required to ensure a personal
// workspace membership for an authenticated user.
type PersonalWorkspace struct {
	UserID            string
	WorkspaceID       string
	Role              string
	Email             string
	DisplayName       string
	DisplayNameSource string
}

// UserProfile is the editable profile surface for the authenticated user.
type UserProfile struct {
	UserID             string `json:"user_id"`
	Email              string `json:"email"`
	DisplayName        string `json:"display_name"`
	DisplayNameSource  string `json:"display_name_source"`
	DefaultWorkspaceID string `json:"default_workspace_id"`
}

// Store defines persistence operations for identity bootstrap behavior.
type Store interface {
	EnsurePersonalWorkspace(ctx context.Context, workspace PersonalWorkspace) error
	GetUserProfile(ctx context.Context, userID string) (UserProfile, error)
	UpdateDisplayName(ctx context.Context, userID, displayName string) (UserProfile, error)
}

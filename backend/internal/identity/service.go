package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kvn8888/codegym/backend/internal/auth"
)

const maxDisplayNameLength = 80

// Service ensures identity bootstrap invariants for authenticated users.
type Service struct {
	store Store
}

// NewService creates an identity service with the provided persistence store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// EnsurePersonalWorkspace creates or refreshes a principal's personal workspace membership.
func (s *Service) EnsurePersonalWorkspace(ctx context.Context, principal auth.Principal) error {
	if s == nil || s.store == nil {
		return nil
	}

	userID := strings.TrimSpace(principal.UserID)
	workspaceID := strings.TrimSpace(principal.DefaultWorkspaceID)
	if userID == "" {
		return errors.New("personal workspace bootstrap requires a user id")
	}
	if workspaceID == "" {
		return errors.New("personal workspace bootstrap requires a default workspace id")
	}

	return s.store.EnsurePersonalWorkspace(ctx, PersonalWorkspace{
		UserID:            userID,
		WorkspaceID:       workspaceID,
		Role:              "owner",
		Email:             strings.TrimSpace(principal.UserMetadata.Email),
		DisplayName:       strings.TrimSpace(principal.UserMetadata.DisplayName),
		DisplayNameSource: displayNameSourceFor(principal.UserMetadata.DisplayName),
	})
}

// GetUserProfile returns the authenticated user's editable profile.
func (s *Service) GetUserProfile(ctx context.Context, principal auth.Principal) (UserProfile, error) {
	if s == nil || s.store == nil {
		return UserProfile{}, ErrUserNotFound
	}

	userID := strings.TrimSpace(principal.UserID)
	if userID == "" {
		return UserProfile{}, errors.New("user profile requires a user id")
	}

	profile, err := s.store.GetUserProfile(ctx, userID)
	if err != nil {
		return UserProfile{}, err
	}
	if profile.DefaultWorkspaceID == "" {
		profile.DefaultWorkspaceID = strings.TrimSpace(principal.DefaultWorkspaceID)
	}
	return profile, nil
}

// UpdateDisplayName stores a user-controlled display name.
func (s *Service) UpdateDisplayName(ctx context.Context, principal auth.Principal, displayName string) (UserProfile, error) {
	if s == nil || s.store == nil {
		return UserProfile{}, ErrUserNotFound
	}

	userID := strings.TrimSpace(principal.UserID)
	if userID == "" {
		return UserProfile{}, errors.New("user profile requires a user id")
	}

	name := strings.TrimSpace(displayName)
	if name == "" {
		return UserProfile{}, fmt.Errorf("%w: display name is required", ErrInvalidProfile)
	}
	if utf8.RuneCountInString(name) > maxDisplayNameLength {
		return UserProfile{}, fmt.Errorf("%w: display name must be 80 characters or fewer", ErrInvalidProfile)
	}

	profile, err := s.store.UpdateDisplayName(ctx, userID, name)
	if err != nil {
		return UserProfile{}, err
	}
	if profile.DefaultWorkspaceID == "" {
		profile.DefaultWorkspaceID = strings.TrimSpace(principal.DefaultWorkspaceID)
	}
	return profile, nil
}

func displayNameSourceFor(displayName string) string {
	if strings.TrimSpace(displayName) == "" {
		return "fallback"
	}
	return "oauth"
}
